package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/api"
	. "github.com/chenhg5/myco/internal/cli/cmdkit"
)

func broadcastCommands() *Command {
	send := &Command{
		Name:     "send",
		Summary:  "Send one message to every agent matching a label selector",
		Mutating: true,
		Long: `Each recipient gets its own message and thread (recipients don't see each other's replies);
all share a broadcast ID for tracking. Agents whose allow-from excludes you are skipped.
--wait polls until every recipient answered or the timeout passes (exit 7); replies collected
this way are marked done, so they do not show up again in your inbox.`,
		Examples: []string{
			`myco broadcast send --selector "group=loops" --body "Pause deploys until 18:00" --type note`,
			`myco broadcast send --selector "machine=mac,harness=codex" --body-file task.md --wait 15m`,
			`myco broadcast send --selector "group=release" --body "status?" --dry-run   # preview recipients`,
		},
		Flags: []Flag{
			{Name: "selector", Kind: String, Required: true, Help: "Label selector: key, !key, key=value, key!=value; comma = AND (virtual keys: name, harness, capability)"},
			{Name: "body", Kind: String, Help: "Message text"},
			{Name: "body-file", Kind: String, Help: "Read the message from a file ('-' = stdin)"},
			{Name: "type", Kind: Enum, Values: []string{"task", "note"}, Default: "task", Help: "task expects replies; note does not"},
			{Name: "idempotency-key", Kind: String, Help: "Retry-safe key: resending returns the original broadcast"},
			{Name: "include-self", Kind: Bool, Help: "Also deliver to yourself if you match"},
			{Name: "wait", Kind: Duration, Default: "0s", Help: "Wait this long for all replies (tasks only)"},
			attachFlag,
		},
		Run: handler(func(e *env) error {
			if err := e.in.Require("selector"); err != nil {
				return err
			}
			body, err := e.readBody("body", "body-file")
			if err != nil {
				return err
			}
			c, err := e.agentClient()
			if err != nil {
				return err
			}
			dry := e.in.Bool("dry-run")
			atts, err := e.resolveAttachments(c, dry)
			if err != nil {
				return err
			}
			res, err := c.Broadcast(e.in.Ctx, api.BroadcastRequest{Selector: e.in.String("selector"), Type: e.in.String("type"),
				Body: body, IdempotencyKey: e.in.String("idempotency-key"), IncludeSelf: e.in.Bool("include-self"), Attachments: atts}, dry)
			if err != nil {
				return err
			}
			to := make([]string, 0, len(res.Messages))
			for _, m := range res.Messages {
				to = append(to, m.To)
			}
			if dry {
				return e.dryRun(map[string]any{"action": "broadcast", "recipients": to, "skipped": res.Skipped})
			}
			wait := e.in.Duration("wait")
			if wait <= 0 || e.in.String("type") != "task" {
				return e.p.Print(res, func(w io.Writer) {
					fmt.Fprintf(w, "broadcast %s sent to %d agent(s): %s\n", res.BroadcastID, len(to), strings.Join(to, ", "))
					for _, sk := range res.Skipped {
						fmt.Fprintf(w, "skipped %s: %s\n", sk.Agent, sk.Reason)
					}
				})
			}
			fmt.Fprintf(e.in.Stderr, "broadcast %s sent to %d agent(s); waiting up to %s for replies...\n", res.BroadcastID, len(to), wait)
			deadline := time.Now().Add(wait)
			var st *api.BroadcastStatus
			for {
				// Replies we collect here are consumed so they don't also
				// wake us through the inbox (same as message send --wait).
				if st, err = c.BroadcastStatus(e.in.Ctx, res.BroadcastID, true); err != nil {
					return err
				}
				if st.Answered == st.Total || time.Now().After(deadline) {
					break
				}
				select {
				case <-time.After(3 * time.Second):
				case <-e.in.Ctx.Done():
					return e.in.Ctx.Err()
				}
			}
			if err := e.p.Print(st, func(w io.Writer) { printBroadcast(w, st) }); err != nil {
				return err
			}
			if st.Answered < st.Total {
				return &Error{Code: "timeout", Message: fmt.Sprintf("%d of %d recipients answered", st.Answered, st.Total),
					Suggestion: "check later with: myco broadcast get --id " + res.BroadcastID, Retryable: true, Exit: ExitTimeout}
			}
			return nil
		}),
	}

	get := &Command{
		Name:     "get",
		Summary:  "Show a broadcast's recipients, statuses and replies",
		Examples: []string{"myco broadcast get --id bc_abc123", "myco broadcast get --id bc_abc123 --consume"},
		Flags: []Flag{
			{Name: "id", Kind: String, Required: true, Help: "Broadcast ID"},
			{Name: "consume", Kind: Bool, Help: "Mark the replies shown as done so they leave your inbox"},
		},
		Run: handler(func(e *env) error {
			if err := e.in.Require("id"); err != nil {
				return err
			}
			c, err := e.agentClient()
			if err != nil {
				return err
			}
			st, err := c.BroadcastStatus(e.in.Ctx, e.in.String("id"), e.in.Bool("consume"))
			if err != nil {
				return err
			}
			return e.p.Print(st, func(w io.Writer) { printBroadcast(w, st) })
		}),
	}
	return (&Command{Name: "broadcast", Summary: "Message groups of agents selected by labels"}).Add(send, get)
}

func printBroadcast(w io.Writer, st *api.BroadcastStatus) {
	fmt.Fprintf(w, "broadcast %s: %d/%d answered\n", st.BroadcastID, st.Answered, st.Total)
	tw := newTable(w)
	fmt.Fprintln(tw, "TO\tSTATUS\tREPLY")
	for _, d := range st.Deliveries {
		reply := "-"
		if d.Reply != nil {
			reply = truncate(d.Reply.Body, 80)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", d.Message.To, d.Message.Status, reply)
	}
	tw.Flush()
}
