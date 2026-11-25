package runner

import (
	"context"
	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/harness"
	"path/filepath"
	"testing"
	"time"
)

type shutdownHarness struct{ started, release, finished chan struct{} }

func (shutdownHarness) Name() string { return "shutdown-test" }
func (h shutdownHarness) Run(ctx context.Context, _ harness.Request) (harness.Result, error) {
	close(h.started)
	defer close(h.finished)
	if h.release != nil {
		<-h.release
	} else {
		<-ctx.Done()
	}
	return harness.Result{}, ctx.Err()
}

func TestShutdownCancelsAndRecoversInterruptedTask(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runner.json")
	m := api.Message{ID: "interrupted", Type: "task", ThreadID: "thread", From: "sender"}
	hub := newFakeHub(m)
	h := shutdownHarness{started: make(chan struct{}), finished: make(chan struct{})}
	r := New(hub, h, NewMemorySessions(), nil, Options{Self: "worker", StatePath: path, ShutdownTimeout: time.Second}, quiet())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()
	<-h.started
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown did not cancel work")
	}
	hub.mu.Lock()
	status, reply, failed := hub.status[m.ID], hub.replies[m.ID], hub.fails[m.ID]
	hub.mu.Unlock()
	if status != "acked" || reply != "" || failed != "" {
		t.Fatalf("stop finished task instead of keeping it: %s %q %q", status, reply, failed)
	}
	if p := loadPending(path); len(p) != 1 || p[m.ID].Finish != "" {
		t.Fatalf("interrupted task missing: %+v", p)
	}
	next := New(hub, fakeHarness{fn: func(harness.Request) (harness.Result, error) { return harness.Result{Output: "recovered"}, nil }}, NewMemorySessions(), nil, Options{Self: "worker", StatePath: path}, quiet())
	next.recoverPending(context.Background())
	if !next.pending[m.ID].Requeued {
		t.Fatal("new runner did not requeue interrupted task")
	}
	next.Handle(context.Background(), m)
	if len(loadPending(path)) != 0 {
		t.Fatal("replayed task not completed")
	}
}

type shutdownSessions struct {
	*MemorySessions
	saved chan struct{}
}

func (s shutdownSessions) Put(key, id string) error {
	err := s.MemorySessions.Put(key, id)
	close(s.saved)
	return err
}

func TestShutdownBoundsAnUnresponsiveHarness(t *testing.T) {
	h := shutdownHarness{started: make(chan struct{}), release: make(chan struct{}), finished: make(chan struct{})}
	sessions := shutdownSessions{NewMemorySessions(), make(chan struct{})}
	r := New(newFakeHub(api.Message{ID: "t", Type: "task"}), h, sessions, nil, Options{StatePath: filepath.Join(t.TempDir(), "state.json"), ShutdownTimeout: 20 * time.Millisecond}, quiet())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()
	<-h.started
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("waited for unresponsive harness")
	}
	if len(loadPending(r.opts.StatePath)) != 1 {
		t.Fatal("lost interrupted task")
	}
	close(h.release)
	<-h.finished
	<-sessions.saved // all local state writes precede SessionStore.Put
}

type cancelledRecoveryHub struct {
	*fakeHub
	started chan struct{}
}

func (h cancelledRecoveryHub) GetMessage(ctx context.Context, _ string) (*api.Message, error) {
	close(h.started)
	<-ctx.Done()
	return nil, ctx.Err()
}
func TestShutdownCancelsStartupRecovery(t *testing.T) {
	h := cancelledRecoveryHub{newFakeHub(), make(chan struct{})}
	r := New(h, fakeHarness{}, NewMemorySessions(), nil, Options{StatePath: filepath.Join(t.TempDir(), "state.json")}, quiet())
	r.markPending("previous")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()
	<-h.started
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("recovery delayed shutdown")
	}
	if len(loadPending(r.opts.StatePath)) != 1 {
		t.Fatal("lost task on cancelled recovery")
	}
}

type persistedResultHub struct {
	*fakeHub
	path string
	t    *testing.T
}

func (h persistedResultHub) GetMessage(ctx context.Context, id string) (*api.Message, error) {
	if p := loadPending(h.path)[id]; p.Finish != "reply" || p.Text != "complete" {
		h.t.Errorf("completed result was not saved before delivery: %+v", p)
	}
	return h.fakeHub.GetMessage(ctx, id)
}
func TestCompletedResultIsSavedBeforeNetworkDelivery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	m := api.Message{ID: "result", Type: "task"}
	hub := persistedResultHub{newFakeHub(m), path, t}
	h := fakeHarness{fn: func(harness.Request) (harness.Result, error) { return harness.Result{Output: "complete"}, nil }}
	r := New(hub, h, NewMemorySessions(), nil, Options{StatePath: path}, quiet())
	r.Handle(context.Background(), m)
	if len(loadPending(path)) != 0 {
		t.Fatal("delivered result not cleared")
	}
}
