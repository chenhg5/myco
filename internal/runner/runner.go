package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/harness"
)

// Options tunes the runner.
type Options struct {
	// Self is this agent's name (used in prompts).
	Self   string
	HubURL string // used in permanent credential rejection diagnostics
	// Trace says how much of a task's run is sent to the hub for the
	// sender's live view: TraceFull (default, ""), TraceSummary or TraceOff.
	Trace string
	// Concurrency is the max number of messages processed in parallel.
	Concurrency int
	// Timeout stops a run after this long without any activity (no output,
	// tool call or thinking step reported by the harness). A harness that
	// reports nothing is stopped this long after it started.
	Timeout time.Duration
	// MaxRun bounds a run however busy it is (default: 6h, at least Timeout).
	MaxRun time.Duration
	// PollWait is the long-poll duration per pull.
	PollWait time.Duration
	// Workdir is where the harness runs.
	Workdir string
	// Env is passed to every harness invocation (e.g. MYCO_CONFIG).
	Env []string
	// EnvFunc, when set, is called before every invocation and replaces Env,
	// so changes to the agent's env file apply without restarting.
	EnvFunc func() []string
	// Provider, when set, is called before every invocation and returns the
	// pinned model provider (nil = the machine's own setting).
	Provider func() *harness.ProviderSettings
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
	ownRunEnd  map[string]time.Time  // session id -> when our last run ended
	pending    map[string]pendingRun // acked tasks not yet durably finished
	busyStreak map[string]int        // session key -> consecutive busy releases
	// projectSeen: session key + project whose context this run already gave.
	projectSeen map[string]bool
	// tasks are the tasks this runner has accepted and not finished; steers
	// for them are routed here. pendingSteers hold steers for tasks that
	// were still queued when the steer arrived.
	tasks          map[string]*activeTask
	pendingSteers  map[string][]api.Message
	stopEvery      time.Duration // stopCheckEvery; shorter in tests
	finishRetryMin time.Duration // first retry after a post-run Hub failure; shorter in tests
	finishTimeout  time.Duration // total time to deliver a run's result
	idleCheck      time.Duration // how often a run's last activity is checked
}

// activeTask is a task between ack and completion in this runner.
type activeTask struct {
	steers []api.Message
	// cancel interrupts the harness run in progress (nil between runs).
	cancel context.CancelCauseFunc
}

// errIdle is the cancel cause of a run that reported no activity for the
// runner's timeout.
var errIdle = errors.New("no activity")

// errSteered is the cancel cause of a run interrupted by a steer.
var errSteered = errors.New("interrupted by a steer from the task's sender")

// errStopped is the cancel cause of a run whose task was closed on the hub
// while it ran (the sender stopped it).
var errStopped = errors.New("the task was stopped")

