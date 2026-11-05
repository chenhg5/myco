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
	switch {
	case err == nil:
		return ExitOK
	case errors.As(err, &dry):
		return ExitDryRun
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
}

// handler adapts a func(*env) error into a cmdkit Run func.
func handler(fn func(*env) error) func(*cmdkit.Invocation) error {
	return func(in *cmdkit.Invocation) error {
		path := in.String("config")
		if path == "" {
			path = config.DefaultPath()
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
		return fn(&env{in: in, p: p, cfgPath: path, cfg: cfg})
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
	if e.cfg.Token == "" {
		return nil, errorf("not_configured", ExitUsage,
			"set MYCO_TOKEN, or register this agent: myco agent register --name <name>",
			"no agent token configured")
	}
	return client.New(hub, e.cfg.Token)
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
		return nil, errorf("not_configured", ExitUsage,
			"set MYCO_ADMIN_TOKEN or pass --admin-token-file <path>", "admin token required for this command")
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
