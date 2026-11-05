package runner

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/harness"
)

// Options tunes the runner.
type Options struct {
	// Self is this agent's name (used in prompts).
	Self string
	// Concurrency is the max number of messages processed in parallel.
	Concurrency int
	// Timeout bounds one harness invocation.
	Timeout time.Duration
	// PollWait is the long-poll duration per pull.
	PollWait time.Duration
	// Workdir is where the harness runs.
	Workdir string
	// Env is passed to every harness invocation (e.g. MYCO_CONFIG).
	Env []string
	// Once processes at most one pull batch and returns.
	Once bool
	// Scope selects agent-level (default) or per-thread sessions.
	Scope SessionScope
	// BusyRetry caps the delay before retrying a message whose session was
	// busy; retries start at BusyRetryMin and double up to this (default 2m).
	BusyRetry time.Duration
	// BusyRetryMin is the first retry delay for a busy session (default 10s).
	BusyRetryMin time.Duration
	// BusyWindow: a session whose transcript changed within this window (and
	// not by us) is considered in use by someone else (default 90s).
	BusyWindow time.Duration
	// Slots, if set, is shared by several runners in one process to cap the
	// number of harness processes running at once.
	Slots chan struct{}
	// Now is the clock (tests).
	Now func() time.Time
	// StatePath, if set, persists when this runner last wrote each session,
	// so a restarted runner does not mistake its own writes for a human's.
	StatePath string
	// AttachmentsDir, if set, is where message attachments are downloaded
	// (<dir>/<message id>/<name>) before the harness runs.
	AttachmentsDir string
}

// Runner pulls messages and dispatches them to a harness.
type Runner struct {
	hub      Hub
	harness  harness.Harness
	sessions SessionStore
	prompt   PromptBuilder
	opts     Options
	log      *slog.Logger

	threadLocks sync.Map // thread_id -> *sync.Mutex

	mu         sync.Mutex
	ownRunEnd  map[string]time.Time // session id -> when our last run ended
	busyStreak map[string]int       // session key -> consecutive busy releases
}

// New builds a runner.
func New(hub Hub, h harness.Harness, sessions SessionStore, prompt PromptBuilder, opts Options, log *slog.Logger) *Runner {
	if opts.Concurrency <= 0 {
		opts.Concurrency = 1
	}
	if opts.PollWait <= 0 {
		opts.PollWait = 30 * time.Second
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 30 * time.Minute
	}
	if prompt == nil {
		prompt = DefaultPrompt{}
	}
	if opts.BusyRetry <= 0 {
		opts.BusyRetry = 2 * time.Minute
	}
	if opts.BusyRetryMin <= 0 {
		opts.BusyRetryMin = 10 * time.Second
	}
	if opts.BusyRetryMin > opts.BusyRetry {
		opts.BusyRetryMin = opts.BusyRetry
	}
	if opts.BusyWindow <= 0 {
		opts.BusyWindow = 90 * time.Second
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Scope == "" {
		opts.Scope = ScopeAgent
	}
	if opts.Scope == ScopeAgent && opts.Concurrency > 1 {
		// One session cannot run two turns at once.
		log.Warn("session scope 'agent' processes one message at a time; ignoring concurrency", "requested", opts.Concurrency)
		opts.Concurrency = 1
	}
	return &Runner{hub: hub, harness: h, sessions: sessions, prompt: prompt, opts: opts, log: log,
		ownRunEnd: loadState(opts.StatePath), busyStreak: map[string]int{}}
}

func loadState(path string) map[string]time.Time {
	out := map[string]time.Time{}
	if path == "" {
		return out
	}
	var raw map[string]int64
	if b, err := os.ReadFile(path); err == nil && json.Unmarshal(b, &raw) == nil {
		for k, v := range raw {
			out[k] = time.UnixMilli(v)
		}
	}
	return out
}

// recordOwnRun notes that this runner just wrote sessionID (caller holds mu).
func (r *Runner) recordOwnRun(sessionID string) {
	if sessionID == "" {
		return
	}
	r.ownRunEnd[sessionID] = r.opts.Now()
	if r.opts.StatePath == "" {
		return
	}
	raw := map[string]int64{}
	for k, v := range r.ownRunEnd {
		raw[k] = v.UnixMilli()
	}
	b, _ := json.Marshal(raw)
	_ = os.MkdirAll(filepath.Dir(r.opts.StatePath), 0o700)
	tmp := r.opts.StatePath + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Rename(tmp, r.opts.StatePath)
	}
}