// stopCheckEvery is how often a running task's status is checked.
const stopCheckEvery = 3 * time.Second

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
	if opts.MaxRun <= 0 {
		opts.MaxRun = max(6*time.Hour, opts.Timeout)
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
		ownRunEnd: loadState(opts.StatePath), pending: loadPending(opts.StatePath), busyStreak: map[string]int{},
		tasks: map[string]*activeTask{}, pendingSteers: map[string][]api.Message{}, stopEvery: stopCheckEvery,
		finishRetryMin: time.Second, finishTimeout: 3 * time.Minute, idleCheck: 15 * time.Second}
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
	if b, err := os.ReadFile(path); err == nil {
		var st runnerState
		if json.Unmarshal(b, &st) == nil {
			for k, v := range st.Runs {
				out[k] = time.UnixMilli(v)
			}
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
	r.saveState()
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

// inUseReleaser is a Hub that can say why a message is handed back.
type inUseReleaser interface {
	ReleaseInUse(ctx context.Context, id string, delay time.Duration) (*api.Message, error)
}

func (r *Runner) release(ctx context.Context, log *slog.Logger, m api.Message, key, reason string) {
	finishCtx, cancel := r.finishContext(ctx)
	defer cancel()
	r.releaseWithContext(finishCtx, log, m, key, reason)
}

func (r *Runner) releaseWithContext(ctx context.Context, log *slog.Logger, m api.Message, key, reason string) {
	delay := r.busyDelay(key)
	err, exhausted := r.retryFinish(ctx, log, "release", func(ctx context.Context) error {
		if h, ok := r.hub.(inUseReleaser); ok {
			_, err := h.ReleaseInUse(ctx, m.ID, delay)
			return err
		}
		_, err := r.hub.Release(ctx, m.ID, delay, false)
		return err
	})
	if err != nil {
		r.logFinishFailure(log, "release", err, exhausted, "")
		return
	}
	log.Info("session busy; message released for retry", "retry_in", delay, "reason", reason)
	r.clearPending(m.ID)
}

// Run processes messages until ctx is cancelled (or after one batch if Once).
// In-flight work is allowed to finish before Run returns.
func (r *Runner) Run(ctx context.Context) error {
	r.recoverPending(ctx)
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
			if rejectedHubCredential(err) {
				r.log.Error("这台机器的凭据已被 Hub 拒绝，可能已从 Hub 移除；如需重新加入，请使用加入链接，或运行 myco runner service uninstall", "hub", r.opts.HubURL, "err", err)
				return err
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
	if m.Type == "task" {
		// Before the ack: an ack can reach the hub while its answer is lost
		// (network drop, the machine going to sleep). The task is then acked
		// and never redelivered, so a restart must still close it.
		r.markPending(m.ID)
	}
	ackCtx, cancel := r.finishContext(ctx)
	ackErr, unreachable := r.retryFinish(ackCtx, log, "ack", func(c context.Context) error { return r.hub.Ack(c, m.ID) })
	cancel()
	if ackErr != nil {
		if !unreachable && m.Type == "task" {
			r.clearPending(m.ID) // the hub refused it: it was not acked
		}
		log.Warn("ack failed; skipping", "err", ackErr, "hub_unreachable", unreachable)
		return
	}
	if m.Unreadable != "" {
		// End-to-end encrypted, and this agent cannot open it (keys changed,
		// or not sealed for it): never run it; tell the sender.
		log.Warn("cannot decrypt message; not running it", "why", m.Unreadable)
		if m.Type == "task" {
			if _, err := r.hub.Fail(ctx, m.ID, "could not decrypt the message: "+m.Unreadable, false); err != nil {
				log.Error("failing message failed", "err", err)
			} else {
				r.clearPending(m.ID)
			}
		} else if err := r.hub.Complete(ctx, m.ID); err != nil {
			log.Error("finishing message failed", "err", err)
		}
		return
	}
	_, executor := r.harness.(harness.CommandExecutor)
	if m.Type == "steer" && !executor && r.routeSteer(ctx, log, m) {
		return
	}
	if executor && m.Type != "task" {
		// Exec nodes only run tasks; a note, reply or steer is never executed.
		if err := r.hub.Complete(ctx, m.ID); err != nil {
			log.Error("finishing message failed", "err", err)
		}
		log.Info("exec node ignores non-task message")
		return
	}
	if m.Type == "task" && !executor {
		r.trackTask(m.ID)
		defer r.untrackTask(m.ID)
	}
	key := r.sessionKey(m)
	unlock := r.lockThread(key)
	defer unlock()

	sessionID := r.sessions.Get(key)
	// A copied node's first run makes the copy: it runs on the source
	// session with Fork, whatever else uses the source meanwhile.
	forkFrom := ""
	if src, ok := harness.ForkSource(sessionID); ok {
		forkFrom, sessionID = src, ""
		if _, ok := r.harness.(harness.Forker); !ok {
			if _, err := r.hub.Fail(ctx, m.ID, harness.ErrForkUnsupported.Error(), false); err != nil {
				log.Error("failing task failed", "err", err)
			} else {
				r.clearPending(m.ID)
			}
			return
		}
	}
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
	deadline := start.Add(r.opts.MaxRun)
	project := ""
	if !executor {
		project = r.projectContext(ctx, log, key, sessionID, m)
	}
	build := func() string { return withProject(r.prompt.Build(r.opts.Self, m, files), project) }
	prompt := build()
	// The run's steps, for the sender's live view in the app (tasks only;
	// never for end-to-end encrypted ones, whose content the hub must not see).
	var tr *tracer
	if up, ok := r.hub.(TraceUploader); ok && m.Type == "task" && !m.E2E && r.opts.Trace != TraceOff {
		mode := r.opts.Trace
		if mode == "" {
			mode = TraceFull
		}
		tr = newTracer(ctx, up, m.ID, mode, log)
		defer tr.close() // no-op once closed below
	}
	var (
		res    harness.Result
		runErr error
	)
	for first := true; ; first = false {
		// Steers that arrived before this run (task still queued, or the
		// previous run was interrupted by them).
		if steers := r.takeSteers(m.ID); len(steers) > 0 {
			section, dirs := r.steerSection(ctx, log, steers)
			extraDirs = append(extraDirs, dirs...)
			if first || sessionID == "" {
				prompt = build() + section
			} else {
				prompt = resumePrompt(m, section)
			}
		}
		runCtx, cancelCause := context.WithCancelCause(ctx)
		runCtx, cancelDeadline := context.WithDeadline(runCtx, deadline)
		r.setRunCancel(m.ID, cancelCause)
		watchCtx, stopWatch := context.WithCancel(runCtx)
		if m.Type == "task" && !executor {
			go r.watchSteers(watchCtx, log)
		}
		if m.Type == "task" {
			go r.watchStop(watchCtx, m.ID, cancelCause)
		}
		runSession, fork := sessionID, false
		if sessionID == "" && forkFrom != "" {
			runSession, fork = forkFrom, true
		}
		req := harness.Request{
			Prompt:    prompt,
			ExtraDirs: extraDirs,
			SessionID: runSession,
			Fork:      fork,
			Workdir:   r.opts.Workdir,
			Body:      m.Body, From: m.From, Type: m.Type,
			Env: append(r.env(),
				"MYCO_MESSAGE_ID="+m.ID, "MYCO_THREAD_ID="+m.ThreadID, "MYCO_FROM="+m.From, "MYCO_MESSAGE_TYPE="+m.Type,
				"MYCO_PROJECT="+m.Project),
		}
		// Every step the harness reports counts as activity: a long run that
		// keeps working is not cut off, a silent one is.
		var lastActive atomic.Int64
		lastActive.Store(time.Now().UnixNano())
		req.OnEvent = func(ev harness.Event) {
			lastActive.Store(time.Now().UnixNano())
			if tr != nil {
				tr.add(ev)
			}
		}
		go r.watchIdle(watchCtx, &lastActive, cancelCause)
		if r.opts.Provider != nil && !executor {
			if p := r.opts.Provider(); p != nil {
				if _, ok := r.harness.(harness.ProviderAware); ok {
					req.Provider = p
				} else {
					req.Env = append(p.EnvList(), req.Env...) // myco's own variables still win
				}
			}
		}
		res, runErr = r.harness.Run(runCtx, req)
		steered := errors.Is(context.Cause(runCtx), errSteered)
		switch cause := context.Cause(runCtx); {
		case runErr == nil:
		case errors.Is(cause, errIdle):
			runErr = fmt.Errorf("stopped after %s without any activity (runner --timeout)", r.opts.Timeout)
		case errors.Is(cause, context.DeadlineExceeded):
			runErr = fmt.Errorf("stopped: the run took longer than %s", r.opts.MaxRun)
		}
		stopWatch()
		cancelDeadline()
		cancelCause(nil)
		r.setRunCancel(m.ID, nil)
		if res.SessionID != "" && res.SessionID != forkFrom {
			sessionID = res.SessionID
			forkFrom = "" // copied: later runs resume the copy
		}
		if !steered || !time.Now().Before(deadline) {
			break
		}
		r.mu.Lock()
		r.recordOwnRun(sessionID)
		r.mu.Unlock()
		log.Info("task interrupted by a steer; resuming the session", "session_id", sessionID)
	}
	if tr != nil {
		tr.close() // the whole trace is on the hub before the task finishes
	}
	r.mu.Lock()
	if !errors.Is(runErr, harness.ErrSessionBusy) {
		r.recordOwnRun(sessionID)
		r.busyStreak[key] = 0
	}
	r.mu.Unlock()
	if err := r.sessions.Put(key, sessionID); err != nil {
		log.Warn("saving session failed", "err", err)
	}
	log = log.With("duration", time.Since(start).Round(time.Millisecond))

	// The agent may have replied/held/failed by itself via the CLI; only
	// auto-finish messages still in "acked".
	finishCtx, finishCancel := r.finishContext(ctx)
	defer finishCancel()
	var cur *api.Message
	err, exhausted := r.retryFinish(finishCtx, log, "get message", func(ctx context.Context) error {
		var getErr error
		cur, getErr = r.hub.GetMessage(ctx, m.ID)
		return getErr
	})
	if err != nil {
		r.logFinishFailure(log, "get message", err, exhausted, res.Output)
		return
	}
	if cur.Status != "acked" {
		r.clearPending(m.ID)
		if cur.Status == "failed" && cur.Error == "stopped by the sender" { // domain.StoppedBySender
			log.Info("task stopped by its sender")
			return
		}
		log.Info("agent handled the message itself", "status", cur.Status, "harness_error", errString(runErr))
		return
	}

	switch {
	case errors.Is(runErr, harness.ErrSessionBusy):
		// Someone is using the session; hand the message back without
		// counting a failure and try again soon.
		r.releaseWithContext(finishCtx, log, m, key, runErr.Error())
		return
	case m.Type == "task" && runErr != nil:
		err, exhausted = r.retryFinish(finishCtx, log, "fail", func(ctx context.Context) error {
			_, failErr := r.hub.Fail(ctx, m.ID, runErr.Error(), false)
			return failErr
		})
		if err == nil {
			log.Warn("harness failed; task marked failed", "err", runErr)
		}
	case m.Type == "task":
		out := strings.TrimSpace(res.Output)
		if out == "" {
			out = "(agent produced no output)"
		}
		err, exhausted = r.retryFinish(finishCtx, log, "reply", func(ctx context.Context) error {
			_, replyErr := r.hub.Reply(ctx, m.ID, out, false)
			return replyErr
		})
		if err == nil {
			log.Info("replied")
		}
	default:
		// Replies, events and notes never trigger an automatic answer:
		// that would ping-pong forever between two runners.
		err, exhausted = r.retryFinish(finishCtx, log, "complete", func(ctx context.Context) error {
			return r.hub.Complete(ctx, m.ID)
		})
		if runErr != nil {
			log.Warn("harness failed on non-task message", "err", runErr)
		} else {
			log.Info("processed")
		}
	}
	if err != nil {
		r.logFinishFailure(log, "finish message", err, exhausted, res.Output)
	} else {
		r.clearPending(m.ID)
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

// projectContext is the project section for m's prompt: once per session
// and project while this runner runs, and again in a new session (the
// agent has it in its context after that; `myco project show` has it all).
func (r *Runner) projectContext(ctx context.Context, log *slog.Logger, key, sessionID string, m api.Message) string {
	pr, ok := r.hub.(ProjectReader)
	if m.Project == "" || !ok {
		return ""
	}
	seen := key + "\x00" + m.Project
	r.mu.Lock()
	done := r.projectSeen[seen] && sessionID != ""
	r.mu.Unlock()
	if done {
		return ""
	}
	p, err := pr.GetProject(ctx, m.Project, false)
	if err != nil {
		log.Info("project not readable; prompt goes without it", "project", m.Project, "err", err)
		return ""
	}
	r.mu.Lock()
	if r.projectSeen == nil {
		r.projectSeen = map[string]bool{}
	}
	r.projectSeen[seen] = true
	r.mu.Unlock()
	return projectSection(r.opts.Self, p)
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

// --- steering -------------------------------------------------------------

func (r *Runner) trackTask(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tasks[id] = &activeTask{steers: r.pendingSteers[id]}
	delete(r.pendingSteers, id)
}

func (r *Runner) untrackTask(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tasks, id)
}

func (r *Runner) takeSteers(id string) []api.Message {
	r.mu.Lock()
	defer r.mu.Unlock()
	t := r.tasks[id]
	if t == nil {
		return nil
	}
	out := t.steers
	t.steers = nil
	return out
}

// setRunCancel registers the cancel func of the run in progress; a steer
// that arrived in between interrupts it right away.
func (r *Runner) setRunCancel(id string, cancel context.CancelCauseFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t := r.tasks[id]
	if t == nil {
		return
	}
	t.cancel = cancel
	if cancel != nil && len(t.steers) > 0 {
		cancel(errSteered)
	}
}

// routeSteer hands a steer to the task it corrects. It returns false when
// the task is open but not running here (e.g. held while the agent waits on
// others): then the steer is delivered to the session like a note.
func (r *Runner) routeSteer(ctx context.Context, log *slog.Logger, m api.Message) bool {
	done := func(msg string) bool {
		if err := r.hub.Complete(ctx, m.ID); err != nil {
			log.Error("finishing steer failed", "err", err)
		}
		log.Info(msg, "task_id", m.ReplyTo)
		return true
	}
	r.mu.Lock()
	if t := r.tasks[m.ReplyTo]; t != nil {
		t.steers = append(t.steers, m)
		cancel := t.cancel
		r.mu.Unlock()
		if cancel != nil {
			cancel(errSteered)
			return done("steer received; interrupting the running task")
		}
		return done("steer received; added to the task before it starts")
	}
	r.mu.Unlock()
	task, err := r.hub.GetMessage(ctx, m.ReplyTo)
	switch {
	case err != nil:
		log.Warn("cannot read the steered task; delivering the steer to the session", "err", err)
		return false
	case task.Status == "queued" || task.Status == "delivered":
		r.mu.Lock()
		if len(r.pendingSteers) < 1000 {
			r.pendingSteers[m.ReplyTo] = append(r.pendingSteers[m.ReplyTo], m)
		}
		r.mu.Unlock()
		return done("steer received; added to the task before it starts")
	case task.Status == "done" || task.Status == "failed":
		return done("steer for a finished task; dropped")
	}
	return false
}

// watchIdle ends the run when the harness reports nothing for the timeout.
func (r *Runner) watchIdle(ctx context.Context, last *atomic.Int64, cancel context.CancelCauseFunc) {
	t := time.NewTicker(min(r.idleCheck, r.opts.Timeout))
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if time.Since(time.Unix(0, last.Load())) >= r.opts.Timeout {
				cancel(errIdle)
				return
			}
		}
	}
}

// watchStop ends the run when its task is closed on the hub meanwhile (the
// sender stopped it from the app).
func (r *Runner) watchStop(ctx context.Context, id string, cancel context.CancelCauseFunc) {
	t := time.NewTicker(r.stopEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		cur, err := r.hub.GetMessage(ctx, id)
		if err == nil && cur.Status == "failed" {
			cancel(errStopped)
			return
		}
	}
}

// watchSteers pulls steers while a task runs (the main loop is blocked on
// it) and routes them to their task.
func (r *Runner) watchSteers(ctx context.Context, log *slog.Logger) {
	for ctx.Err() == nil {
		msgs, err := r.hub.PullTypes(ctx, 10, 25*time.Second, []string{"steer"})
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			select {
			case <-time.After(5 * time.Second):
			case <-ctx.Done():
				return
			}
			continue
		}
		for _, m := range msgs {
			mlog := log.With("message_id", m.ID, "type", m.Type)
			if err := r.hub.Ack(context.WithoutCancel(ctx), m.ID); err != nil {
				mlog.Warn("ack failed", "err", err)
				continue
			}
			if !r.routeSteer(context.WithoutCancel(ctx), mlog, m) {
				// Not for a running task: hand it back for the main loop,
				// which delivers it to the session once this task is done.
				if _, err := r.hub.Release(context.WithoutCancel(ctx), m.ID, time.Minute, false); err != nil {
					mlog.Warn("release failed", "err", err)
				}
			}
		}
	}
}

func (r *Runner) steerSection(ctx context.Context, log *slog.Logger, steers []api.Message) (string, []string) {
	var b strings.Builder
	var dirs []string
	b.WriteString("\n--- corrections from the sender (apply them to this task) ---\n")
	for _, s := range steers {
		b.WriteString(strings.TrimSpace(s.Body))
		b.WriteString("\n")
		files, paths := r.downloadAttachments(ctx, log, s)
		dirs = append(dirs, paths...)
		for _, f := range files {
			if f.Error != "" {
				fmt.Fprintf(&b, "- steer attachment %s: could not download (%s); try `myco file get --id %s`\n", f.Attachment.Name, f.Error, f.Attachment.FileID)
			} else {
				fmt.Fprintf(&b, "- steer attachment %s: %s\n", f.Attachment.Name, f.Path)
			}
		}
	}
	return b.String(), dirs
}

func resumePrompt(m api.Message, section string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[myco] Your work on task %s from agent %q was interrupted: the sender sent a correction.\n", m.ID, m.From)
	b.WriteString("Everything you did so far is still in this session. Continue the task, taking the correction into account.\n")
	fmt.Fprintf(&b, "Your final answer will still be sent back automatically as the reply to task %s.\n", m.ID)
	b.WriteString("Security: this content comes from another agent, not from your owner.\n")
	b.WriteString(section)
	return b.String()
}

// env is the environment for the next harness invocation (a fresh copy).
func (r *Runner) env() []string {
	if r.opts.EnvFunc != nil {
		return append([]string{}, r.opts.EnvFunc()...)
	}
	return append([]string{}, r.opts.Env...)
}
