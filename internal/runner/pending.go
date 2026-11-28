package runner

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/harness"
)

type pendingRun struct {
	MessageID string    `json:"message_id"`
	Agent     string    `json:"agent"`
	StartedAt time.Time `json:"started_at"`
	// Finish, once the harness is done but the hub could not be told:
	// "reply" (Text is the reply), "fail" (Text is why) or "complete".
	// Sent again until it lands (retryUndelivered, and at the next start).
	Finish string `json:"finish,omitempty"`
	Text   string `json:"text,omitempty"`
	// Requeued: the runner restarted mid-run once already and put the task
	// back in the queue; a second restart fails it.
	Requeued bool `json:"requeued,omitempty"`
}

// RunSuccess records successful local CLI use, without task or credential data.
type RunSuccess struct {
	CLI string    `json:"cli"`
	At  time.Time `json:"at"`
}

// ReadSuccess reads only success metadata. Older runner files have none.
func ReadSuccess(path string) *RunSuccess {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var st struct {
		Success *RunSuccess `json:"success"`
	}
	if json.NewDecoder(io.LimitReader(f, 8<<20)).Decode(&st) != nil {
		return nil
	}
	return st.Success
}

type runnerState struct {
	Success *RunSuccess           `json:"success,omitempty"`
	Runs    map[string]int64      `json:"runs"`
	Tasks   map[string]pendingRun `json:"tasks"`
}

func loadPending(path string) map[string]pendingRun {
	out := map[string]pendingRun{}
	if path == "" {
		return out
	}
	b, err := os.ReadFile(path)
	if err == nil {
		var st runnerState
		if json.Unmarshal(b, &st) == nil {
			for k, v := range st.Tasks {
				out[k] = v
			}
		}
	}
	return out
}

// saveState is called with r.mu held. The old runs-only JSON remains readable.
func (r *Runner) saveState() {
	r.opts.Activity.Pending(SlotOwner(r.opts.HubURL, r.opts.Self), len(r.pending))
	if r.opts.StatePath == "" {
		return
	}
	st := runnerState{Runs: map[string]int64{}, Tasks: r.pending, Success: r.lastSuccess}
	for k, v := range r.ownRunEnd {
		st.Runs[k] = v.UnixMilli()
	}
	b, _ := json.Marshal(st)
	if err := os.MkdirAll(filepath.Dir(r.opts.StatePath), 0o700); err != nil {
		r.log.Error("saving runner state failed", "err", err)
		return
	}
	tmp := r.opts.StatePath + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		r.log.Error("saving runner state failed", "err", err)
		return
	}
	if err := os.Rename(tmp, r.opts.StatePath); err != nil {
		r.log.Error("saving runner state failed", "err", err)
	}
}

// markPending records task id as running. It reports whether the task was
// requeued after a restart cut its last run short.
func (r *Runner) markPending(id string) (requeued bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	requeued = r.pending[id].Requeued
	r.pending[id] = pendingRun{MessageID: id, Agent: r.opts.Self, StartedAt: r.opts.Now(), Requeued: requeued}
	r.saveState()
	return requeued
}

// markRequeued keeps task id in the state file, marked as put back once.
func (r *Runner) markRequeued(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p := r.pending[id]
	p.Requeued = true
	r.pending[id] = p
	r.saveState()
}

// keepFinish remembers how task id is to be finished, for a later try.
func (r *Runner) keepFinish(id, finish, text string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.pending[id]
	if !ok {
		p = pendingRun{MessageID: id, Agent: r.opts.Self, StartedAt: r.opts.Now()}
	}
	p.Finish, p.Text = finish, text
	r.pending[id] = p
	r.saveState()
}

func (r *Runner) clearPending(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.pending, id)
	r.saveState()
}

func (r *Runner) recoverPending(ctx context.Context) {
	r.finishPending(ctx, false)
}

// retryUndelivered sends again the results the hub could not be told
// (it was down past the finish budget), while the runner runs: the task
// would otherwise stay acked, "working", until the runner restarts.
func (r *Runner) retryUndelivered(ctx context.Context) {
	r.finishPending(ctx, true)
}

