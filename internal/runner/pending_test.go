package runner

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/harness"
)

func TestRunnerRequeuesAnInterruptedTaskOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runner.json")
	hub := newFakeHub(api.Message{ID: "lost", Type: "task"}, api.Message{ID: "already-done", Type: "task"})
	hub.set("lost", "acked")
	hub.set("already-done", "done")
	hub.queue = nil
	var prompts []string
	h := fakeHarness{fn: func(req harness.Request) (harness.Result, error) {
		prompts = append(prompts, req.Prompt)
		return harness.Result{Output: "ok"}, nil
	}}
	r := New(hub, h, NewMemorySessions(), nil, Options{Self: "worker", StatePath: path, Once: true}, quiet())
	r.markPending("lost")
	r.markPending("already-done")
	restarted := New(hub, r.harness, NewMemorySessions(), nil, r.opts, quiet())
	if len(restarted.pending) != 2 {
		t.Fatalf("lost persisted tasks: %+v", restarted.pending)
	}
	restarted.recoverPending(context.Background())
	if hub.status["lost"] != "delivered" || hub.fails["lost"] != "" || !restarted.pending["lost"].Requeued {
		t.Fatalf("interrupted task not put back: status=%q fail=%q pending=%+v", hub.status["lost"], hub.fails["lost"], restarted.pending)
	}
	if _, ok := restarted.pending["already-done"]; ok || hub.status["already-done"] != "done" {
		t.Fatalf("finished task touched: %+v", restarted.pending)
	}
	// It runs again, told that it was cut short.
	restarted.Handle(context.Background(), api.Message{ID: "lost", Type: "task", From: "sender", ThreadID: "lost"})
	if len(prompts) != 1 || !strings.Contains(prompts[0], "runner restarted") || len(loadPending(path)) != 0 {
		t.Fatalf("rerun: prompts=%q pending=%v", prompts, loadPending(path))
	}

	// Cut short a second time: it fails.
	hub.set("lost", "acked")
	restarted.markPending("lost")
	restarted.markRequeued("lost")
	again := New(hub, h, NewMemorySessions(), nil, r.opts, quiet())
	again.recoverPending(context.Background())
	if !strings.Contains(hub.fails["lost"], "restarted twice") || len(again.pending) != 0 {
		t.Fatalf("second interruption: fail=%q pending=%v", hub.fails["lost"], again.pending)
	}
}

func TestRunnerFailsAnInterruptedCommand(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runner.json")
	hub := newFakeHub(api.Message{ID: "cmd", Type: "task"})
	hub.set("cmd", "acked")
	hub.queue = nil
	r := New(hub, fakeExecutor{}, NewMemorySessions(), nil, Options{Self: "shell", StatePath: path, Once: true}, quiet())
	r.markPending("cmd")
	restarted := New(hub, r.harness, NewMemorySessions(), nil, r.opts, quiet())
	restarted.recoverPending(context.Background())
	if !strings.Contains(hub.fails["cmd"], "may have finished in part") || len(restarted.pending) != 0 {
		t.Fatalf("command: fail=%q pending=%v", hub.fails["cmd"], restarted.pending)
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

type fakeExecutor struct{ fakeHarness }

func (fakeExecutor) ExecutesBody() bool { return true }
