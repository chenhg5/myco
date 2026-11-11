package runner

import (
	"context"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/harness"
)

type waitHarness struct{ started chan struct{} }

func (waitHarness) Name() string { return "wait" }
func (w waitHarness) Run(ctx context.Context, _ harness.Request) (harness.Result, error) {
	close(w.started)
	select {
	case <-ctx.Done():
		return harness.Result{}, context.Cause(ctx)
	case <-time.After(10 * time.Second):
		return harness.Result{Output: "finished"}, nil
	}
}

func TestRunEndsWhenTaskStopped(t *testing.T) {
	hub := newFakeHub(api.Message{ID: "t1", Type: "task", From: "alice", ThreadID: "th"})
	h := waitHarness{started: make(chan struct{})}
	go func() {
		<-h.started
		hub.set("t1", "failed") // the sender stopped it on the hub
	}()
	r := New(hub, h, NewHubSessions(&fakeBinder{}, ""), nil, Options{Once: true, Concurrency: 1}, quiet())
	r.stopEvery = 20 * time.Millisecond
	start := time.Now()
	r.Run(context.Background())
	if time.Since(start) > 5*time.Second {
		t.Fatal("the run was not ended when the task was stopped")
	}
	hub.mu.Lock()
	defer hub.mu.Unlock()
	if hub.replies["t1"] != "" || hub.fails["t1"] != "" {
		t.Fatalf("a stopped task gets no answer: reply %q, fail %q", hub.replies["t1"], hub.fails["t1"])
	}
}

type forkHarness struct {
	fakeHarness
}

func (forkHarness) CanFork() {}

// A copied node (session "fork:<source>:…") makes the copy in its first
// task's run, then resumes the copy.
func TestCopiedNodeForksOnFirstRun(t *testing.T) {
	marker := harness.ForkMarker("orig-session")
	hub := newFakeHub(
		api.Message{ID: "t1", Type: "task", From: "alice", ThreadID: "th1"},
		api.Message{ID: "t2", Type: "task", From: "alice", ThreadID: "th1"},
	)
	binder := &fakeBinder{}
	var runs []harness.Request
	h := forkHarness{fakeHarness{fn: func(r harness.Request) (harness.Result, error) {
		runs = append(runs, r)
		if r.Fork {
			return harness.Result{Output: "ok", SessionID: "copy-session"}, nil
		}
		return harness.Result{Output: "ok", SessionID: r.SessionID}, nil
	}}}
	r := New(hub, h, NewHubSessions(binder, marker), nil, Options{Once: true, Concurrency: 1}, quiet())
	r.Run(context.Background())
	r.Run(context.Background())
	if len(runs) != 2 || !runs[0].Fork || runs[0].SessionID != "orig-session" || runs[1].Fork || runs[1].SessionID != "copy-session" {
		t.Fatalf("runs: %+v", runs)
	}
	if len(binder.binds) == 0 || binder.binds[0] != "copy-session" {
		t.Fatalf("the copy was not bound: %v", binder.binds)
	}
}
