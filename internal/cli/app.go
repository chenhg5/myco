// Package cli defines myco's agent-friendly command line (noun-verb tree,
// JSON output when piped, structured errors, semantic exit codes).
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/cli/cmdkit"
	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/config"
)

// Version is set at build time via -ldflags "-X .../cli.Version=...".
var Version = "dev"

// Main runs the CLI and returns the process exit code.
func Main(args []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return Run(ctx, args, os.Stdin, os.Stdout, os.Stderr, os.Getenv, isTerminal(os.Stdout))
}

// Run executes args with explicit IO (used by tests).
func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string, tty bool) int {
	if len(args) == 1 && args[0] == "--version" {
		fmt.Fprintln(stdout, Version)
		return ExitOK
	}
	root := Root()
	cmd, values, pos, err := cmdkit.Resolve(root, args)
	p := &Printer{Format: resolveFormat(values, getenv, tty), Out: stdout, Err: stderr}

	var help cmdkit.HelpRequest
	if errors.As(err, &help) {
		footer := ""
		if help.Command == root {
			footer = exitCodeHelp
		}
		cmdkit.WriteHelp(stdout, help.Command, footer)
		return ExitOK
	}
	if err != nil {
		e := toError(err, cmd.Path())
		p.PrintError(e)
		return e.Exit
	}
	inv := cmdkit.NewInvocation(ctx, cmd, values, pos, stdin, stdout, stderr, getenv)
	err = cmd.Run(inv)
	var dry dryRunDone
	var status exitStatus
	switch {
	case err == nil:
		return ExitOK
	case errors.As(err, &dry):
		return ExitDryRun
	case errors.As(err, &status):
		return int(status)
	}
	e := toError(err, cmd.Path())
	p.PrintError(e)
	return e.Exit
}

func resolveFormat(values map[string][]string, getenv func(string) string, tty bool) Format {
	v := ""
	if f := values["format"]; len(f) > 0 {
		v = f[len(f)-1]
	} else if getenv != nil {
		v = getenv("MYCO_FORMAT")
	}
	switch v {
	case "json":
		return FormatJSON
	case "text":
		return FormatText
	}
	if tty {
		return FormatText
	}
	return FormatJSON
}

// env bundles per-invocation dependencies for command handlers.
type env struct {
	in      *cmdkit.Invocation
	p       *Printer
	cfgPath string
	cfg     *config.Config
	// explicitConfig is true when --config / MYCO_CONFIG chose the file.
	explicitConfig bool
	// defaultPath is ~/.myco/config.yaml (profiles live next to it).
	defaultPath string
}

// handler adapts a func(*env) error into a cmdkit Run func.
func handler(fn func(*env) error) func(*cmdkit.Invocation) error {
	return func(in *cmdkit.Invocation) error {
		path := in.String("config")
		explicit := path != ""
		def := config.DefaultPath()
		if !explicit {
			path = def
			// Inside a known session, use that session's own identity so
			// several agents on one machine never share a config.
			if b := detectBinding(in.Getenv); b.SessionID != "" {
				if p, ok := config.FindBySession(config.ProfilesDir(def), b.SessionID); ok {
					path = p.Path
				}
			}
		}
		cfg, err := config.Load(path)
		if err != nil {
			return errorf("invalid_config", ExitUsage, "fix or remove "+path, "cannot read config %s: %v", path, err)
		}
		cfg.ApplyEnv(in.Getenv)
		if h := in.String("hub"); h != "" && in.IsSet("hub") {
			cfg.Hub = h
		}
		tty := false
		if f, ok := in.Stdout.(*os.File); ok {
			tty = isTerminal(f)
		}
		p := &Printer{Format: resolveFormat(map[string][]string{"format": nonEmpty(in.String("format"))}, in.Getenv, tty), Out: in.Stdout, Err: in.Stderr}
		return fn(&env{in: in, p: p, cfgPath: path, cfg: cfg, explicitConfig: explicit, defaultPath: def})
	}
}

