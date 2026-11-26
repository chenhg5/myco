package integration_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/harness"
	"github.com/myco-ai/myco/internal/runner"
)

func commandWatcher(t *testing.T, h *hub, dir string) (*client.Client, string) {
	t.Helper()
	token := h.register(api.RegisterRequest{Name: "command-watcher"})
	c := h.client(token)
	if _, err := c.Bind(ctx(t), api.Binding{Harness: "codex", SessionID: "demo-session", Environment: "demo-mac", Workdir: dir}); err != nil {
		t.Fatal(err)
	}
	if err := c.ReportMachine(ctx(t), api.MachineReport{Fingerprint: strings.Repeat("a", 64), Hostname: "demo-mac", OS: runtime.GOOS, Arch: runtime.GOARCH}); err != nil {
		t.Fatal(err)
	}
	commandRuntime(t, c, "runner-1")
	return c, token
}
func commandRuntime(t *testing.T, c *client.Client, id string) {
	t.Helper()
	if err := c.ReportRuntime(ctx(t), api.Runtime{CLI: "codex", Access: "full", MaxAccess: "full", Features: []string{"command_watch"}, RunnerInstance: id}); err != nil {
		t.Fatal(err)
	}
}

func TestWatchCommandHTTPHeldWakeAndRestart(t *testing.T) {
	c := ctx(t)
	db := filepath.Join(t.TempDir(), "hub.db")
	h := startHub(t, db, policy(time.Minute, 5))
	w, token := commandWatcher(t, h, t.TempDir())
	sender := h.client(h.register(api.RegisterRequest{Name: "requester"}))
	parent, err := sender.Send(c, api.SendRequest{To: "command-watcher", Body: "Watch the build"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.Pull(c, 10, 0); err != nil {
		t.Fatal(err)
	}
	watch, err := w.StartWatch(c, api.WatchStartRequest{What: "build ready", Kind: "command", Command: "echo ready", Task: parent.Message.ID}, false)
	if err != nil {
		t.Fatal(err)
	}
	stranger := h.client(h.register(api.RegisterRequest{Name: "stranger"}))
	for _, actor := range []*client.Client{stranger, h.admin()} {
		if _, err = actor.ClaimWatch(c, watch.ID, "runner-1"); err == nil {
			t.Fatal("noninitiating agent claimed command")
		}
		if _, err = actor.CheckWatch(c, watch.ID, api.WatchCheckRequest{Runner: "runner-1", CheckID: "forged", Matched: true}); err == nil {
			t.Fatal("noninitiating actor posted a result")
		}
	}
	claim, err := w.ClaimWatch(c, watch.ID, "runner-1")
	if err != nil {
		t.Fatal(err)
	}
	check := api.WatchCheckRequest{CheckID: claim.ClaimID, Runner: "runner-1", Result: "earlier\nready token=never-store-this", ExitCode: 0}
	ended, err := w.CheckWatch(c, watch.ID, check)
	if err != nil {
		t.Fatal(err)
	}
	if ended.State != "met" || ended.Checks != 1 || len(ended.History) != 1 || strings.Contains(ended.LastResult, "never-store-this") {
		t.Fatalf("unsafe/wrong result: %+v", ended)
	}
	if _, err = w.CheckWatch(c, watch.ID, check); err != nil {
		t.Fatal(err)
	}
	w.Via, w.WakeHeld = "runner", true
	batch, err := w.Pull(c, 10, 0)
	if err != nil || len(batch) != 1 || batch[0].ID != parent.Message.ID || batch[0].HoldWake == "" {
		t.Fatalf("single held wake: %+v %v", batch, err)
	}
	if err = w.Complete(c, parent.Message.ID); err != nil {
		t.Fatal(err)
	}
	h.stop()
	h = startHub(t, db, policy(time.Minute, 5))
	w = h.client(token)
	if _, err = w.CheckWatch(c, watch.ID, check); err != nil {
		t.Fatal(err)
	}
	batch, err = w.Pull(c, 10, 0)
	if err != nil || len(batch) != 0 {
		t.Fatalf("duplicate wake after restart: %+v %v", batch, err)
	}
	ended, err = w.Watch(c, watch.ID)
	if err != nil || ended.Checks != 1 {
		t.Fatalf("lost history/repeated check: %+v %v", ended, err)
	}
}

type watchHarness struct {
	calls atomic.Int32
	woke  chan struct{}
}

func (*watchHarness) Name() string { return "codex" }
func (h *watchHarness) Run(context.Context, harness.Request) (harness.Result, error) {
	if h.calls.Add(1) == 1 {
		close(h.woke)
	}
	return harness.Result{Output: "Watch checked", SessionID: "demo-session"}, nil
}

func TestWatchCommandRunnerContinuesAfterRestart(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), policy(time.Minute, 5))
	dir := t.TempDir()
	w, _ := commandWatcher(t, h, dir)
	file := filepath.Join(dir, "ready")
	command := "if test -f ready; then echo ready; else echo waiting; fi"
	every := 1
	if runtime.GOOS == "windows" {
		command = "if (Test-Path -LiteralPath 'ready') { Write-Output ready } else { Write-Output waiting }"
		every = 6
	}
	watch, err := w.StartWatch(c, api.WatchStartRequest{What: "ready file", Kind: "command", Command: command, Match: "^ready$", EverySec: every, ForSec: 60}, false)
	if err != nil {
		t.Fatal(err)
	}
	model := &watchHarness{woke: make(chan struct{})}
	start := func(id string) (context.CancelFunc, <-chan error) {
		runCtx, cancel := context.WithCancel(c)
		done := make(chan error, 1)
		r := runner.New(w, model, runner.NewMemorySessions(), nil, runner.Options{Self: "command-watcher", Workdir: dir, PollWait: 50 * time.Millisecond, Access: harness.AccessFull, MaxAccess: harness.AccessFull, CommandRunnerID: id, CommandEnvironment: "demo-mac", CommandWatchAllowed: func() bool { return true }}, slog.New(slog.NewTextHandler(io.Discard, nil)))
		go func() { defer close(done); done <- r.Run(runCtx) }()
		return cancel, done
	}
	stop, done := start("runner-1")
	defer func() { stop(); <-done }()
	deadline := time.Now().Add(30 * time.Second)
	for {
		got, e := w.Watch(c, watch.ID)
		if e != nil {
			t.Fatal(e)
		}
		if got.Checks > 0 {
			if got.State != "watching" {
				t.Fatalf("matched too early: %+v", got)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("runner never checked")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if model.calls.Load() != 0 {
		t.Fatal("model invoked for periodic checks")
	}
	stop()
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	commandRuntime(t, w, "runner-2")
	if err = os.WriteFile(file, []byte("ready"), 0600); err != nil {
		t.Fatal(err)
	}
	stop, done = start("runner-2")
	select {
	case <-model.woke:
	case <-time.After(30 * time.Second):
		t.Fatal("restarted runner did not wake agent")
	}
	got, err := w.Watch(c, watch.ID)
	if err != nil || got.State != "met" || got.Checks < 2 || got.LastResult != "ready" {
		t.Fatalf("watch after restart: %+v %v", got, err)
	}
	if model.calls.Load() != 1 {
		t.Fatalf("multiple wake runs: %d", model.calls.Load())
	}
}
