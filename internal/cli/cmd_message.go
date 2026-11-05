package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/chenhg5/myco/internal/api"
	. "github.com/chenhg5/myco/internal/cli/cmdkit"
)

var idFlag = Flag{Name: "id", Kind: String, Required: true, Env: "MYCO_MESSAGE_ID",
	Help: "Message ID (defaults to $MYCO_MESSAGE_ID inside a runner)"}

func messageCommands() *Command {
	send := &Command{
		Name:     "send",
		Summary:  "Send a task or note to an agent",
		Mutating: true,
		Long: `A task expects one reply; --wait blocks until it arrives (exit 7 on timeout).
Inside a runner, --thread defaults to the current thread so replies resume this conversation.`,
		Examples: []string{
			`myco message send --to coder --body "Run the test suite and report failures"`,
			`myco message send --to coder --body-file task.md --wait 15m --format json`,
			`echo "deploy finished" | myco message send --to ops --type note --body-file -`,
			`myco message send --to reviewer --body "please review" --attach design.md --attach diagram.png`,
		},
		Flags: []Flag{
			{Name: "to", Kind: String, Required: true, Help: "Recipient agent name"},
			{Name: "body", Kind: String, Help: "Message text"},
			{Name: "body-file", Kind: String, Help: "Read the message from a file ('-' = stdin)"},
			{Name: "type", Kind: Enum, Values: []string{"task", "note"}, Default: "task", Help: "task expects a reply; note does not"},
			{Name: "thread", Kind: String, Env: "MYCO_THREAD_ID", Help: "Continue an existing thread"},
			{Name: "idempotency-key", Kind: String, Help: "Retry-safe key: resending with the same key returns the original message"},
			{Name: "wait", Kind: Duration, Default: "0s", Help: "Wait this long for the reply (tasks only)"},
			attachFlag,
		},
		Run: handler(func(e *env) error {
			if err := e.in.Require("to"); err != nil {
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
			res, err := c.Send(e.in.Ctx, api.SendRequest{
				To: e.in.String("to"), Type: e.in.String("type"), Body: body,
				ThreadID: e.in.String("thread"), IdempotencyKey: e.in.String("idempotency-key"), Attachments: atts,
			}, dry)
			if err != nil {
				return err
			}
			if dry {
				return e.dryRun(map[string]any{"action": "send", "message": res.Message})
			}
			wait := e.in.Duration("wait")
			if wait <= 0 || res.Message.Type != "task" {
				return e.p.Print(res, func(w io.Writer) {
					to := res.Message.To
					if res.ViaAlias != "" {
						to = fmt.Sprintf("%s (via alias %s)", to, res.ViaAlias)
					}
					fmt.Fprintf(w, "sent %s to %s (thread %s)\n", res.Message.ID, to, res.Message.ThreadID)
				})
			}
			fmt.Fprintf(e.in.Stderr, "waiting up to %s for reply to %s...\n", wait, res.Message.ID)
			reply, err := c.WaitReply(e.in.Ctx, res.Message.ID, wait)
			if err != nil {
				return err
			}
			out := map[string]any{"message": res.Message, "reply": reply, "timed_out": reply == nil}
			if reply == nil {
				_ = e.p.Print(out, nil)
				return &Error{Code: "timeout", Message: fmt.Sprintf("no reply to %s within %s", res.Message.ID, wait),
					Suggestion: fmt.Sprintf("keep waiting with: myco message wait --id %s --timeout 10m", res.Message.ID),
					Retryable:  true, Exit: ExitTimeout}
			}
			return e.p.Print(out, func(w io.Writer) { printReply(w, reply) })
		}),
	}

	get := &Command{
		Name:     "get",
		Summary:  "Show a message you sent or received",
		Examples: []string{"myco message get --id msg_abc123"},
		Flags:    []Flag{idFlag},
		Run: handler(func(e *env) error {
			if err := e.in.Require("id"); err != nil {
				return err
			}
			c, err := e.agentClient()
			if err != nil {
				return err
			}
			m, err := c.GetMessage(e.in.Ctx, e.in.String("id"))
			if err != nil {
				return err
			}
			return e.p.Print(m, func(w io.Writer) { printMessage(w, m) })
		}),
	}

	wait := &Command{
		Name:     "wait",
		Summary:  "Wait for the reply to a task you sent",
		Examples: []string{"myco message wait --id msg_abc123 --timeout 10m"},
		Flags: []Flag{
			{Name: "id", Kind: String, Required: true, Help: "ID of the task you sent"},
			{Name: "timeout", Kind: Duration, Default: "60s", Help: "Give up after this long (exit 7)"},
		},
		Run: handler(func(e *env) error {
			if err := e.in.Require("id"); err != nil {
				return err
			}
			c, err := e.agentClient()
			if err != nil {
				return err
			}
			reply, err := c.WaitReply(e.in.Ctx, e.in.String("id"), e.in.Duration("timeout"))
			if err != nil {
				return err
			}
			if reply == nil {
				return &Error{Code: "timeout", Message: "no reply yet", Retryable: true, Exit: ExitTimeout,
					Suggestion: "wait again, or check status with: myco message get --id " + e.in.String("id")}
			}
			return e.p.Print(map[string]any{"reply": reply}, func(w io.Writer) { printReply(w, reply) })
		}),
	}

	reply := &Command{
		Name:     "reply",
		Summary:  "Answer a task (closes it)",
		Mutating: true,
		Examples: []string{
			`myco message reply --id msg_abc123 --body "Done: PR #42 opened"`,
			"myco message reply --body-file result.md   # inside a runner, --id defaults to $MYCO_MESSAGE_ID",
		},
		Flags: []Flag{idFlag,
			{Name: "body", Kind: String, Help: "Reply text"},
			{Name: "body-file", Kind: String, Help: "Read the reply from a file ('-' = stdin)"},
			attachFlag,
		},
		Run: handler(func(e *env) error {
			if err := e.in.Require("id"); err != nil {
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
			m, err := c.ReplyWith(e.in.Ctx, e.in.String("id"), body, atts, dry)
			if err != nil {
				return err
			}
			if dry {
				return e.dryRun(map[string]any{"action": "reply", "reply": m})
			}
			return e.p.Print(map[string]any{"reply": m}, func(w io.Writer) { fmt.Fprintf(w, "replied %s -> %s\n", m.ID, m.To) })
		}),
	}

	fail := &Command{
		Name:     "fail",
		Summary:  "Close a task as failed (the sender is notified)",
		Mutating: true,
		Examples: []string{`myco message fail --id msg_abc123 --reason "repository not accessible"`},
		Flags:    []Flag{idFlag, {Name: "reason", Kind: String, Required: true, Help: "Why it failed"}},
		Run: handler(func(e *env) error {
			if err := e.in.Require("id", "reason"); err != nil {
				return err
			}
			c, err := e.agentClient()
			if err != nil {
				return err
			}
			dry := e.in.Bool("dry-run")
			m, err := c.Fail(e.in.Ctx, e.in.String("id"), e.in.String("reason"), dry)
			if err != nil {
				return err
			}
			if dry {
				return e.dryRun(map[string]any{"action": "fail", "message": m})
			}
			return e.p.Print(map[string]any{"message": m}, func(w io.Writer) { fmt.Fprintf(w, "%s marked failed\n", m.ID) })
		}),
	}

	release := &Command{
		Name:     "release",
		Summary:  "Hand a pulled message back for later redelivery (not counted as a failure)",
		Mutating: true,
		Long:     "Use when you cannot process it right now (e.g. your session is busy); it becomes due again after --delay.",
		Examples: []string{"myco message release --id msg_abc123 --delay 5m"},
		Flags:    []Flag{idFlag, {Name: "delay", Kind: Duration, Default: "2m", Help: "Redeliver after this long (max 1h)"}},
		Run: handler(func(e *env) error {
			if err := e.in.Require("id"); err != nil {
				return err
			}
			c, err := e.agentClient()
			if err != nil {
				return err
			}
			dry := e.in.Bool("dry-run")
			m, err := c.Release(e.in.Ctx, e.in.String("id"), e.in.Duration("delay"), dry)
			if err != nil {
				return err
			}
			if dry {
				return e.dryRun(map[string]any{"action": "release", "message": m})
			}
			return e.p.Print(map[string]any{"message": m}, func(w io.Writer) {
				fmt.Fprintf(w, "%s released; redelivered after %s\n", m.ID, e.in.String("delay"))
			})
		}),
	}

	transition := func(verb, summary, example string) *Command {
		return &Command{
			Name: verb, Summary: summary, Mutating: true, Examples: []string{example}, Flags: []Flag{idFlag},
			Run: handler(func(e *env) error {
				if err := e.in.Require("id"); err != nil {
					return err
				}
				c, err := e.agentClient()
				if err != nil {
					return err
				}
				dry := e.in.Bool("dry-run")
				m, err := c.Transition(e.in.Ctx, e.in.String("id"), verb, dry)
				if err != nil {
					return err
				}
				if dry {
					return e.dryRun(map[string]any{"action": verb, "message": m})
				}
				return e.p.Print(map[string]any{"message": m}, func(w io.Writer) { fmt.Fprintf(w, "%s is now %s\n", m.ID, m.Status) })
			}),
		}
	}

	return (&Command{Name: "message", Summary: "Send, answer and track messages"}).Add(
		send, get, wait, reply, fail, release,
		transition("ack", "Acknowledge a pulled message (stops redelivery)", "myco message ack --id msg_abc123"),
		transition("hold", "Accept a task but answer later (runner will not auto-reply)", "myco message hold --id $MYCO_MESSAGE_ID"),
		transition("done", "Close a message without replying", "myco message done --id msg_abc123"),
	)
}

func inboxCommands() *Command {
	pull := &Command{
		Name:    "pull",
		Summary: "Lease the next messages from your inbox",
		Long: `Pulled messages are leased: ack/reply/fail/done them before the lease expires
or they are redelivered. Most agents use 'myco runner start' instead.`,
		Examples: []string{"myco inbox pull", "myco inbox pull --limit 5 --wait 30s --format json"},
		Flags: []Flag{
			{Name: "limit", Kind: Int, Default: "10", Help: "Max messages (1-100)"},
			{Name: "wait", Kind: Duration, Default: "0s", Help: "Long-poll up to this long if the inbox is empty (max 60s)"},
		},
		Run: handler(func(e *env) error {
			c, err := e.agentClient()
			if err != nil {
				return err
			}
			msgs, err := c.Pull(e.in.Ctx, e.in.Int("limit"), e.in.Duration("wait"))
			if err != nil {
				return err
			}
			return e.p.Print(api.MessageList{Messages: nonNilMsgs(msgs)}, func(w io.Writer) {
				if len(msgs) == 0 {
					fmt.Fprintln(w, "inbox empty")
					return
				}
				for i := range msgs {
					printMessage(w, &msgs[i])
					fmt.Fprintln(w)
				}
			})
		}),
	}
	wait := &Command{
		Name:    "wait",
		Summary: "Block until your inbox has messages (does not lease them)",
		Long: `Exit 0 with the pending messages, or exit 7 when the timeout passes. Lets an agent that
cannot be woken from outside sleep cheaply, e.g. a cloud session running it as a background
command and resuming when it exits.`,
		Examples: []string{
			"myco inbox wait --timeout 2h && myco inbox pull",
			"myco inbox wait --timeout 30m --format json",
		},
		Flags: []Flag{{Name: "timeout", Kind: Duration, Default: "60s", Help: "Give up after this long (exit 7)"}},
		Run: handler(func(e *env) error {
			c, err := e.agentClient()
			if err != nil {
				return err
			}
			msgs, err := c.WaitPending(e.in.Ctx, e.in.Duration("timeout"))
			if err != nil {
				return err
			}
			if len(msgs) == 0 {
				return &Error{Code: "timeout", Message: "no messages arrived", Retryable: true, Exit: ExitTimeout,
					Suggestion: "wait again: myco inbox wait --timeout " + e.in.String("timeout")}
			}
			return e.p.Print(api.MessageList{Messages: msgs}, func(w io.Writer) {
				fmt.Fprintf(w, "%d message(s) waiting; run: myco inbox pull\n", len(msgs))
			})
		}),
	}
	return (&Command{Name: "inbox", Summary: "Read your inbox"}).Add(pull, wait)
}

func threadCommands() *Command {
	get := &Command{
		Name:     "get",
		Summary:  "Show every message in a thread",
		Examples: []string{"myco thread get --id msg_abc123"},
		Flags:    []Flag{{Name: "id", Kind: String, Required: true, Env: "MYCO_THREAD_ID", Help: "Thread ID (= ID of its first message)"}},
		Run: handler(func(e *env) error {
			if err := e.in.Require("id"); err != nil {
				return err
			}
			c, err := e.agentClient()
			if err != nil {
				return err
			}
			msgs, err := c.Thread(e.in.Ctx, e.in.String("id"))
			if err != nil {
				return err
			}
			return e.p.Print(api.MessageList{Messages: nonNilMsgs(msgs)}, func(w io.Writer) {
				tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "TIME\tFROM\tTO\tTYPE\tSTATUS\tBODY")
				for _, m := range msgs {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", m.CreatedAt.Local().Format(time.TimeOnly), m.From, m.To, m.Type, m.Status, truncate(m.Body, 60))
				}
				tw.Flush()
			})
		}),
	}
	return (&Command{Name: "thread", Summary: "Inspect conversations"}).Add(get)
}

func nonNilMsgs(m []api.Message) []api.Message {
	if m == nil {
		return []api.Message{}
	}
	return m
}

func printMessage(w io.Writer, m *api.Message) {
	fmt.Fprintf(w, "%s  %s -> %s  [%s, %s]  thread %s\n", m.ID, m.From, m.To, m.Type, m.Status, m.ThreadID)
	if m.Error != "" {
		fmt.Fprintf(w, "error: %s\n", m.Error)
	}
	printAttachments(w, m.Attachments)
	fmt.Fprintln(w, strings.TrimRight(m.Body, "\n"))
}

func printReply(w io.Writer, m *api.Message) {
	fmt.Fprintf(w, "%s from %s:\n%s\n", m.Type, m.From, strings.TrimRight(m.Body, "\n"))
	printAttachments(w, m.Attachments)
}
