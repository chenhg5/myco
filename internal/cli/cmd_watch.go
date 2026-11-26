package cli

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/api"
	. "github.com/myco-ai/myco/internal/cli/cmdkit"
	"github.com/myco-ai/myco/internal/client"
)

func watchError(err error) error {
	if ce, ok := client.AsError(err); ok && ce.Status == 404 && (ce.Code == "internal" || strings.Contains(ce.Message, "not enabled")) {
		return errorf("unsupported", ExitUsage, "upgrade the workspace Hub to use watches", "this Hub does not provide watch endpoints")
	}
	return err
}
func watchCommands() *Command {
	root := &Command{Name: "watch", Summary: "Keep a visible, bounded background wait on a reply, task, workflow, or local command"}
	admin := Flag{Name: "admin-token-file", Kind: String, Help: "File containing the owner's/admin token"}
	id := Flag{Name: "id", Kind: String, Required: true, Help: "Watch id (wat_...)"}
	duration := Flag{Name: "for", Kind: Duration, Default: "30m", Help: "Duration from now, up to 24h from the watch's start"}
	extendDuration := duration
	extendDuration.Required = true
	extendDuration.Default = ""
	printWatch := func(e *env, w *api.Watch, err error) error {
		if err != nil {
			return watchError(err)
		}
		if e.in.Bool("dry-run") {
			return e.dryRun(w)
		}
		return e.p.Print(w, func(out io.Writer) {
			fmt.Fprintf(out, "%s  %s  %s\nDeadline: %s\n", w.ID, w.State, w.What, w.Deadline.Local().Format(time.RFC3339))
		})
	}
	root.Add(&Command{Name: "start", Summary: "Start a bounded reply, task, workflow, or local command watch", Mutating: true,
		Examples: []string{`myco watch start --what "Wait for the review" --until reply:msg_example --for 30m`, `myco watch start --what "Wait for the build" --cmd "gh run view 123 --json conclusion --jq .conclusion" --match "^success$" --every 30s --for 30m`},
		Flags:    []Flag{{Name: "what", Kind: String, Required: true, Help: "One sentence describing what you are waiting for"}, {Name: "until", Kind: String, Help: "reply:<message>, task:<message>, or run:<workflow-run>"}, {Name: "cmd", Kind: String, Help: "Shell check on this agent's permitted full-access runner (exclusive with --until)"}, {Name: "match", Kind: String, Help: "Regular expression on bounded stdout/stderr; otherwise exit code 0 matches"}, {Name: "every", Kind: Duration, Default: "30s", Help: "Command check interval (1s–24h)"}, duration, {Name: "task", Kind: String, Help: "Associated open task addressed to you"}, {Name: "project", Kind: String, Help: "Project for this watch"}},
		Run: handler(func(e *env) error {
			kind, target := "command", ""
			command := e.in.String("cmd")
			if command != "" {
				if e.in.IsSet("until") {
					return errorf("invalid_argument", ExitUsage, "choose --cmd or --until", "--cmd and --until are mutually exclusive")
				}
				if _, err := regexp.Compile(e.in.String("match")); err != nil {
					return errorf("invalid_argument", ExitUsage, "use a valid regular expression", "invalid --match: %v", err)
				}
			} else {
				var ok bool
				kind, target, ok = strings.Cut(e.in.String("until"), ":")
				if !ok || target == "" || (kind != "reply" && kind != "task" && kind != "run") {
					return errorf("invalid_argument", ExitUsage, "use --cmd or --until reply:<msg>, task:<msg>, or run:<run>", "invalid watch condition")
				}
				if e.in.IsSet("match") || e.in.IsSet("every") {
					return errorf("invalid_argument", ExitUsage, "use these flags with --cmd", "--match and --every require --cmd")
				}
			}
			every := e.in.Duration("every")
			if every < time.Second || every > 24*time.Hour || every%time.Second != 0 {
				return errorf("invalid_argument", ExitUsage, "use whole seconds from 1s to 24h", "invalid --every")
			}
			everySec := 0
			if kind == "command" {
				everySec = int(every / time.Second)
			}
			if kind == "run" {
				kind = "workflow"
			}
			d := e.in.Duration("for")
			if d < time.Second || d > 24*time.Hour || d%time.Second != 0 {
				return errorf("invalid_argument", ExitUsage, "use a whole number of seconds from 1s to 24h", "invalid --for")
			}
			c, err := e.agentClient()
			if err != nil {
				return err
			}
			w, err := c.StartWatch(e.in.Ctx, api.WatchStartRequest{What: e.in.String("what"), Kind: kind, Target: target, Task: e.in.String("task"), Project: e.in.String("project"), ForSec: int(d / time.Second), Command: command, Match: e.in.String("match"), EverySec: everySec}, e.in.Bool("dry-run"))
			return printWatch(e, w, err)
		})},
		&Command{Name: "list", Summary: "List active watches you may manage (--all includes ended watches)", Flags: []Flag{admin, {Name: "agent", Kind: String, Help: "Filter by agent name"}, {Name: "project", Kind: String, Help: "Filter by project"}, {Name: "all", Kind: Bool, Help: "Include ended watches"}, {Name: "dry-run", Kind: Bool, Help: "Preview this read-only query"}}, Run: handler(func(e *env) error {
			c, err := e.watchClient()
			if err != nil {
				return err
			}
			ws, err := c.Watches(e.in.Ctx, e.in.String("agent"), e.in.String("project"), e.in.Bool("all"))
			if err != nil {
				return watchError(err)
			}
			out := api.WatchList{Watches: ws}
			if e.in.Bool("dry-run") {
				return e.dryRun(out)
			}
			return e.p.Print(out, func(w io.Writer) {
				for _, v := range ws {
					fmt.Fprintf(w, "%s  %s  %s  %s\n", v.ID, v.Agent, v.State, v.What)
				}
			})
		})},
		&Command{Name: "stop", Summary: "Stop a watch (initiating agent, owner, or project coordinator)", Mutating: true, Flags: []Flag{admin, id, {Name: "reason", Kind: String, Help: "Why this watch was stopped"}}, Run: handler(func(e *env) error {
			c, err := e.watchClient()
			if err != nil {
				return err
			}
			w, err := c.StopWatch(e.in.Ctx, e.in.String("id"), e.in.String("reason"), e.in.Bool("dry-run"))
			return printWatch(e, w, err)
		})},
		&Command{Name: "extend", Summary: "Set a watch's deadline from now (within 24 hours of its start)", Mutating: true, Flags: []Flag{admin, id, extendDuration}, Run: handler(func(e *env) error {
			d := e.in.Duration("for")
			if d < time.Second || d > 24*time.Hour || d%time.Second != 0 {
				return errorf("invalid_argument", ExitUsage, "use a whole number of seconds from 1s to 24h", "invalid --for")
			}
			c, err := e.watchClient()
			if err != nil {
				return err
			}
			w, err := c.ExtendWatch(e.in.Ctx, e.in.String("id"), int(d/time.Second), e.in.Bool("dry-run"))
			return printWatch(e, w, err)
		})})
	return root
}

func (e *env) watchClient() (*client.Client, error) {
	if e.in.String("admin-token-file") != "" {
		return e.adminClient()
	}
	return e.projectClient()
}