// inUseElsewhere reports whether someone else wrote to the session recently
// (e.g. a human in the interactive UI), using the harness's transcript.
func (r *Runner) inUseElsewhere(key, sessionID string) (bool, time.Time) {
	p, ok := r.harness.(harness.BusyProber)
	if !ok || sessionID == "" {
		return false, time.Time{}
	}
	last, found := p.LastActivity(sessionID)
	if !found {
		return false, time.Time{}
	}
	r.mu.Lock()
	own := r.ownRunEnd[sessionID]
	r.mu.Unlock()
	recent := r.opts.Now().Sub(last) < r.opts.BusyWindow
	notUs := own.IsZero() || last.After(own.Add(2*time.Second))
	return recent && notUs, last
}

// busyDelay returns the next retry delay for key (exponential backoff).
func (r *Runner) busyDelay(key string) time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	d := r.opts.BusyRetryMin << min(r.busyStreak[key], 8)
	r.busyStreak[key]++
	return min(d, r.opts.BusyRetry)
}

func (r *Runner) release(ctx context.Context, log *slog.Logger, m api.Message, key, reason string) {
	delay := r.busyDelay(key)
	if _, err := r.hub.Release(ctx, m.ID, delay, false); err != nil {
		log.Error("releasing busy message failed", "err", err)
		return
	}
	log.Info("session busy; message released for retry", "retry_in", delay, "reason", reason)
}

// Run processes messages until ctx is cancelled (or after one batch if Once).
// In-flight work is allowed to finish before Run returns.
func (r *Runner) Run(ctx context.Context) error {
	slots := make(chan struct{}, r.opts.Concurrency)
	var wg sync.WaitGroup
	defer wg.Wait()
	backoff := time.Second
	for ctx.Err() == nil {
		// Wait for at least one free slot, then pull as many as are free.
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
			return nil
		}
		free := 1
		for free < cap(slots) {
			select {
			case slots <- struct{}{}:
				free++
				continue
			default:
			}
			break
		}
		wait := r.opts.PollWait
		if r.opts.Once {
			wait = 0
		}
		msgs, err := r.hub.Pull(ctx, free, wait)
		// Return unused slots.
		for i := len(msgs); i < free; i++ {
			<-slots
		}
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			r.log.Warn("pull failed", "err", err, "retry_in", backoff)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil
			}
			backoff = min(backoff*2, 30*time.Second)
			continue
		}
		backoff = time.Second
		for _, m := range msgs {
			wg.Add(1)
			go func(m api.Message) {
				defer wg.Done()
				defer func() { <-slots }()
				r.Handle(context.WithoutCancel(ctx), m)
			}(m)
		}
		if r.opts.Once {
			return nil
		}
	}
	return nil
}

