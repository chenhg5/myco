package runner

import (
	"context"
	"errors"
	"log/slog"
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
	// BusyRetry is how long to wait before retrying a message whose session
	// was busy (default 2m).
	BusyRetry time.Duration
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
	if opts.Scope == "" {
		opts.Scope = ScopeAgent
	}
	if opts.Scope == ScopeAgent && opts.Concurrency > 1 {
		// One session cannot run two turns at once.
		log.Warn("session scope 'agent' processes one message at a time; ignoring concurrency", "requested", opts.Concurrency)
		opts.Concurrency = 1
	}
	return &Runner{hub: hub, harness: h, sessions: sessions, prompt: prompt, opts: opts, log: log}
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
	key := r.sessionKey(m)
	unlock := r.lockThread(key)
	defer unlock()

	log.Info("running harness", "harness", r.harness.Name())
	start := time.Now()
	runCtx, cancel := context.WithTimeout(ctx, r.opts.Timeout)
	res, runErr := r.harness.Run(runCtx, harness.Request{
		Prompt:    r.prompt.Build(r.opts.Self, m),
		SessionID: r.sessions.Get(key),
		Workdir:   r.opts.Workdir,
		Env: append(append([]string{}, r.opts.Env...),
			"MYCO_MESSAGE_ID="+m.ID, "MYCO_THREAD_ID="+m.ThreadID, "MYCO_FROM="+m.From, "MYCO_MESSAGE_TYPE="+m.Type),
	})
	cancel()
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
		// counting a failure and try again later.
		_, err = r.hub.Release(ctx, m.ID, r.opts.BusyRetry, false)
		log.Info("session busy; message released for retry", "retry_in", r.opts.BusyRetry, "reason", runErr)
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
