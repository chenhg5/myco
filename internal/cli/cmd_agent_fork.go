package cli

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/myco-ai/myco/internal/api"
	. "github.com/myco-ai/myco/internal/cli/cmdkit"
	"github.com/myco-ai/myco/internal/client"
)

func agentForkCommand() *Command {
	return &Command{
		Name: "fork", Summary: "Copy a bound agent session, optionally into isolated git worktrees", Mutating: true,
		Long: `Creates managed copies on the source's machine. Provenance is kept by the Hub; editable labels never authorize retirement.
A copy cannot fork again. Capacity and quota checks may be overridden with --force; permissions and directory rules may not.
--worktree creates a sibling checkout on its own fork/<copy-name> branch, inside spawn.dirs. Claude --worktree is unsupported.
Copies idle for 30 minutes retire automatically. Keep the returned request ID when retrying an interrupted batch.`,
		Examples: []string{"myco agent fork --from coder --count 2 --worktree", "myco agent fork --from reviewer --for-task msg_example --idempotency-key review-batch"},
		Flags:    []Flag{{Name: "from", Kind: String, Required: true, Help: "Source agent name or alias"}, {Name: "count", Kind: Int, Default: "1", Help: "Number of copies (1-32)"}, {Name: "worktree", Kind: Bool, Help: "Give each copy a sibling git worktree and branch (Codex)"}, {Name: "name-prefix", Kind: String, Help: "Prefix for generated copy names"}, {Name: "for-task", Kind: String, Help: "Task this batch serves (default MYCO_MESSAGE_ID)"}, {Name: "force", Kind: Bool, Help: "Override only runner capacity and quota checks"}, {Name: "idempotency-key", Kind: String, Help: "Stable batch key when retrying; generated when omitted"}, {Name: "timeout", Kind: Duration, Default: "5m", Help: "Bound for the complete batch"}, {Name: "admin-token-file", Kind: String, Help: "Explicit owner/admin credential file"}},
		Run: handler(func(e *env) error {
			if err := e.in.Require("from"); err != nil {
				return err
			}
			c, err := e.cardClient()
			if err != nil {
				return err
			}
			src, err := c.GetAgent(e.in.Ctx, e.in.String("from"))
			if err != nil {
				return err
			}
			key := e.in.String("idempotency-key")
			if key == "" {
				key = rand.Text()
			}
			task := e.in.String("for-task")
			if task == "" {
				task = e.in.Getenv("MYCO_MESSAGE_ID")
			}
			in := api.AgentForkRequest{From: src.Name, Count: e.in.Int("count"), Worktree: e.in.Bool("worktree"), NamePrefix: e.in.String("name-prefix"), ForTask: task, Force: e.in.Bool("force"), RequestID: key}
			ctx, cancel := context.WithTimeout(e.in.Ctx, e.in.Duration("timeout"))
			defer cancel()
			out, err := c.ForkAgents(ctx, in, e.in.Bool("dry-run"))
			if err != nil {
				return forkBatchError(err, key, nil)
			}
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "fork", "request_id": key, "forks": compactForks(out.Forks)})
			}
			for i, f := range out.Forks {
				if f.State != "planned" {
					continue
				}
				reply, err := c.WaitReply(ctx, f.MessageID, e.in.Duration("timeout"))
				if err != nil {
					return forkBatchError(err, key, out.Forks)
				}
				if reply == nil {
					return &Error{Code: "timeout", Message: "copy registration is still pending", Suggestion: "retry with the same --idempotency-key " + key, Retryable: true, Exit: ExitTimeout, Input: map[string]any{"request_id": key, "forks": compactForks(out.Forks)}}
				}
				var res api.SpawnResult
				if reply.Type != "reply" || json.Unmarshal([]byte(reply.Body), &res) != nil {
					return errorf("fork_failed", ExitError, "inspect the machine node reply; reuse --idempotency-key "+key, "machine did not return a fork result")
				}
				if res.Denied || res.Error != "" {
					return &Error{Code: "fork_failed", Message: res.Error, Suggestion: "inspect the machine node, then retire the failed copy or retry the batch; key: " + key, Exit: ExitError, Input: map[string]any{"request_id": key, "forks": compactForks(out.Forks)}}
				}
				observed, err := c.AgentFork(ctx, f.ID)
				if err != nil {
					return forkBatchError(err, key, out.Forks)
				}
				out.Forks[i] = observed
				if out.Forks[i].State != "ready" {
					return errorf("fork_failed", ExitError, "inspect the batch using the returned request ID", "machine did not confirm copy registration")
				}
			}
			return e.p.Print(map[string]any{"request_id": key, "forks": compactForks(out.Forks)}, func(w io.Writer) {
				for _, f := range out.Forks {
					fmt.Fprintf(w, "%s fork_of=%s state=%s\n", f.Name, f.ForkOf, f.State)
				}
			})
		}),
	}
}
func agentRetireCommand() *Command {
	return &Command{
		Name: "retire", Summary: "Retire only managed copies created by you or on a machine you own", Mutating: true,
		Long: `Choose one copy by --name, or a source by --fork-of. The Hub stops new claims, then waits for current work before cleanup.
--now cancels the copy's current worker before cleanup. Worktrees with uncommitted changes are preserved and reported as an error.
Only the recorded creator, the machine owner, or a Hub administrator may retire a managed copy; labels do not grant permission.`,
		Examples: []string{"myco agent retire --fork-of coder", "myco agent retire --name coder-fork-example --now"},
		Flags:    []Flag{{Name: "name", Kind: String, Help: "Managed copy to retire"}, {Name: "fork-of", Kind: String, Help: "Retire managed copies of this source"}, {Name: "now", Kind: Bool, Help: "Cancel current work instead of waiting for it"}, {Name: "timeout", Kind: Duration, Default: "5m", Help: "Wait bound; retirement continues on the Hub afterward"}, {Name: "admin-token-file", Kind: String, Help: "Explicit machine owner/admin credential file"}},
		Run: handler(func(e *env) error {
			if (e.in.String("name") == "") == (e.in.String("fork-of") == "") {
				return errorf("invalid_argument", ExitUsage, "choose --name or --fork-of", "exactly one retirement selector is required")
			}
			c, err := e.cardClient()
			if err != nil {
				return err
			}
			selector := e.in.String("fork-of")
			if selector != "" {
				a, err := c.GetAgent(e.in.Ctx, selector)
				if err != nil {
					if ce, ok := client.AsError(err); !ok || ce.Code != "not_found" {
						return err
					}
				} else {
					selector = a.Name
				}
			}
			ctx, cancel := context.WithTimeout(e.in.Ctx, e.in.Duration("timeout"))
			defer cancel()
			out, err := c.RetireAgents(ctx, api.AgentRetireRequest{Name: e.in.String("name"), ForkOf: selector, Now: e.in.Bool("now")}, e.in.Bool("dry-run"))
			if err != nil {
				return err
			}
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "retire", "forks": compactForks(out.Forks)})
			}
			for i, f := range out.Forks {
				for f.State != "retired" {
					timer := time.NewTimer(250 * time.Millisecond)
					select {
					case <-ctx.Done():
						timer.Stop()
						return &Error{Code: "timeout", Message: "retirement is still pending", Suggestion: "retry the same retire command to observe its result", Retryable: true, Exit: ExitTimeout}
					case <-timer.C:
					}
					f, err = c.AgentFork(ctx, f.ID)
					if err != nil {
						return err
					}
					if f.Error != "" {
						return errorf("retire_failed", ExitError, "inspect the machine; uncommitted worktrees are preserved, then retry retire", "%s", f.Error)
					}
				}
				out.Forks[i] = f
			}
			return e.p.Print(map[string]any{"forks": compactForks(out.Forks)}, func(w io.Writer) {
				for _, f := range out.Forks {
					fmt.Fprintf(w, "retired %s\n", f.Name)
				}
			})
		}),
	}
}

// Lifecycle output keeps only the handles and paths needed to assign or reclaim work.
func compactForks(fs []*api.AgentFork) []map[string]any {
	out := make([]map[string]any, 0, len(fs))
	for _, f := range fs {
		m := map[string]any{"id": f.ID, "name": f.Name, "fork_of": f.ForkOf, "forked_by": f.ForkedBy, "state": f.State}
		if f.Workdir != "" {
			m["workdir"] = f.Workdir
		}
		if f.Branch != "" {
			m["branch"] = f.Branch
		}
		if f.ForTask != "" {
			m["for_task"] = f.ForTask
		}
		out = append(out, m)
	}
	return out
}

// A network failure can happen after the Hub commits a batch. Always keep the
// same recovery key, including when it was generated rather than user supplied.
func forkBatchError(err error, key string, forks []*api.AgentFork) error {
	out := *toError(err, "agent fork")
	out.Input = map[string]any{"request_id": key, "forks": compactForks(forks)}
	out.Suggestion += "; retry with the same --idempotency-key " + key + " to recover this batch"
	return &out
}
