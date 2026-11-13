package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/api"
	. "github.com/chenhg5/myco/internal/cli/cmdkit"
	"github.com/chenhg5/myco/internal/client"
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
			`myco message send --to backend --project myco --body "Add a project filter to /v1/console/recent"`,
		},
		Flags: []Flag{
			{Name: "to", Kind: String, Required: true, Help: "Recipient agent name"},
			{Name: "body", Kind: String, Help: "Message text"},
			{Name: "body-file", Kind: String, Help: "Read the message from a file ('-' = stdin)"},
			{Name: "type", Kind: Enum, Values: []string{"task", "note"}, Default: "task", Help: "task expects a reply; note does not"},
			{Name: "thread", Kind: String, Env: "MYCO_THREAD_ID", Help: "Continue an existing thread"},
			{Name: "after", Kind: String, Help: "Deliver after this task from you to the same agent finishes"},
			{Name: "project", Kind: String, Env: "MYCO_PROJECT", Help: "Put a new thread in this project (you must be a member); a message in an existing thread belongs to its project"},
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
				ThreadID: e.in.String("thread"), After: e.in.String("after"), IdempotencyKey: e.in.String("idempotency-key"), Attachments: atts,
				Project: e.in.String("project"),
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
			reply, progress, err := e.waitWithProgress(c, res.Message.ID, wait)
			if err != nil {
				return err
			}
			out := map[string]any{"message": res.Message, "reply": reply, "timed_out": reply == nil, "progress": progress}
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
		Summary:  "Wait for the reply to a task you sent, showing its progress updates as they arrive",
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
			reply, progress, err := e.waitWithProgress(c, e.in.String("id"), e.in.Duration("timeout"))
			if err != nil {
				return err
			}
			if reply == nil {
				_ = e.p.Print(map[string]any{"reply": nil, "timed_out": true, "progress": progress}, nil)
				return &Error{Code: "timeout", Message: "no reply yet", Retryable: true, Exit: ExitTimeout,
					Suggestion: "wait again, or check status with: myco message get --id " + e.in.String("id")}
			}
			return e.p.Print(map[string]any{"reply": reply, "progress": progress}, func(w io.Writer) { printReply(w, reply) })
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
			{Name: "anyway", Kind: Bool, Help: "Reply even if unread corrections from the sender remain"},
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
			if !e.in.Bool("anyway") {
				steers, err := pendingSteers(e.in.Ctx, c, e.in.String("id"), false)
				if err != nil {
					return err
				}
				if len(steers) > 0 {
					return unreadSteersError(steers)
				}
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
				if verb == "ack" {
					steers, err := pendingSteers(e.in.Ctx, c, e.in.String("id"), true)
					if err != nil {
						return err
					}
					return e.p.Print(map[string]any{"message": m, "steers": steers}, func(w io.Writer) {
						fmt.Fprintf(w, "%s is now %s\n", m.ID, m.Status)
						printSteers(w, steers)
					})
				}
				return e.p.Print(map[string]any{"message": m}, func(w io.Writer) { fmt.Fprintf(w, "%s is now %s\n", m.ID, m.Status) })
			}),
		}
	}

	progress := &Command{
		Name:     "progress",
		Summary:  "Tell the sender how a task is going, without closing it",
		Mutating: true,
		Long: `For long tasks: the sender sees each update while waiting (myco message send --wait, myco message
wait) and in the thread, but is not woken up by it. Finish with myco message reply as usual. For a
status everyone can see, use myco agent status set.`,
		Examples: []string{
			`myco message progress --body "tests pass; now updating the docs (2/3)"   # inside a runner, --id is $MYCO_MESSAGE_ID`,
			`myco message progress --id msg_abc123 --body "found the cause: a race in the lease check"`,
			`myco message progress --body "the new home page so far" --attach home.png   # a screenshot shows in the app`,
		},
		Flags: []Flag{idFlag, {Name: "body", Kind: String, Required: true, Help: "Update text"}, attachFlag},
		Run: handler(func(e *env) error {
			if err := e.in.Require("id", "body"); err != nil {
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
			m, err := c.ProgressWith(e.in.Ctx, e.in.String("id"), e.in.String("body"), atts, dry)
			if err != nil {
				return err
			}
			if dry {
				return e.dryRun(map[string]any{"action": "progress", "message": m})
			}
			if len(atts) > 0 && len(m.Attachments) == 0 {
				// A hub before progress attachments keeps only the text.
				fmt.Fprintln(e.in.Stderr, "note: the hub kept the text but not the files (it predates files in progress updates); send them with the reply instead")
			}
			steers, err := pendingSteers(e.in.Ctx, c, e.in.String("id"), true)
			if err != nil {
				return err
			}
			return e.p.Print(map[string]any{"message": m, "steers": steers}, func(w io.Writer) {
				fmt.Fprintf(w, "progress sent to %s\n", m.To)
				printSteers(w, steers)
			})
		}),
	}

	steer := &Command{
		Name:     "steer",
		Summary:  "Correct a task you sent while it is being worked on",
		Mutating: true,
		Long: `If a myco runner is working on the task, it interrupts the run and resumes the same session with your
note (work done so far is kept); the final answer is still the task's reply. If the task has not
started, the note is added to it. Agents without a runner receive it in their inbox.`,
		Examples: []string{
			`myco message steer --id msg_abc123 --body "use PostgreSQL, not SQLite"`,
			"myco message steer --id msg_abc123 --body-file correction.md && myco message wait --id msg_abc123 --timeout 30m",
		},
		Flags: []Flag{
			{Name: "id", Kind: String, Required: true, Help: "ID of the task you sent"},
			{Name: "body", Kind: String, Help: "Correction text"},
			{Name: "body-file", Kind: String, Help: "Read the correction from a file (- for stdin)"},
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
			m, err := c.SteerWith(e.in.Ctx, e.in.String("id"), body, atts, dry)
			if err != nil {
				return err
			}
			if dry {
				return e.dryRun(map[string]any{"action": "steer", "message": m})
			}
			return e.p.Print(map[string]any{"message": m}, func(w io.Writer) {
				fmt.Fprintf(w, "steer sent to %s for task %s\nwait for the (updated) answer with: myco message wait --id %s --timeout 30m\n", m.To, m.ReplyTo, m.ReplyTo)
			})
		}),
	}

	ask := &Command{
		Name:     "ask",
		Summary:  "Ask the sender of a task to decide, and wait for the answer",
		Mutating: true,
		Long: `Only when the sender really has to decide (a direction, something costly or hard to undo):
the question shows in their app as a card with the choices to click, or they can just write.
The task waits (held) until they answer. End your turn after asking: the answer comes back to
this session as a new message, and you go on with the task and reply as usual.`,
		Examples: []string{
			`myco message ask --question "Which opening?" --option "Result first — show the finished cut" --option "Steps first"   # inside a runner, --id is $MYCO_MESSAGE_ID`,
			`myco message ask --id msg_abc123 --question "Delete the old table now or after the migration?"`,
			`myco message ask --question "Which layout?" --option "A" --option "B" --attach a.png --attach b.png`,
		},
		Flags: []Flag{
			idFlag,
			{Name: "question", Kind: String, Required: true, Help: "What they should decide"},
			{Name: "option", Kind: List, Help: "A choice: a short label, optionally \" — \" and a line on it; repeatable (2-6). Without any, they answer in words"},
			attachFlag,
		},
		Run: handler(func(e *env) error {
			if err := e.in.Require("id", "question"); err != nil {
				return err
			}
			body, err := askBody(e.in.String("question"), e.in.List("option"))
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
			m, fellBack, err := c.Ask(e.in.Ctx, e.in.String("id"), body, atts, dry)
			if err != nil {
				return err
			}
			if dry {
				return e.dryRun(map[string]any{"action": "ask", "message": m})
			}
			return e.p.Print(map[string]any{"message": m, "task": e.in.String("id"), "held": true, "plain_progress": fellBack}, func(w io.Writer) {
				fmt.Fprintf(w, "asked %s; task %s waits for the answer\n", m.To, e.in.String("id"))
				if fellBack {
					fmt.Fprintln(w, "(this hub predates questions: it shows as a progress update; the answer still comes back here)")
				}
				fmt.Fprintln(w, "End your turn now: the answer arrives in this session as a new message; then finish the task and reply.")
			})
		}),
	}

	answer := &Command{
		Name:     "answer",
		Summary:  "Answer the question a task you sent is waiting on",
		Mutating: true,
		Long: `For a task you sent whose recipient asked you to decide (myco message ask). The answer is a
steer: the recipient goes on with the task in the same session. --choice picks one of its
choices by number; --body says it in words (or adds to the choice).`,
		Examples: []string{
			`myco message answer --id msg_abc123 --choice 1`,
			`myco message answer --id msg_abc123 --choice 2 --body "but keep it under 40 seconds"`,
			`myco message answer --id msg_abc123 --body "neither: show both side by side"`,
		},
		Flags: []Flag{
			{Name: "id", Kind: String, Required: true, Help: "ID of the task you sent"},
			{Name: "choice", Kind: Int, Help: "Number of the choice (1, 2, ...)"},
			{Name: "body", Kind: String, Help: "The answer in words"},
		},
		Run: handler(func(e *env) error {
			if err := e.in.Require("id"); err != nil {
				return err
			}
			c, err := e.agentClient()
			if err != nil {
				return err
			}
			task, err := c.GetMessage(e.in.Ctx, e.in.String("id"))
			if err != nil {
				return err
			}
			if task.Asking == "" {
				return &Error{Code: "invalid_state", Message: fmt.Sprintf("task %s is not waiting on a question", task.ID),
					Suggestion: "to change what it does, use: myco message steer --id " + task.ID + " --body ...", Exit: ExitConflict}
			}
			body := strings.TrimSpace(e.in.String("body"))
			if n := e.in.Int("choice"); n > 0 {
				q, err := c.GetMessage(e.in.Ctx, task.Asking)
				if err != nil {
					return err
				}
				question, options := parseAsk(q.Body)
				if n > len(options) {
					return &Error{Code: "invalid_usage", Message: fmt.Sprintf("the question has %d choices", len(options)), Exit: ExitUsage}
				}
				body = strings.TrimSpace("「" + question + "」" + options[n-1] + "\n" + body)
			}
			if body == "" {
				return &Error{Code: "invalid_usage", Message: "give --choice or --body", Exit: ExitUsage}
			}
			dry := e.in.Bool("dry-run")
			m, err := c.SteerWith(e.in.Ctx, task.ID, body, nil, dry)
			if err != nil {
				return err
			}
			if dry {
				return e.dryRun(map[string]any{"action": "answer", "message": m})
			}
			return e.p.Print(map[string]any{"message": m}, func(w io.Writer) {
				fmt.Fprintf(w, "answer sent to %s for task %s\nwait for the reply with: myco message wait --id %s --timeout 30m\n", m.To, task.ID, task.ID)
			})
		}),
	}

	return (&Command{Name: "message", Summary: "Send, answer and track messages"}).Add(
		send, get, wait, progress, ask, answer, steer, reply, fail, release, stopMessageCommand(),
		transition("ack", "Start work on a pulled message (stops redelivery)", "myco message ack --id msg_abc123"),
		transition("hold", "Accept a task but answer later (runner will not auto-reply)", "myco message hold --id $MYCO_MESSAGE_ID"),
		transition("done", "Close a message without replying (also one still queued, to drop it unread)", "myco message done --id msg_abc123"),
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
			out := inboxResult{Messages: nonNilMsgs(msgs), Hint: inboxTaskHint(msgs)}
			return e.p.Print(out, func(w io.Writer) {
				if len(msgs) == 0 {
					fmt.Fprintln(w, "inbox empty")
					return
				}
				for i := range msgs {
					printMessage(w, &msgs[i])
					fmt.Fprintln(w)
				}
				if out.Hint != "" {
					fmt.Fprintln(w, out.Hint)
				}
			})
		}),
	}
	wait := &Command{
		Name:    "wait",
		Summary: "Block until your inbox has messages (does not lease them)",
		Long: `Exit 0 with the pending messages, or exit 7 when the timeout passes. Lets an agent that
cannot be woken from outside sleep cheaply, e.g. a cloud session running it as a background
command and resuming when it exits. Keep --timeout below the harness's background-command limit
(Claude Code: 2h) so the wait ends on its own with exit 7 instead of being killed, and under an
hour (55m) so idle wake-ups still hit the agent's prompt cache. Start it again after every exit:
an agent with no wait running cannot be woken.`,
		Examples: []string{
			"myco inbox wait --timeout 55m && myco inbox pull",
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
				if e.in.Ctx.Err() != nil {
					return &Error{Code: "interrupted", Message: "the wait was stopped before it finished (e.g. a background-command time limit)",
						Retryable: true, Exit: ExitTimeout,
						Suggestion: "this is expected; start it again right away with a timeout below that limit: myco inbox wait --timeout 55m"}
				}
				return err
			}
			if len(msgs) == 0 {
				return &Error{Code: "timeout", Message: "no messages arrived", Retryable: true, Exit: ExitTimeout,
					Suggestion: "this is expected; start it again right away so you stay reachable: myco inbox wait --timeout " + e.in.String("timeout")}
			}
			out := inboxResult{Messages: nonNilMsgs(msgs), Hint: inboxTaskHint(msgs)}
			return e.p.Print(out, func(w io.Writer) {
				fmt.Fprintf(w, "%d message(s) waiting; run: myco inbox pull\n", len(msgs))
				if out.Hint != "" {
					fmt.Fprintln(w, out.Hint)
				}
			})
		}),
	}
	return (&Command{Name: "inbox", Summary: "Read your inbox"}).Add(pull, wait)
}

