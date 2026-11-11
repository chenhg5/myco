package runner

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/harness"
)

// TraceUploader sends a task's run steps to the hub (the live view in the
// app). A hub client has it; hubs without traces answer not found.
type TraceUploader interface {
	PostTrace(ctx context.Context, id string, events []api.TraceEvent) error
}

// Trace modes (Options.Trace).
const (
	TraceFull    = "full"    // every step with its details
	TraceSummary = "summary" // which tools ran, without arguments, output or thoughts
	TraceOff     = "off"
)

// traceEvery is how often a run's new steps are uploaded.
const traceEvery = 700 * time.Millisecond

// Per-field bounds, so one step stays well under the hub's limit.
const (
	traceMaxText   = 8000
	traceMaxInput  = 1000
	traceMaxOutput = 4000
)

// tracer collects a run's steps and uploads them in batches.
type tracer struct {
	up    TraceUploader
	ctx   context.Context
	id    string
	mode  string
	log   *slog.Logger
	mu    sync.Mutex
	queue []api.TraceEvent
	seq   int
	fails int
	off   bool
	stop  chan struct{}
	done  chan struct{}
	once  sync.Once
}

func newTracer(ctx context.Context, up TraceUploader, id, mode string, log *slog.Logger) *tracer {
	t := &tracer{up: up, ctx: ctx, id: id, mode: mode, log: log, stop: make(chan struct{}), done: make(chan struct{})}
	go t.loop()
	return t
}

// add records a step (harness.Request.OnEvent).
func (t *tracer) add(ev harness.Event) {
	if t.mode == TraceSummary {
		ev.Input, ev.Output = "", ""
		if ev.Kind == "thinking" || ev.Kind == "text" {
			ev.Text = ""
		}
	}
	ev.Text = clip(ev.Text, traceMaxText)
	ev.Input = clip(ev.Input, traceMaxInput)
	ev.Output = clip(ev.Output, traceMaxOutput)
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.off {
		return
	}
	t.seq++
	t.queue = append(t.queue, api.TraceEvent{Seq: t.seq, Kind: ev.Kind, Data: data, At: time.Now()})
}

func (t *tracer) loop() {
	defer close(t.done)
	tick := time.NewTicker(traceEvery)
	defer tick.Stop()
	for {
		select {
		case <-t.stop:
			t.flush()
			return
		case <-tick.C:
			t.flush()
		}
	}
}

func (t *tracer) flush() {
	t.mu.Lock()
	batch := t.queue
	t.queue = nil
	off := t.off
	t.mu.Unlock()
	if off || len(batch) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(t.ctx), 10*time.Second)
	defer cancel()
	err := t.up.PostTrace(ctx, t.id, batch)
	t.mu.Lock()
	defer t.mu.Unlock()
	if err == nil {
		t.fails = 0
		return
	}
	// A hub without traces, a full trace, or a network that keeps failing:
	// stop trying; the task itself is unaffected.
	t.fails++
	if t.fails >= 2 {
		t.off = true
		t.log.Info("run trace not uploaded; continuing without it", "err", err)
	}
}

// close uploads what is left and stops.
func (t *tracer) close() {
	t.once.Do(func() { close(t.stop) })
	<-t.done
}

// clip shortens s to at most n bytes on a character boundary.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n] + "\n… (cut short)"
}
