package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/harness"
)

func TestSessionLocks(t *testing.T) {
	dir := t.TempDir()
	a := NewSessionLocks(dir)
	unlock, ok := a.TryLock("claude:s1")
	if !ok {
		t.Fatal("a free session should lock")
	}
	if _, ok := a.TryLock("claude:s1"); ok {
		t.Fatal("the same session twice in one process")
	}
	if u, ok := a.TryLock("claude:s2"); !ok {
		t.Fatal("another session is free")
	} else {
		u()
	}
	// Another runner process (another lock set on the same directory).
	b := NewSessionLocks(dir)
	if _, ok := b.TryLock("claude:s1"); ok {
		t.Fatal("the same session from another process")
	}
	unlock()
	u, ok := b.TryLock("claude:s1")
	if !ok {
		t.Fatal("released: the other process takes it")
	}
	u()

	// A lock file left by a runner that died mid-turn is taken over once stale.
	c := &SessionLocks{Dir: dir, StaleAfter: time.Minute}
	u, ok = c.TryLock("codex:s3")
	if !ok {
		t.Fatal("lock")
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.lock"))
	if len(files) != 1 {
		t.Fatalf("lock files: %v", files)
	}
	old := time.Now().Add(-2 * time.Minute)
	os.Chtimes(files[0], old, old)
	d := &SessionLocks{Dir: dir, StaleAfter: time.Minute}
	if u2, ok := d.TryLock("codex:s3"); !ok {
		t.Fatal("a stale lock should be taken over")
	} else {
		u2()
	}
	u()

	// No locks configured: everything passes.
	var none *SessionLocks
	if u, ok := none.TryLock("claude:s1"); !ok {
		t.Fatal("nil locks")
	} else {
		u()
	}
}

type inUseHub struct {
	*fakeHub
	inUse []string
}

func (h *inUseHub) ReleaseInUse(_ context.Context, id string, _ time.Duration) (*api.Message, error) {
	h.inUse = append(h.inUse, id)
	return &api.Message{}, h.set(id, "delivered")
}

// A task for a session at work on another task of ours (here: held by
// another runner on this machine) waits in the queue; it is not shown as
// "open somewhere else". Once that turn ends, its transcript writes are
// known to be ours.
func TestOwnBusySessionQueuesNotInUse(t *testing.T) {
	now := time.Now()
	locks := NewSessionLocks(t.TempDir())
	h := probingHarness{fakeHarness: fakeHarness{fn: func(harness.Request) (harness.Result, error) {
		return harness.Result{Output: "ok", SessionID: "s1"}, nil
	}}, last: now}
	unlock, ok := locks.TryLock(h.Name() + ":s1")
	if !ok {
		t.Fatal("lock")
	}
	hub := &inUseHub{fakeHub: newFakeHub(api.Message{ID: "t1", Type: "task", From: "a", ThreadID: "th"})}
	New(hub, h, NewHubSessions(&fakeBinder{}, "s1"), nil, Options{Once: true, SessionLocks: locks}, quiet()).Run(context.Background())
	if hub.status["t1"] != "delivered" || len(hub.inUse) != 0 {
		t.Fatalf("busy with our own task: queued, not in use: status=%s inUse=%v", hub.status["t1"], hub.inUse)
	}
	// The other turn ends; the transcript was written just before.
	h.last = time.Now()
	time.Sleep(5 * time.Millisecond)
	unlock()
	hub2 := &inUseHub{fakeHub: newFakeHub(api.Message{ID: "t2", Type: "task", From: "a", ThreadID: "th"})}
	New(hub2, h, NewHubSessions(&fakeBinder{}, "s1"), nil, Options{Once: true, SessionLocks: locks}, quiet()).Run(context.Background())
	if hub2.replies["t2"] != "ok" || len(hub2.inUse) != 0 {
		t.Fatalf("after our own turn: runs: reply=%q inUse=%v", hub2.replies["t2"], hub2.inUse)
	}
}

// A task run again after a restart cut it short is not held back by the
// transcript its own first run wrote.
func TestRerunIgnoresItsOwnInterruptedWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runner.json")
	h := probingHarness{fakeHarness: fakeHarness{fn: func(harness.Request) (harness.Result, error) {
		return harness.Result{Output: "ok", SessionID: "s1"}, nil
	}}, last: time.Now()}
	hub := &inUseHub{fakeHub: newFakeHub(api.Message{ID: "t1", Type: "task", From: "a", ThreadID: "th"})}
	r := New(hub, h, NewHubSessions(&fakeBinder{}, "s1"), nil, Options{Self: "w", StatePath: path, Once: true}, quiet())
	r.markPending("t1") // the run a restart cut short
	hub.set("t1", "acked")
	r.Run(context.Background()) // puts it back, then runs it again
	if hub.replies["t1"] != "ok" || len(hub.inUse) != 0 {
		t.Fatalf("rerun: reply=%q inUse=%v status=%s", hub.replies["t1"], hub.inUse, hub.status["t1"])
	}
}
