package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/api"
	. "github.com/chenhg5/myco/internal/cli/cmdkit"
	"github.com/chenhg5/myco/internal/config"
	"github.com/chenhg5/myco/internal/harness"
	"github.com/chenhg5/myco/internal/harness/builtin"
	"github.com/chenhg5/myco/internal/runner"
)

func runnerCommands() *Command {
	harnesses := builtin.Registry()
	start := &Command{
		Name:    "start",
		Summary: "Process your inbox continuously by invoking an agent CLI per message",
		Long: `Tasks: the CLI's final output is sent back as the reply (failures mark the task failed).
Replies, events and notes are shown to the CLI but never answered automatically.
Follow-ups in the same thread resume the same CLI session (claude/codex).
Flags override the 'runner' section of the config file. Logs go to stderr.`,
		Examples: []string{
			"myco runner start --harness claude --workdir ~/src/app",
			"myco runner start --harness codex --arg=--full-auto",
			`myco runner start --harness shell --command 'my-agent --stdin'`,
			"myco runner start --once   # process what's queued, then exit",
		},
		Flags: []Flag{
			{Name: "harness", Kind: Enum, Values: harnesses.Names(), Help: "Agent CLI adapter (default from config, else claude)"},
			{Name: "binary", Kind: String, Help: "Path to the agent CLI executable"},
			{Name: "arg", Kind: List, Help: "Extra argument for the agent CLI (repeatable), e.g. --arg=--permission-mode --arg=acceptEdits"},
			{Name: "command", Kind: String, Help: "Shell command line for --harness shell (prompt on stdin, reply on stdout)"},
			{Name: "workdir", Kind: String, Help: "Working directory for the agent CLI (default: current dir)"},
			{Name: "concurrency", Kind: Int, Help: "Messages processed in parallel (default 1)"},
			{Name: "timeout", Kind: Duration, Help: "Max time per message (default 30m)"},
			{Name: "once", Kind: Bool, Help: "Process currently queued messages once and exit"},
			{Name: "session-scope", Kind: Enum, Values: []string{"agent", "thread"}, Default: "agent",
				Help: "agent: this agent is one session, every message resumes it; thread: one session per thread"},
			{Name: "rebind", Kind: Bool, Help: "Allow taking over an agent bound to another environment or harness"},
			{Name: "log-level", Kind: Enum, Values: []string{"debug", "info", "warn", "error"}, Default: "info", Help: "Log verbosity"},
		},
		Run: handler(func(e *env) error {
			rc := e.cfg.Runner
			pick := func(flag, cfgVal, def string) string {
				if e.in.IsSet(flag) {
					return e.in.String(flag)
				}
				if cfgVal != "" {
					return cfgVal
				}
				return def
			}
			name := pick("harness", rc.Harness, "claude")
			args := rc.Args
			if e.in.IsSet("arg") {
				args = e.in.List("arg")
			}
			h, err := harnesses.New(name, harness.Options{Binary: pick("binary", rc.Binary, ""), Args: args, Command: pick("command", rc.Command, "")})
			if err != nil {
				return errorf("invalid_argument", ExitUsage, "available harnesses: "+strings.Join(harnesses.Names(), ", "), "%v", err)
			}
			c, err := e.agentClient()
			if err != nil {
				return err
			}
			me, err := c.Me(e.in.Ctx)
			if err != nil {
				return err
			}
			workdir, err := filepath.Abs(pick("workdir", rc.Workdir, "."))
			if err != nil {
				return err
			}
			if st, err := os.Stat(workdir); err != nil || !st.IsDir() {
				return errorf("not_found", ExitNotFound, "create it or pass --workdir", "workdir %s does not exist", workdir)
			}
			timeout, err := durationOr(e, "timeout", rc.Timeout, 30*time.Minute)
			if err != nil {
				return err
			}
			pollWait, err := time.ParseDuration(orDefault(rc.PollWait, "30s"))
			if err != nil {
				return errorf("invalid_config", ExitUsage, "", "runner.poll_wait: %v", err)
			}
			concurrency := rc.Concurrency
			if e.in.IsSet("concurrency") {
				concurrency = e.in.Int("concurrency")
			}
			host := hostname()
			if err := checkBinding(me, name, host, e.in.Bool("rebind")); err != nil {
				return err
			}
			if me, err = c.Bind(e.in.Ctx, api.Binding{Harness: name, Environment: host, Workdir: workdir}); err != nil {
				return err
			}
			scope := runner.SessionScope(e.in.String("session-scope"))
			var sessions runner.SessionStore = runner.NewHubSessions(c, me.Binding.SessionID)
			if scope == runner.ScopeThread {
				if sessions, err = runner.NewFileSessions(filepath.Join(config.Dir(e.cfgPath), "sessions", me.Name+".json")); err != nil {
					return err
				}
			}
			cfgAbs, _ := filepath.Abs(e.cfgPath)
			log := newLogger(e.in.String("log-level"))
			log.Info("runner started", "agent", me.Name, "hub", e.cfg.Hub, "harness", h.Name(), "workdir", workdir,
				"environment", host, "session_scope", scope, "session_id", me.Binding.SessionID)
			r := runner.New(c, h, sessions, runner.DefaultPrompt{}, runner.Options{
				Self: me.Name, Concurrency: concurrency, Timeout: timeout, PollWait: pollWait,
				Workdir: workdir, Once: e.in.Bool("once"), Scope: scope,
				// Lets the agent call `myco` as itself from inside the harness.
				Env: []string{config.EnvConfig + "=" + cfgAbs, config.EnvHub + "=" + e.cfg.Hub, config.EnvToken + "=" + e.cfg.Token},
			}, log)
			return r.Run(e.in.Ctx)
		}),
	}
	return (&Command{Name: "runner", Summary: "Turn an agent CLI into an always-reachable worker"}).Add(start)
}

