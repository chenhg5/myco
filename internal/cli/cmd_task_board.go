package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/myco-ai/myco/internal/api"
	. "github.com/myco-ai/myco/internal/cli/cmdkit"
	"github.com/myco-ai/myco/internal/client"
)

func taskCommandError(err error) error {
	if ce, ok := client.AsError(err); ok && ce.Code == "unsupported" {
		return errorf("unsupported", ExitUsage, ce.Suggestion, "%s", ce.Message)
	}
	return err
}

func taskCommands() *Command {
	root := &Command{Name: "task", Summary: "Inspect sent tasks and configure coordinator alerts"}
	project := Flag{Name: "project", Kind: String, Help: "Project name; required with --all and alert settings"}
	root.Add(&Command{Name: "board", Summary: "All your unfinished sent tasks, stale first (metadata only)",
		Long:     "Includes queued, delivered, acked and held tasks. queued_for_ms is age since creation when queued, not historical queue latency or a requeue-episode timer. Without --stale, each task uses its project's alert threshold (15m by default). --all requires a project and its coordinator/owner permissions.",
		Examples: []string{"myco task board", "myco task board --project acme --stale 15m --all --format json"},
		Flags:    []Flag{project, {Name: "stale", Kind: Duration, Help: "Override display stale threshold (whole seconds, 1s–168h); does not change alerts"}, {Name: "all", Kind: Bool, Help: "All unfinished tasks in --project (coordinator or owner only)"}},
		Run: handler(func(e *env) error {
			if e.in.IsSet("stale") && e.in.Duration("stale") == 0 {
				return errorf("invalid_argument", ExitUsage, "use whole seconds from 1s to 168h", "invalid stale threshold")
			}
			c, err := e.projectClient()
			if err != nil {
				return err
			}
			out, err := c.TaskBoard(e.in.Ctx, e.in.String("project"), e.in.Bool("all"), e.in.Duration("stale"))
			if err != nil {
				return taskCommandError(err)
			}
			return e.p.Print(out, func(w io.Writer) {
				for _, t := range out.Tasks {
					mark := ""
					if t.Stale {
						mark = " stale"
					}
					fmt.Fprintf(w, "%s  %-12s %-9s queued=%s idle=%s%s\n", t.ID, t.To, t.Status, time.Duration(t.QueuedForMS)*time.Millisecond, time.Duration(t.IdleMS)*time.Millisecond, mark)
				}
				if len(out.Tasks) == 0 {
					fmt.Fprintln(w, "No unfinished sent tasks.")
				}
			})
		}),
	})
	alerts := &Command{Name: "alerts", Summary: "Project stale-alert threshold (offline and machine incidents also alert)"}
	for _, set := range []bool{false, true} {
		name, summary := "show", "Show a project's task alert threshold"
		if set {
			name, summary = "set", "Set a project's stale-alert threshold"
		}
		requiredProject := project
		requiredProject.Required = true
		cmd := &Command{Name: name, Summary: summary, Mutating: set, Flags: []Flag{requiredProject}, Run: handler(func(e *env) error {
			if err := e.in.Require("project"); err != nil {
				return err
			}
			c, err := e.projectClient()
			if err != nil {
				return err
			}
			var req *api.TaskAlertPolicyRequest
			if set {
				if err := e.in.Require("stale"); err != nil {
					return err
				}
				d := e.in.Duration("stale")
				if d < time.Second || d > 7*24*time.Hour || d%time.Second != 0 {
					return errorf("invalid_argument", ExitUsage, "use whole seconds from 1s to 168h", "invalid stale threshold")
				}
				req = &api.TaskAlertPolicyRequest{StaleSeconds: int(d / time.Second)}
			}
			out, err := c.TaskAlertPolicy(e.in.Ctx, e.in.String("project"), req, e.in.Bool("dry-run"))
			if err != nil {
				return taskCommandError(err)
			}
			if set && e.in.Bool("dry-run") {
				return e.dryRun(out)
			}
			return e.p.Print(out, func(w io.Writer) {
				fmt.Fprintf(w, "%s stale=%s\n", out.Project, time.Duration(out.StaleSeconds)*time.Second)
			})
		})}
		if set {
			cmd.Flags = append(cmd.Flags, Flag{Name: "stale", Kind: Duration, Required: true, Help: "Stale threshold (whole seconds, 1s–168h)"})
		}
		alerts.Add(cmd)
	}
	root.Add(alerts)
	return root
}
