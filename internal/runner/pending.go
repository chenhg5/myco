package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/chenhg5/myco/internal/client"
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
}

type runnerState struct {
	Runs  map[string]int64      `json:"runs"`
	Tasks map[string]pendingRun `json:"tasks"`
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
	if r.opts.StatePath == "" {
		return
	}
	st := runnerState{Runs: map[string]int64{}, Tasks: r.pending}
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

func (r *Runner) markPending(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pending[id] = pendingRun{MessageID: id, Agent: r.opts.Self, StartedAt: r.opts.Now()}
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
// result when there is one, else (the runner stopped mid-run) as failed.
// onlyKept: only those with a kept result (the others are running).
func (r *Runner) finishPending(ctx context.Context, onlyKept bool) {
	r.mu.Lock()
	items := make([]pendingRun, 0, len(r.pending))
	for _, v := range r.pending {
		if !onlyKept || v.Finish != "" {
			items = append(items, v)
		}
	}
	r.mu.Unlock()
	for _, item := range items {
		log := r.log.With("message_id", item.MessageID, "agent", item.Agent, "started_at", item.StartedAt)
		finishCtx, cancel := r.finishContext(ctx)
		var status string
		err, exhausted := r.retryFinish(finishCtx, log, "recover get message", func(c context.Context) error {
			m, err := r.hub.GetMessage(c, item.MessageID)
			if err == nil {
				status = m.Status
			}
			return err
		})
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
				_, err := r.hub.Fail(c, item.MessageID, "the runner restarted before this task finished; its result was lost — send it again", false)
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
