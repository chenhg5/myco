package runner

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/harness"
)

// ctxHarness runs fn with the run's context.
type ctxHarness struct {
	fn func(context.Context, harness.Request) (harness.Result, error)
}

func (h ctxHarness) Name() string { return "fake" }
func (h ctxHarness) Run(ctx context.Context, r harness.Request) (harness.Result, error) {
	return h.fn(ctx, r)
}

// A run that keeps reporting steps is not cut off at the timeout.
func TestBusyRunOutlastsTheTimeout(t *testing.T) {
	m := api.Message{ID: "t1", Type: "task", From: "alice", ThreadID: "th"}
	hub := newFakeHub(m)
	h := ctxHarness{fn: func(ctx context.Context, r harness.Request) (harness.Result, error) {
		for i := 0; i < 20; i++ {
			select {
			case <-ctx.Done():
				return harness.Result{}, ctx.Err()
			case <-time.After(10 * time.Millisecond):
			}
			r.OnEvent(harness.Event{Kind: "tool", Tool: "Bash"})
		}
		return harness.Result{Output: "finished"}, nil
	}}
	r := New(hub, h, NewMemorySessions(), nil, Options{Timeout: 60 * time.Millisecond, Trace: TraceOff}, quiet())
	r.idleCheck = 5 * time.Millisecond
	r.Handle(context.Background(), m)
	if hub.replies[m.ID] != "finished" {
		t.Fatalf("a busy run was stopped: reply=%q fail=%q", hub.replies[m.ID], hub.fails[m.ID])
	}
}

// A run that reports nothing for the timeout is stopped, and says why.
func TestSilentRunIsStopped(t *testing.T) {
	m := api.Message{ID: "t1", Type: "task", From: "alice", ThreadID: "th"}
	hub := newFakeHub(m)
	h := ctxHarness{fn: func(ctx context.Context, r harness.Request) (harness.Result, error) {
		<-ctx.Done()
		return harness.Result{}, ctx.Err()
	}}
	r := New(hub, h, NewMemorySessions(), nil, Options{Timeout: 40 * time.Millisecond}, quiet())
	r.idleCheck = 5 * time.Millisecond
	start := time.Now()
	r.Handle(context.Background(), m)
	if time.Since(start) > 2*time.Second {
		t.Fatal("the silent run was not stopped")
	}
	if !strings.Contains(hub.fails[m.ID], "without any activity") {
		t.Fatalf("fail reason: %q", hub.fails[m.ID])
	}
}
