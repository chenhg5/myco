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

func (r *Runner) clearPending(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.pending, id)
	r.saveState()
}

func (r *Runner) recoverPending(ctx context.Context) {
	r.mu.Lock()
	items := make([]pendingRun, 0, len(r.pending))
	for _, v := range r.pending {
		items = append(items, v)
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
			err, exhausted = r.retryFinish(finishCtx, log, "recover fail", func(c context.Context) error {
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
		log.Info("recovered unfinished task", "previous_status", status)
	}
}