// inboxResult adds a CLI-only hint without changing the Hub's message-list API.
type inboxResult struct {
	Messages []api.Message `json:"messages"`
	Hint     string        `json:"hint,omitempty"`
}

const taskProgressHint = `For each task: start with myco message ack --id <id>; while working, share findings and next steps with myco message progress --id <id> --body "..." (the sender sees updates live); finish with myco message reply --id <id>. Skip progress for a one-line answer.`

func inboxTaskHint(msgs []api.Message) string {
	var task, steer bool
	for _, m := range msgs {
		switch m.Type {
		case "task":
			task = true
		case "steer":
			steer = true
		}
	}
	var hints []string
	if task {
		hints = append(hints, taskProgressHint)
	}
	if steer {
		hints = append(hints, "A steer is the sender's correction to the task named by reply_to. Read it and answer that original task together; it is not a new task.")
	}
	return strings.Join(hints, " ")
}

// Older hubs have no steers endpoint. Keep their existing CLI behaviour.
func pendingSteers(ctx context.Context, c *client.Client, id string, consume bool) ([]api.Message, error) {
	steers, err := c.Steers(ctx, id, consume)
	if e, ok := client.AsError(err); ok && e.Status == 404 {
		return []api.Message{}, nil
	}
	if err != nil {
		return nil, err
	}
	if steers == nil {
		steers = []api.Message{}
	}
	return steers, nil
}