// checkBinding refuses to silently move an agent's session to another
// environment or harness: the session would not exist there.
func checkBinding(me *api.Agent, harness, host string, rebind bool) error {
	b := me.Binding
	if rebind || b.SessionID == "" {
		return nil
	}
	if b.Environment != "" && b.Environment != host {
		return errorf("conflict", ExitConflict,
			fmt.Sprintf("run the runner on %s, or pass --rebind to start a new session here", b.Environment),
			"agent %s is bound to session %s in environment %s, not %s", me.Name, b.SessionID, b.Environment, host)
	}
	if b.Harness != "" && b.Harness != harness {
		return errorf("conflict", ExitConflict, "use --harness "+b.Harness+", or pass --rebind",
			"agent %s is bound to a %s session, not %s", me.Name, b.Harness, harness)
	}
	return nil
}

func durationOr(e *env, flag, cfgVal string, def time.Duration) (time.Duration, error) {
	if e.in.IsSet(flag) {
		return e.in.Duration(flag), nil
	}
	if cfgVal == "" {
		return def, nil
	}
	d, err := time.ParseDuration(cfgVal)
	if err != nil {
		return 0, errorf("invalid_config", ExitUsage, "", "runner.%s: %v", flag, err)
	}
	return d, nil
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func configCommands() *Command {
	get := &Command{
		Name:     "get",
		Summary:  "Show the effective configuration (token redacted)",
		Examples: []string{"myco config get", "myco config get --format json"},
		Run: handler(func(e *env) error {
			r := e.cfg.Redacted()
			out := map[string]any{"path": e.cfgPath, "config": r}
			return e.p.Print(out, func(w io.Writer) {
				fmt.Fprintf(w, "path:   %s\nhub:    %s\nagent:  %s\ntoken:  %s\nrunner: %+v\n", e.cfgPath, r.Hub, r.Agent, r.Token, r.Runner)
			})
		}),
	}
	return (&Command{Name: "config", Summary: "Inspect local configuration"}).Add(get)
}

func schemaCommand(root *Command) *Command {
	return &Command{
		Name:    "schema",
		Summary: "Describe commands as JSON (flags, types, enums, examples)",
		Args:    "[command path...]  e.g. 'message send'",
		Examples: []string{
			"myco schema --all",
			"myco schema message send",
		},
		Flags: []Flag{{Name: "all", Kind: Bool, Help: "Describe every runnable command"}},
		Run: func(in *Invocation) error {
			p := &Printer{Format: FormatJSON, Out: in.Stdout, Err: in.Stderr}
			if in.Bool("all") {
				var all []Schema
				Walk(root, func(c *Command) {
					if c.Run != nil {
						all = append(all, SchemaOf(c))
					}
				})
				return p.Print(map[string]any{"commands": all}, nil)
			}
			target := root
			for _, a := range in.Args {
				next := target.Find(a)
				if next == nil {
					return &UsageError{Message: fmt.Sprintf("unknown command %q under %s", a, target.Path()),
						Suggestion: "run: myco schema --all", Command: "myco schema"}
				}
				target = next
			}
			return p.Print(SchemaOf(target), nil)
		},
	}
}

func versionCommand() *Command {
	return &Command{
		Name:    "version",
		Summary: "Print the myco version",
		Run: func(in *Invocation) error {
			fmt.Fprintln(in.Stdout, Version)
			return nil
		},
	}
}
