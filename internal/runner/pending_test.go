package runner

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/harness"
)

func TestRunnerRecoversPendingTasks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runner.json")
	hub := newFakeHub(api.Message{ID: "lost", Type: "task"}, api.Message{ID: "already-done", Type: "task"})
	hub.set("lost", "acked")
	hub.set("already-done", "done")
	hub.queue = nil
	r := New(hub, fakeHarness{fn: func(harness.Request) (harness.Result, error) { return harness.Result{}, nil }}, NewMemorySessions(), nil, Options{Self: "worker", StatePath: path, Once: true}, quiet())
	r.markPending("lost")
	r.markPending("already-done")
	restarted := New(hub, r.harness, NewMemorySessions(), nil, r.opts, quiet())
	if len(restarted.pending) != 2 {
		t.Fatalf("lost persisted tasks: %+v", restarted.pending)
	}
	if err := restarted.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(hub.fails["lost"], "result was lost") || hub.status["already-done"] != "done" || len(restarted.pending) != 0 {
		t.Fatalf("recovery: failed=%q done=%q pending=%v", hub.fails["lost"], hub.status["already-done"], restarted.pending)
	}
}

func TestRunnerRecordsTaskDuringRunAndClearsOnReply(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runner.json")
	m := api.Message{ID: "task-1", Type: "task", From: "sender", ThreadID: "thread"}
	hub := newFakeHub(m)
	h := fakeHarness{fn: func(harness.Request) (harness.Result, error) {
		if got := loadPending(path); got[m.ID].Agent != "worker" {
			t.Fatalf("task not recorded during run: %v", got)
		}
		return harness.Result{Output: "done"}, nil
	}}
	r := New(hub, h, NewMemorySessions(), nil, Options{Self: "worker", StatePath: path}, quiet())
	r.Handle(context.Background(), m)
	if len(loadPending(path)) != 0 {
		t.Fatal("finished task still in state file")
	}
}