func nonEmpty(s string) []string {
	if s == "" || s == "auto" {
		return nil
	}
	return []string{s}
}

func (e *env) hubURL() (string, error) {
	if e.cfg.Hub == "" {
		// A machine that joined with a link, or the hub's own host (as root).
		if m, _ := config.LoadMachine(e.defaultPath); m != nil && m.Hub != "" {
			e.cfg.Hub = m.Hub
		} else if h := e.readHubEnv()["MYCO_HUB"]; h != "" {
			e.cfg.Hub = h
		}
	}
	if e.cfg.Hub == "" {
		return "", errorf("not_configured", ExitUsage,
			"pass --hub <url>, set MYCO_HUB, or run: myco agent register --hub <url> --name <name>",
			"no hub configured")
	}
	return e.cfg.Hub, nil
}

// agentClient returns a client authenticated as the configured agent.
func (e *env) agentClient() (*client.Client, error) {
	hub, err := e.hubURL()
	if err != nil {
		return nil, err
	}
	if e.cfg.Token == "" && e.in.Getenv(config.EnvEnrollToken) != "" {
		if err := e.autoEnroll(hub); err != nil {
			return nil, err
		}
	}
	if e.cfg.Token == "" {
		return nil, errorf("not_configured", ExitUsage,
			"set MYCO_TOKEN or MYCO_ENROLL_TOKEN, or register this agent: myco agent register --name <name>",
			"no agent token configured")
	}
	return client.New(hub, e.cfg.Token)
}

// autoEnroll joins the network as the current session using
// MYCO_ENROLL_TOKEN (idempotent: the same session gets the same agent back)
// and saves the resulting credentials.
func (e *env) autoEnroll(hub string) error {
	b := detectBinding(e.in.Getenv)
	if b.SessionID == "" {
		return errorf("not_configured", ExitUsage,
			"run `myco agent enroll --session-id <id> --harness <name>` to enroll explicitly",
			"MYCO_ENROLL_TOKEN is set but no session could be detected for auto-enrollment")
	}
	res, err := e.enroll(hub, e.in.Getenv(config.EnvEnrollToken), api.EnrollRequest{Binding: b}, false)
	if err != nil {
		return err
	}
	fmt.Fprintf(e.in.Stderr, "myco: enrolled as %s (session %s); credentials saved to %s\n", res.Agent.Name, b.SessionID, e.cfgPath)
	return nil
}

// enroll calls POST /v1/enroll and, unless dry, saves hub/agent/token.
func (e *env) enroll(hub, token string, req api.EnrollRequest, dry bool) (*api.RegisterResponse, error) {
	c, err := client.New(hub, token)
	if err != nil {
		return nil, err
	}
	res, err := c.Enroll(e.in.Ctx, req, dry)
	if err != nil || dry {
		return res, err
	}
	if err := e.saveIdentity(hub, res.Agent.Name, res.Token, res.Agent.Binding); err != nil {
		return nil, fmt.Errorf("enrolled but saving config failed: %w", err)
	}
	return res, nil
}

// saveIdentity stores an agent's credentials. Identities bound to a session
// go to their own profile (~/.myco/agents/<agent>.yaml) unless a config file
// was chosen explicitly; others go to the current config file.
func (e *env) saveIdentity(hub, agent, token string, b api.Binding) error {
	// Session agents get a profile per session; exec nodes have no session
	// but still need a profile so `runner start --all` serves them.
	if e.explicitConfig || (b.SessionID == "" && b.Harness != "exec") {
		e.cfg.Hub, e.cfg.Agent, e.cfg.Token = hub, agent, token
		e.cfg.Harness, e.cfg.SessionID = b.Harness, b.SessionID
		return config.Save(e.cfgPath, e.cfg)
	}
	path := config.ProfilePath(e.defaultPath, agent)
	cfg, err := config.Load(path) // keep runner settings on re-enrollment
	if err != nil {
		return err
	}
	cfg.Hub, cfg.Agent, cfg.Token, cfg.Harness, cfg.SessionID = hub, agent, token, b.Harness, b.SessionID
	if cfg.Runner.Harness == "" {
		cfg.Runner.Harness = runnerHarnessFor(b.Harness)
	}
	if cfg.Runner.Workdir == "" {
		cfg.Runner.Workdir = b.Workdir
	}
	if err := config.Save(path, cfg); err != nil {
		return err
	}
	e.cfgPath, e.cfg = path, cfg
	return nil
}

