package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/harness"
)

// fakeHub records calls and serves a fixed queue.
type fakeHub struct {
	mu      sync.Mutex
	queue   []api.Message
	status  map[string]string
	replies map[string]string
	fails   map[string]string
}

func newFakeHub(msgs ...api.Message) *fakeHub {
	h := &fakeHub{queue: msgs, status: map[string]string{}, replies: map[string]string{}, fails: map[string]string{}}
	for _, m := range msgs {
		h.status[m.ID] = "delivered"
	}
	return h
}

func (h *fakeHub) Pull(ctx context.Context, limit int, wait time.Duration) ([]api.Message, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := min(limit, len(h.queue))
	out := h.queue[:n]
	h.queue = h.queue[n:]
	return out, nil
}
func (h *fakeHub) PullTypes(ctx context.Context, _ int, wait time.Duration, _ []string) ([]api.Message, error) {
	select { // no steers in these tests
	case <-time.After(wait):
	case <-ctx.Done():
	}
	return nil, nil
}
func (h *fakeHub) set(id, s string) error                      { h.mu.Lock(); h.status[id] = s; h.mu.Unlock(); return nil }
func (h *fakeHub) Ack(_ context.Context, id string) error      { return h.set(id, "acked") }
func (h *fakeHub) Complete(_ context.Context, id string) error { return h.set(id, "done") }
func (h *fakeHub) Reply(_ context.Context, id, body string, _ bool) (*api.Message, error) {
	h.mu.Lock()
	h.replies[id], h.status[id] = body, "done"
	h.mu.Unlock()
	return &api.Message{}, nil
}
func (h *fakeHub) Fail(_ context.Context, id, reason string, _ bool) (*api.Message, error) {
	h.mu.Lock()
	h.fails[id], h.status[id] = reason, "failed"
	h.mu.Unlock()
	return &api.Message{}, nil
}
func (h *fakeHub) Release(_ context.Context, id string, _ time.Duration, _ bool) (*api.Message, error) {
	return &api.Message{}, h.set(id, "delivered")
}
func (h *fakeHub) DownloadFile(_ context.Context, id, dest string) (*api.File, string, error) {
	p := filepath.Join(dest, id+".txt")
	return &api.File{ID: id}, p, os.WriteFile(p, []byte("content of "+id), 0o600)
}
func (h *fakeHub) GetMessage(_ context.Context, id string) (*api.Message, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return &api.Message{ID: id, Status: h.status[id]}, nil
}

type fakeHarness struct {
	fn func(harness.Request) (harness.Result, error)
}