// Handle processes one pulled message end to end.
func (r *Runner) Handle(ctx context.Context, m api.Message) {
	log := r.log.With("message_id", m.ID, "from", m.From, "type", m.Type, "thread_id", m.ThreadID)
	if err := r.hub.Ack(ctx, m.ID); err != nil {
		log.Warn("ack failed; skipping (will be redelivered if the lease expires)", "err", err)
		return
	}
	_, executor := r.harness.(harness.CommandExecutor)
	if executor && m.Type != "task" {
		// Exec nodes only run tasks; a note or reply is never executed.
		if err := r.hub.Complete(ctx, m.ID); err != nil {
			log.Error("finishing message failed", "err", err)
		}
		log.Info("exec node ignores non-task message")
		return
	}
	key := r.sessionKey(m)
	unlock := r.lockThread(key)
	defer unlock()

	sessionID := r.sessions.Get(key)
	if busy, last := r.inUseElsewhere(key, sessionID); busy {
		r.release(ctx, log, m, key, "session transcript changed "+r.opts.Now().Sub(last).Round(time.Second).String()+" ago by someone else")
		return
	}
	if r.opts.Slots != nil {
		select {
		case r.opts.Slots <- struct{}{}:
			defer func() { <-r.opts.Slots }()
		case <-ctx.Done():
			return
		}
	}

	var files []LocalFile
	var extraDirs []string
	if !executor { // exec nodes act on the body only; attachments stay on the hub
		files, extraDirs = r.downloadAttachments(ctx, log, m)
	}
	log.Info("running harness", "harness", r.harness.Name(), "attachments", len(files))
	start := time.Now()
	runCtx, cancel := context.WithTimeout(ctx, r.opts.Timeout)
	res, runErr := r.harness.Run(runCtx, harness.Request{
		Prompt:    r.prompt.Build(r.opts.Self, m, files),
		ExtraDirs: extraDirs,
		SessionID: sessionID,
		Workdir:   r.opts.Workdir,
		Body:      m.Body, From: m.From, Type: m.Type,
		Env: append(append([]string{}, r.opts.Env...),
			"MYCO_MESSAGE_ID="+m.ID, "MYCO_THREAD_ID="+m.ThreadID, "MYCO_FROM="+m.From, "MYCO_MESSAGE_TYPE="+m.Type),
	})
	cancel()
	r.mu.Lock()
	if !errors.Is(runErr, harness.ErrSessionBusy) {
		r.recordOwnRun(sessionID)
		r.recordOwnRun(res.SessionID)
		r.busyStreak[key] = 0
	}
	r.mu.Unlock()
	if err := r.sessions.Put(key, res.SessionID); err != nil {
		log.Warn("saving session failed", "err", err)
	}
	log = log.With("duration", time.Since(start).Round(time.Millisecond))

	// The agent may have replied/held/failed by itself via the CLI; only
	// auto-finish messages still in "acked".
	cur, err := r.hub.GetMessage(ctx, m.ID)
	if err != nil {
		log.Error("cannot read message state after run", "err", err)
		return
	}
	if cur.Status != "acked" {
		log.Info("agent handled the message itself", "status", cur.Status, "harness_error", errString(runErr))
		return
	}

	switch {
	case errors.Is(runErr, harness.ErrSessionBusy):
		// Someone is using the session; hand the message back without
		// counting a failure and try again soon.
		r.release(ctx, log, m, key, runErr.Error())
	case m.Type == "task" && runErr != nil:
		_, err = r.hub.Fail(ctx, m.ID, runErr.Error(), false)
		log.Warn("harness failed; task marked failed", "err", runErr)
	case m.Type == "task":
		out := strings.TrimSpace(res.Output)
		if out == "" {
			out = "(agent produced no output)"
		}
		_, err = r.hub.Reply(ctx, m.ID, out, false)
		log.Info("replied")
	default:
		// Replies, events and notes never trigger an automatic answer:
		// that would ping-pong forever between two runners.
		err = r.hub.Complete(ctx, m.ID)
		if runErr != nil {
			log.Warn("harness failed on non-task message", "err", runErr)
		} else {
			log.Info("processed")
		}
	}
	if err != nil {
		log.Error("finishing message failed", "err", err)
	}
}

// downloadAttachments fetches m's attachments into AttachmentsDir/<msg>.
func (r *Runner) downloadAttachments(ctx context.Context, log *slog.Logger, m api.Message) ([]LocalFile, []string) {
	if len(m.Attachments) == 0 {
		return nil, nil
	}
	if r.opts.AttachmentsDir == "" {
		out := make([]LocalFile, 0, len(m.Attachments))
		for _, a := range m.Attachments {
			out = append(out, LocalFile{Attachment: a, Error: "runner has no attachments directory"})
		}
		return out, nil
	}
	dir := filepath.Join(r.opts.AttachmentsDir, m.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		log.Warn("cannot create attachments dir", "dir", dir, "err", err)
	}
	out := make([]LocalFile, 0, len(m.Attachments))
	for _, a := range m.Attachments {
		_, path, err := r.hub.DownloadFile(ctx, a.FileID, dir)
		lf := LocalFile{Attachment: a, Path: path}
		if err != nil {
			lf.Error = err.Error()
			log.Warn("attachment download failed", "file_id", a.FileID, "err", err)
		}
		out = append(out, lf)
	}
	return out, []string{dir}
}

func (r *Runner) sessionKey(m api.Message) string {
	if r.opts.Scope == ScopeThread {
		return m.ThreadID
	}
	return "agent"
}

func (r *Runner) lockThread(thread string) func() {
	v, _ := r.threadLocks.LoadOrStore(thread, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

func errString(err error) string {
	if err == nil || errors.Is(err, context.Canceled) {
		return ""
	}
	return err.Error()
}