// runnerHarnessFor maps a binding harness to the runner adapter that can
// resume it ("" if it cannot be driven by a local runner).
func runnerHarnessFor(h string) string {
	switch h {
	case "claude", "codex", "exec":
		return h
	}
	return ""
}

// adminClient returns a client authenticated with the hub admin token.
func (e *env) adminClient() (*client.Client, error) {
	hub, err := e.hubURL()
	if err != nil {
		return nil, err
	}
	tok := e.in.Getenv(config.EnvAdminToken)
	if f := e.in.String("admin-token-file"); f != "" {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, errorf("invalid_argument", ExitUsage, "", "cannot read --admin-token-file: %v", err)
		}
		tok = strings.TrimSpace(string(b))
	}
	if tok == "" {
		tok = e.readHubEnv()[config.EnvAdminToken] // on the hub's own machine
	}
	if tok == "" {
		return nil, errorf("not_configured", ExitUsage,
			"set MYCO_ADMIN_TOKEN or pass --admin-token-file <path> (on the hub's own server: sudo /usr/local/bin/myco ...)", "admin token required for this command")
	}
	return client.New(hub, tok)
}

// readBody resolves --body / --body-file (use "-" for stdin).
func (e *env) readBody(bodyFlag, fileFlag string) (string, error) {
	if e.in.IsSet(bodyFlag) {
		return e.in.String(bodyFlag), nil
	}
	path := e.in.String(fileFlag)
	if path == "" {
		return "", &cmdkit.UsageError{Message: fmt.Sprintf("missing --%s or --%s", bodyFlag, fileFlag),
			Suggestion: fmt.Sprintf("pass --%s <text>, or --%s - to read stdin", bodyFlag, fileFlag), Command: e.in.Cmd.Path()}
	}
	if path == "-" {
		b, err := io.ReadAll(io.LimitReader(e.in.Stdin, 1<<20))
		return string(b), err
	}
	if err := safePath(path); err != nil {
		return "", err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", errorf("not_found", ExitNotFound, "check the path", "cannot read %s: %v", path, err)
	}
	return string(b), nil
}

// safePath rejects reading from credential directories.
func safePath(p string) error {
	abs, err := filepath.Abs(p)
	if err != nil {
		return err
	}
	for _, bad := range []string{"/.ssh/", "/.gnupg/", "/.aws/", "/.myco/"} {
		if strings.Contains(abs+"/", bad) {
			return errorf("permission_denied", ExitPermission, "send content from a regular file",
				"refusing to read %s: it is inside a credentials directory", p)
		}
	}
	return nil
}

// dryRun prints a preview and returns the dry-run sentinel.
func (e *env) dryRun(v any) error {
	if err := e.p.Print(v, nil); err != nil {
		return err
	}
	return dryRunDone{}
}

// hubEnvPath is where install-hub.sh keeps the admin token and local hub
// URL (root-only). `myco hub up` keeps its own in ~/.myco/hub/hub.env.
// Admin commands run on the hub's machine use them when nothing else is set.
var hubEnvPath = "/etc/myco/hub.env"

func (e *env) readHubEnv() map[string]string {
	for _, path := range []string{filepath.Join(e.hubDir(), "hub.env"), hubEnvPath} {
		vals, err := config.ReadEnvFile(path)
		if err != nil {
			continue
		}
		out := map[string]string{}
		for _, kv := range vals {
			if k, v, ok := strings.Cut(kv, "="); ok {
				out[k] = v
			}
		}
		return out
	}
	return nil
}