// finishPending closes the tasks in the state file: with their kept
// result when there is one. A task the runner stopped in the middle of
// (an upgrade or restart) goes back in the queue once, so it runs again;
// a command on an exec node, or a task cut short twice, fails.
// onlyKept: only those with a kept result (the others are running).
func (r *Runner) finishPending(ctx context.Context, onlyKept bool) {
	r.mu.Lock()
	items := make([]pendingRun, 0, len(r.pending))
	for _, v := range r.pending {
		if !onlyKept || (v.Finish != "" && !r.finishing[v.MessageID]) {
			items = append(items, v)
		}
	}
	r.mu.Unlock()
	for _, item := range items {
		if ctx.Err() != nil {
			return
		}
		log := r.log.With("message_id", item.MessageID, "agent", item.Agent, "started_at", item.StartedAt)
		finishCtx, cancel := context.WithTimeout(ctx, r.finishTimeout)
		var status string
		err, exhausted := r.retryFinish(finishCtx, log, "recover get message", func(c context.Context) error {
			m, err := r.hub.GetMessage(c, item.MessageID)
			if err == nil {
				status = m.Status
			}
			return err
		})
		_, executor := r.harness.(harness.CommandExecutor)
		requeue := item.Finish == "" && !item.Requeued && !executor
		if err == nil && status == "acked" && requeue {
			err, exhausted = r.retryFinish(finishCtx, log, "recover requeue", func(c context.Context) error {
				_, err := r.hub.Release(c, item.MessageID, 0, false)
				return err
			})
			cancel()
			if err == nil {
				r.markRequeued(item.MessageID)
				log.Info("put an interrupted task back in the queue", "previous_status", status)
				continue
			}
			if e, ok := client.AsError(err); ok && e.Status == 404 {
				r.clearPending(item.MessageID)
			} else {
				r.logFinishFailure(log, "recover task", err, exhausted, "")
			}
			continue
		}
		if err == nil && status == "acked" {
			err, exhausted = r.retryFinish(finishCtx, log, "recover "+orStr(item.Finish, "fail"), func(c context.Context) error {
				switch item.Finish {
				case "reply":
					_, err := r.hub.Reply(c, item.MessageID, item.Text, false)
					return err
				case "fail":
					_, err := r.hub.Fail(c, item.MessageID, item.Text, false)
					return err
				case "complete":
					return r.hub.Complete(c, item.MessageID)
				}
				_, err := r.hub.Fail(c, item.MessageID, lostReason(executor), false)
				return err
			})
		}
		cancel()
		if err != nil {
			if e, ok := client.AsError(err); ok && e.Status == 404 {
				r.clearPending(item.MessageID)
				continue
			}
			r.logFinishFailure(log, "recover task", err, exhausted, "")
			continue
		}
		r.clearPending(item.MessageID)
		if item.Finish != "" {
			log.Info("delivered a result kept for later", "finish", item.Finish, "previous_status", status)
		} else {
			log.Info("recovered unfinished task", "previous_status", status)
		}
	}
}

// lostReason is why a task the runner stopped in the middle of failed.
func lostReason(executor bool) string {
	if executor {
		return "the runner restarted (an upgrade or a restart of its service) while this command ran; it may have finished in part. Check, then send it again if needed"
	}
	return "the runner restarted twice (an upgrade or a restart of its service) while this task ran; its result was lost. Send it again"
}

// MarkOwnRun records in a runner's state file that myco itself last wrote
// sessionID, at at: a session another part of myco made for the agent (a
// computer's first turn for a new agent). Its runner then does not take
// that write for someone else using the session.
func MarkOwnRun(path, sessionID string, at time.Time) error {
	if path == "" || sessionID == "" {
		return nil
	}
	st := runnerState{Runs: map[string]int64{}}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &st)
		if st.Runs == nil {
			st.Runs = map[string]int64{}
		}
	}
	st.Runs[sessionID] = at.UnixMilli()
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// PendingCount accounts for unfinished claims before a profile can reach its Hub.
func PendingCount(path string) int { return len(loadPending(path)) }