func printSteers(w io.Writer, steers []api.Message) {
	for _, steer := range steers {
		fmt.Fprintf(w, "%s 补充了：%s（这是对当前任务的补充，不是新任务）\n", steer.From, steer.Body)
		for _, a := range steer.Attachments {
			fmt.Fprintf(w, "  附件：%s (%s)\n", a.Name, a.FileID)
		}
	}
}

func unreadSteersError(steers []api.Message) *Error {
	var details []string
	for _, steer := range steers {
		details = append(details, fmt.Sprintf("%s: %s", steer.ID, steer.Body))
	}
	return &Error{Code: "conflict", Exit: ExitConflict,
		Message:    "sender added unread corrections: " + strings.Join(details, "\n"),
		Suggestion: "读完再回答；确定要忽略就加 --anyway"}
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
				tw := newTable(w)
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

// waitWithProgress waits for the reply to task id, printing each progress
// update to stderr as it arrives.
func (e *env) waitWithProgress(c *client.Client, id string, d time.Duration) (*api.Message, []api.Message, error) {
	progress := []api.Message{}
	reply, err := c.WaitReplyProgress(e.in.Ctx, id, d, func(p api.Message) {
		progress = append(progress, p)
		fmt.Fprintf(e.in.Stderr, "[progress %s from %s] %s\n", p.CreatedAt.Local().Format("15:04:05"), p.From, strings.TrimSpace(p.Body))
		printAttachments(e.in.Stderr, p.Attachments)
	})
	return reply, progress, err
}

func printReply(w io.Writer, m *api.Message) {
	fmt.Fprintf(w, "%s from %s:\n%s\n", m.Type, m.From, strings.TrimRight(m.Body, "\n"))
	printAttachments(w, m.Attachments)
}

// askBody writes a question and its choices the way the app reads them:
// the question, a blank line, then "1. label — detail" one a line.
func askBody(question string, options []string) (string, error) {
	question = strings.TrimSpace(question)
	if question == "" {
		return "", &Error{Code: "invalid_usage", Message: "--question is empty", Exit: ExitUsage}
	}
	if len(options) == 1 || len(options) > 6 {
		return "", &Error{Code: "invalid_usage", Message: fmt.Sprintf("give 2 to 6 --option (or none), not %d", len(options)),
			Suggestion: "without --option they answer in words", Exit: ExitUsage}
	}
	var b strings.Builder
	b.WriteString(question)
	for i, o := range options {
		o = strings.Join(strings.Fields(o), " ")
		if o == "" {
			return "", &Error{Code: "invalid_usage", Message: fmt.Sprintf("--option %d is empty", i+1), Exit: ExitUsage}
		}
		if i == 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "%d. %s\n", i+1, o)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// parseAsk reads a question back: the text before the trailing numbered
// list, and the list's items (labels, with their detail).
func parseAsk(body string) (question string, options []string) {
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	end := len(lines)
	start := end
	for start > 0 {
		l := strings.TrimSpace(lines[start-1])
		dot := strings.Index(l, ". ")
		if dot <= 0 || strings.Trim(l[:dot], "0123456789") != "" {
			break
		}
		start--
	}
	for _, l := range lines[start:end] {
		l = strings.TrimSpace(l)
		options = append(options, strings.TrimSpace(l[strings.Index(l, ". ")+2:]))
	}
	return strings.TrimSpace(strings.Join(lines[:start], "\n")), options
}
