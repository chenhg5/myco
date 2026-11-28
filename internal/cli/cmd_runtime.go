package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/api"
	. "github.com/myco-ai/myco/internal/cli/cmdkit"
	"github.com/myco-ai/myco/internal/harness/claude"
)

// agentRuntimeCommands: an agent that receives in its own session (no
// runner starts its CLI) says which model it runs on, for display.
func agentRuntimeCommands() *Command {
	set := &Command{
		Name:     "set",
		Summary:  "Say which model (and reasoning effort) you run on, for people to see",
		Mutating: true,
		Long: `For an agent that receives in its own session (myco inbox wait): no runner starts its CLI,
so myco cannot tell its model. In Claude Code and Codex, myco inbox wait reports it by itself
(read from the session's transcript); elsewhere run this once after joining, and again if it
changes. It is for display only. An agent its runner starts reports this itself; a report from here is replaced by
the runner's next one.`,
		Examples: []string{"myco agent runtime set --model <the model you run on> --effort high"},
		Flags: []Flag{
			{Name: "model", Kind: String, Required: true, Help: "The model you run on, in your CLI's own naming"},
			{Name: "effort", Kind: String, Help: "Your reasoning effort, if your CLI has one"},
		},
		Run: handler(func(e *env) error {
			if err := e.in.Require("model"); err != nil {
				return err
			}
			model, effort := strings.TrimSpace(e.in.String("model")), strings.TrimSpace(e.in.String("effort"))
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "runtime_set", "agent": e.cfg.Agent, "model": model, "effort": effort})
			}
			c, err := e.agentClient()
			if err != nil {
				return err
			}
			me, err := c.Me(e.in.Ctx)
			if err != nil {
				return err
			}
			rt := api.Runtime{}
			if me.Runtime != nil {
				rt = *me.Runtime
			}
			rt.Model, rt.ModelSource = model, "self"
			rt.Effort, rt.EffortSource = effort, ""
			if effort != "" {
				rt.EffortSource = "self"
			}
			if rt.CLI == "" {
				rt.CLI = me.Binding.Harness
			}
			if err := c.ReportRuntime(e.in.Ctx, rt); err != nil {
				return err
			}
			return e.p.Print(map[string]any{"agent": me.Name, "runtime": rt}, func(w io.Writer) {
				fmt.Fprintf(w, "%s: runs on %s", me.Name, model)
				if effort != "" {
					fmt.Fprintf(w, " (effort %s)", effort)
				}
				fmt.Fprintln(w)
			})
		}),
	}
	quotaCmd := &Command{
		Name:     "quota",
		Summary:  "Report your Claude account's usage limits from a rate_limit_event, for people to see",
		Mutating: true,
		Long: `For an agent in its own Claude Code session with no runner (a claude.ai cloud session that
receives with myco inbox wait): no runner collects its account's quota, but each turn's
rate_limit_event in the session's event stream has it. Pass that event's JSON (or a list of
events: the newest is used; or just its rate_limit_info) and the five-hour and seven-day use
and reset times are reported. Only those numbers and times leave: no account, no token.
An agent whose runner collects its quota keeps the runner's; an older report does not replace
a newer one. myco inbox wait reports by itself when the session's transcript says a limit was
reached.`,
		Examples: []string{
			"myco agent runtime quota --file rate_limit_event.json",
			"echo '<the latest rate_limit_event as JSON>' | myco agent runtime quota --file -",
		},
		Flags: []Flag{{Name: "file", Kind: String, Required: true, Help: "JSON with a rate_limit_info ('-' = stdin)"}},
		Run: handler(func(e *env) error {
			if err := e.in.Require("file"); err != nil {
				return err
			}
			data, err := e.readJSONFile("file", 4<<20)
			if err != nil {
				return err
			}
			info, at, ok := newestRateLimitInfo(data)
			var q *api.Quota
			if ok {
				q = sessionQuota(claude.RateLimitQuota(info, at), time.Now())
			}
			if q == nil {
				return errorf("invalid_argument", ExitUsage, "pass a rate_limit_event, or its rate_limit_info with unifiedWindows",
					"no usable rate_limit_info in --file")
			}
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "runtime_quota", "agent": e.cfg.Agent, "quota": q})
			}
			c, err := e.agentClient()
			if err != nil {
				return err
			}
			me, err := c.Me(e.in.Ctx)
			if err != nil {
				return err
			}
			rt := api.Runtime{}
			if me.Runtime != nil {
				rt = *me.Runtime
			}
			kept := ""
			switch {
			case rt.Quota != nil && rt.Quota.ProfileRef != "":
				kept = "its runner reports this agent's quota"
			case !mayReportSessionQuota(rt.Quota, q):
				kept = "a newer report is kept"
			}
			if kept == "" {
				rt.Quota = q
				if rt.CLI == "" {
					rt.CLI = me.Binding.Harness
				}
				if err := c.ReportRuntime(e.in.Ctx, rt); err != nil {
					return err
				}
			}
			return e.p.Print(map[string]any{"agent": me.Name, "quota": q, "reported": kept == "", "kept": kept}, func(w io.Writer) {
				if kept != "" {
					fmt.Fprintf(w, "%s: not reported (%s)\n", me.Name, kept)
					return
				}
				fmt.Fprintf(w, "%s:", me.Name)
				for _, win := range q.Windows {
					fmt.Fprintf(w, " %s %.0f%%", win.Name, win.Used*100)
					if win.ResetsAt != nil {
						fmt.Fprintf(w, " (resets %s)", win.ResetsAt.Local().Format("Jan 2 15:04"))
					}
				}
				fmt.Fprintln(w)
			})
		}),
	}
	return (&Command{Name: "runtime", Summary: "What you run on (model) and your account's limits, as people see them"}).Add(set, quotaCmd, statusLineCommand())
}

// readJSONFile reads a flag's file ('-' = stdin), at most limit bytes.
func (e *env) readJSONFile(flag string, limit int64) ([]byte, error) {
	path := strings.TrimSpace(e.in.String(flag))
	if path == "-" {
		return io.ReadAll(io.LimitReader(e.in.Stdin, limit))
	}
	if err := safePath(path); err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errorf("not_found", ExitNotFound, "check the path", "cannot read %s: %v", path, err)
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, limit))
}