func (f fakeHarness) Name() string { return "fake" }
func (f fakeHarness) Run(_ context.Context, r harness.Request) (harness.Result, error) {
	return f.fn(r)
}

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestRunnerRepliesFailsAndNeverAnswersReplies(t *testing.T) {
	hub := newFakeHub(
		api.Message{ID: "t1", Type: "task", From: "alice", ThreadID: "th1", Body: "ok"},
		api.Message{ID: "t2", Type: "task", From: "alice", ThreadID: "th2", Body: "boom"},
		api.Message{ID: "r1", Type: "reply", From: "bob", ThreadID: "th1", Body: "result"},
	)
	sessions := NewMemorySessions()
	var seenSession string
	h := fakeHarness{fn: func(r harness.Request) (harness.Result, error) {
		switch {
		case r.SessionID != "":
			seenSession = r.SessionID
			return harness.Result{Output: "noted", SessionID: r.SessionID}, nil
		case strings.Contains(r.Prompt, "boom"):
			return harness.Result{}, errors.New("harness crashed")
		}
		return harness.Result{Output: "done!", SessionID: "sess-th1"}, nil
	}}
	r := New(hub, h, sessions, nil, Options{Self: "me", Once: true, Scope: ScopeThread}, quiet())
	for range 3 {
		if err := r.Run(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if hub.replies["t1"] != "done!" {
		t.Errorf("task t1 should be auto-replied, got %q", hub.replies["t1"])
	}
	if hub.fails["t2"] == "" {
		t.Error("task t2 should be failed")
	}
	if _, replied := hub.replies["r1"]; replied || hub.status["r1"] != "done" {
		t.Errorf("reply r1 must be completed, not answered: status=%s", hub.status["r1"])
	}
	if seenSession != "sess-th1" {
		t.Errorf("follow-up in thread th1 should resume session, got %q", seenSession)
	}
}

func TestRunnerRespectsAgentHandledMessages(t *testing.T) {
	hub := newFakeHub(api.Message{ID: "t1", Type: "task", From: "alice", ThreadID: "th"})
	h := fakeHarness{fn: func(r harness.Request) (harness.Result, error) {
		_ = hub.set("t1", "held") // agent ran `myco message hold`
		return harness.Result{Output: "will answer later"}, nil
	}}
	New(hub, h, NewMemorySessions(), nil, Options{Once: true}, quiet()).Run(context.Background())
	if _, ok := hub.replies["t1"]; ok || hub.status["t1"] != "held" {
		t.Fatalf("held task must not be auto-replied: %v", hub.status["t1"])
	}
}

type fakeBinder struct{ binds []string }

func (f *fakeBinder) Bind(_ context.Context, b api.Binding) (*api.Agent, error) {
	f.binds = append(f.binds, b.SessionID)
	return &api.Agent{}, nil
}

func TestAgentScopeResumesOneBoundSession(t *testing.T) {
	hub := newFakeHub(
		api.Message{ID: "t1", Type: "task", From: "alice", ThreadID: "th1"},
		api.Message{ID: "t2", Type: "task", From: "bob", ThreadID: "th2"},
	)
	binder := &fakeBinder{}
	var seen []string
	h := fakeHarness{fn: func(r harness.Request) (harness.Result, error) {
		seen = append(seen, r.SessionID)
		sid := r.SessionID
		if sid == "" {
			sid = "agent-session"
		}
		return harness.Result{Output: "ok", SessionID: sid}, nil
	}}
	r := New(hub, h, NewHubSessions(binder, ""), nil, Options{Once: true, Concurrency: 4}, quiet())
	r.Run(context.Background())
	r.Run(context.Background())
	if len(seen) != 2 || seen[0] != "" || seen[1] != "agent-session" {
		t.Fatalf("second message (other thread) must resume the agent session: %v", seen)
	}
	if len(binder.binds) != 1 || binder.binds[0] != "agent-session" {
		t.Fatalf("new session must be bound on the hub exactly once: %v", binder.binds)
	}
}

func TestBusySessionReleasesInsteadOfFailing(t *testing.T) {
	hub := newFakeHub(api.Message{ID: "t1", Type: "task", From: "alice", ThreadID: "th"})
	h := fakeHarness{fn: func(r harness.Request) (harness.Result, error) {
		return harness.Result{}, fmt.Errorf("%w: open in UI", harness.ErrSessionBusy)
	}}
	New(hub, h, NewMemorySessions(), nil, Options{Once: true}, quiet()).Run(context.Background())
	if hub.fails["t1"] != "" || hub.status["t1"] != "delivered" {
		t.Fatalf("busy session must release, not fail: status=%s fail=%q", hub.status["t1"], hub.fails["t1"])
	}
}

type probingHarness struct {
	fakeHarness
	last time.Time
}

func (p probingHarness) LastActivity(string) (time.Time, bool) { return p.last, true }

func TestTranscriptActivityDefersAndBacksOff(t *testing.T) {
	now := time.Unix(1000, 0)
	hub := newFakeHub(api.Message{ID: "t1", Type: "task", From: "alice", ThreadID: "th"})
	ran := false
	h := probingHarness{fakeHarness: fakeHarness{fn: func(harness.Request) (harness.Result, error) {
		ran = true
		return harness.Result{Output: "ok"}, nil
	}}, last: now.Add(-5 * time.Second)} // a human typed 5s ago
	r := New(hub, h, NewHubSessions(&fakeBinder{}, "sess-1"), nil, Options{Once: true, Now: func() time.Time { return now }}, quiet())
	r.Run(context.Background())
	if ran || hub.status["t1"] != "delivered" {
		t.Fatalf("must not run in a session used 5s ago: ran=%v status=%s", ran, hub.status["t1"])
	}
	if d1, d2 := r.busyDelay("agent"), r.busyDelay("agent"); d1 != 20*time.Second || d2 != 40*time.Second {
		t.Fatalf("backoff should double from 10s: %v %v", d1, d2)
	}

	// Activity older than the window: runs.
	hub2 := newFakeHub(api.Message{ID: "t2", Type: "task", From: "alice", ThreadID: "th"})
	h.last = now.Add(-10 * time.Minute)
	New(hub2, h, NewHubSessions(&fakeBinder{}, "sess-1"), nil, Options{Once: true, Now: func() time.Time { return now }}, quiet()).Run(context.Background())
	if !ran || hub2.replies["t2"] != "ok" {
		t.Fatalf("idle session should run: ran=%v reply=%q", ran, hub2.replies["t2"])
	}
}

func TestOwnWritesSurviveRestart(t *testing.T) {
	now := time.Unix(5000, 0)
	state := filepath.Join(t.TempDir(), "state.json")
	h := probingHarness{fakeHarness: fakeHarness{fn: func(harness.Request) (harness.Result, error) {
		return harness.Result{Output: "ok", SessionID: "s1"}, nil
	}}}
	// First runner process writes the session at `now`.
	hub := newFakeHub(api.Message{ID: "t1", Type: "task", From: "a", ThreadID: "th"})
	h.last = now.Add(-time.Hour)
	New(hub, h, NewHubSessions(&fakeBinder{}, "s1"), nil, Options{Once: true, StatePath: state, Now: func() time.Time { return now }}, quiet()).Run(context.Background())
	// Restarted runner, 5s later; the transcript changed at `now` (our write).
	h.last = now
	later := now.Add(5 * time.Second)
	hub2 := newFakeHub(api.Message{ID: "t2", Type: "task", From: "a", ThreadID: "th"})
	New(hub2, h, NewHubSessions(&fakeBinder{}, "s1"), nil, Options{Once: true, StatePath: state, Now: func() time.Time { return later }}, quiet()).Run(context.Background())
	if hub2.replies["t2"] != "ok" {
		t.Fatalf("own recent write must not count as busy after restart: status=%s", hub2.status["t2"])
	}
}

func TestAttachmentsDownloadedAndInPrompt(t *testing.T) {
	dir := t.TempDir()
	hub := newFakeHub(api.Message{ID: "t1", Type: "task", From: "a", ThreadID: "th", Body: "see file",
		Attachments: []api.Attachment{{FileID: "f_1", Name: "spec.md", Size: 12}}})
	var gotPrompt string
	var gotDirs []string
	h := fakeHarness{fn: func(r harness.Request) (harness.Result, error) {
		gotPrompt, gotDirs = r.Prompt, r.ExtraDirs
		return harness.Result{Output: "ok"}, nil
	}}
	New(hub, h, NewMemorySessions(), nil, Options{Once: true, AttachmentsDir: dir}, quiet()).Run(context.Background())
	want := filepath.Join(dir, "t1", "f_1.txt")
	if !strings.Contains(gotPrompt, "spec.md (12 bytes): "+want) || len(gotDirs) != 1 || gotDirs[0] != filepath.Join(dir, "t1") {
		t.Fatalf("prompt/dirs: %q %v", gotPrompt, gotDirs)
	}
	if b, err := os.ReadFile(want); err != nil || string(b) != "content of f_1" {
		t.Fatalf("download: %q %v", b, err)
	}
}
