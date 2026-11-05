package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/chenhg5/myco/internal/api"
	. "github.com/chenhg5/myco/internal/cli/cmdkit"
)

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
