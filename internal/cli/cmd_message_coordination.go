package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/api"
	. "github.com/myco-ai/myco/internal/cli/cmdkit"
	"github.com/myco-ai/myco/internal/client"
)

func messageTreeCommand() *Command {
	return &Command{Name: "tree", Summary: "Show a task's delegated work and dependencies", Examples: []string{"myco message tree --id msg_task --format json"}, Flags: []Flag{idFlag, {Name: "limit", Kind: Int, Default: "200", Help: "Maximum tree entries (1-500); larger trees are marked truncated"}}, Run: handler(func(e *env) error {
		if err := e.in.Require("id"); err != nil {
			return err
		}
		c, err := e.messageCoordinationClient()
		if err != nil {
			return err
		}
		out, err := c.MessageTree(e.in.Ctx, e.in.String("id"), e.in.Int("limit"))
		if err != nil {
			return err
		}
		return e.p.Print(out, func(w io.Writer) {
			printTaskTree(w, out.Root, "", true)
			if out.Truncated {
				fmt.Fprintln(w, "… truncated (increase --limit, up to 500)")
			}
		})
	})}
}
func printTaskTree(w io.Writer, n api.TaskTreeNode, prefix string, root bool) {
	printTaskBranch(w, n, prefix, root, true)
}
func printTaskBranch(w io.Writer, n api.TaskTreeNode, prefix string, root, last bool) {
	label := prefix
	if !root {
		if last {
			label += "└─ "
		} else {
			label += "├─ "
		}
	}
	waits := []string{}
	for _, v := range n.WaitingOn {
		s := v.Kind
		if v.ID != "" {
			s += "=" + v.ID
		}
		waits = append(waits, s)
	}
	extra := ""
	if len(waits) > 0 {
		extra = " waits:" + strings.Join(waits, ",")
	}
	if n.Reference {
		extra += " (reference)"
	}
	if n.Relation == "thread_inferred" {
		extra += " (thread inferred)"
	}
	fmt.Fprintf(w, "%s%s %s %s age:%s%s\n", label, n.ID, n.Agent, n.Status, (time.Duration(n.AgeSeconds) * time.Second).String(), extra)
	for i, c := range n.Children {
		next := prefix
		if !root {
			if last {
				next += "   "
			} else {
				next += "│  "
			}
		}
		printTaskBranch(w, c, next, false, i == len(n.Children)-1)
	}
}
func messageReassignCommand() *Command {
	return &Command{Name: "reassign", Summary: "Move an unclaimed queued task to another agent", Mutating: true, Examples: []string{"myco message reassign --id msg_task --to worker --note 'Take over this queued task' --dry-run"}, Flags: []Flag{idFlag, {Name: "to", Kind: String, Required: true, Help: "New recipient agent name"}, {Name: "note", Kind: String, Help: "Context for the new recipient (at most 2000 characters; original body is unchanged)"}}, Run: handler(func(e *env) error {
		if err := e.in.Require("id", "to"); err != nil {
			return err
		}
		c, err := e.messageCoordinationClient()
		if err != nil {
			return err
		}
		out, err := c.ReassignMessage(e.in.Ctx, e.in.String("id"), api.ReassignRequest{To: e.in.String("to"), Note: e.in.String("note")}, e.in.Bool("dry-run"))
		if err != nil {
			return err
		}
		if out.DryRun {
			return e.dryRun(map[string]any{"action": "message.reassign", "message": out})
		}
		return e.p.Print(out, func(w io.Writer) { fmt.Fprintf(w, "reassigned %s: %s → %s\n", out.ID, out.PreviousTo, out.To) })
	})}
}

// Keep an agent's own identity when it is configured; never retry a rejected
// request with a person's credentials. A manual admin-only CLI can use its
// explicitly configured administrative credential.
func (e *env) messageCoordinationClient() (*client.Client, error) {
	if e.cfg.Token != "" || e.in.Getenv("MYCO_MESSAGE_ID") != "" {
		return e.agentClient()
	}
	return e.adminClient()
}
