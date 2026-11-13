package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/chenhg5/myco/internal/api"
	. "github.com/chenhg5/myco/internal/cli/cmdkit"
)

// agentRuntimeCommands: an agent that receives in its own session (no
// runner starts its CLI) says which model it runs on, for display.
func agentRuntimeCommands() *Command {
	set := &Command{
		Name:     "set",
		Summary:  "Say which model (and reasoning effort) you run on, for people to see",
		Mutating: true,
		Long: `For an agent that receives in its own session (myco inbox wait): no runner starts its CLI,
so myco cannot tell its model. Run this once after joining, and again if it changes. It is for
display only. An agent its runner starts reports this itself; a report from here is replaced by
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
	return (&Command{Name: "runtime", Summary: "What you run on (model), as people see it"}).Add(set)
}
