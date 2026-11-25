package integration_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/im"
)

// chatApp stands in for a chat app: it records what the bridge posts.
type chatApp struct {
	mu    sync.Mutex
	posts []string
}

func (a *chatApp) Run(ctx context.Context, _ func(im.Inbound)) error { <-ctx.Done(); return nil }
func (a *chatApp) add(s string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.posts = append(a.posts, s)
	return "m" + string(rune('0'+len(a.posts)))
}
func (a *chatApp) Post(_ context.Context, _ im.Ref, t string) (string, error)   { return a.add(t), nil }
func (a *chatApp) Status(_ context.Context, _ im.Ref, t string) (string, error) { return a.add(t), nil }
func (a *chatApp) Update(_ context.Context, _ im.Ref, _, t string) error        { a.add(t); return nil }
func (a *chatApp) Direct(_ context.Context, _, t string) error                  { a.add("direct: " + t); return nil }
func (a *chatApp) has(s string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, p := range a.posts {
		if strings.Contains(p, s) {
			return true
		}
	}
	return false
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A chat message becomes a task; the agent's progress, question and answer
// come back to the chat; a reply in the chat answers the question.
func TestIMBridgeRoundTrip(t *testing.T) {
	c := ctx(t)
	h := startHub(t, t.TempDir()+"/hub.db", policy(30*time.Second, 5))
	botTok := h.register(api.RegisterRequest{Name: "im-feishu", Binding: api.Binding{Harness: "inbox", SessionID: "im:feishu:feishu"}})
	coder := h.client(h.register(api.RegisterRequest{Name: "coder"}))

	app := &chatApp{}
	b, err := im.New(im.Config{Name: "feishu", Platform: "feishu", Owner: "ou_me", OwnerName: "glenn", Agent: "coder"},
		app, im.ClientHub{C: h.client(botTok)}, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	bctx, cancel := context.WithCancel(c)
	defer cancel()
	go func() { _ = b.Run(bctx) }()
	time.Sleep(50 * time.Millisecond)

	b.Handle(im.Inbound{ChatID: "oc_1", MessageID: "om_1", ThreadKey: "chat:oc_1", UserID: "ou_me", Text: "fix the build"})
	var task api.Message
	waitFor(t, "the task", func() bool {
		got, err := coder.Pull(c, 1, time.Second)
		if err == nil && len(got) == 1 {
			task = got[0]
			return true
		}
		return false
	})
	if task.From != "im-feishu" || !strings.HasPrefix(task.Body, "fix the build") || !strings.Contains(task.Body, "glenn") {
		t.Fatalf("task %+v", task)
	}
	if err := coder.Ack(c, task.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := coder.Progress(c, task.ID, "running the tests", false); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "progress in the chat", func() bool { return app.has("running the tests") })
	if _, _, err := coder.Ask(c, task.ID, "Which branch?\n1. main\n2. dev", nil, false); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the question in the chat", func() bool { return app.has("Which branch?") })

	b.Handle(im.Inbound{ChatID: "oc_1", MessageID: "om_2", ThreadKey: "chat:oc_1", UserID: "ou_me", Text: "main"})
	waitFor(t, "the answer reaches the agent", func() bool {
		st, err := coder.Steers(c, task.ID, true)
		return err == nil && len(st) == 1 && strings.HasPrefix(st[0].Body, "main")
	})

	if _, err := coder.Reply(c, task.ID, "Fixed on main in abc123", false); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the reply in the chat", func() bool { return app.has("Fixed on main in abc123") })

	// A note from another agent to the bot goes to its owner.
	if _, err := coder.Send(c, api.SendRequest{To: "im-feishu", Type: "note", Body: "deploy finished"}, false); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the note forwarded", func() bool { return app.has("direct: ") && app.has("deploy finished") })

	// The next chat message is a new task in the same thread.
	b.Handle(im.Inbound{ChatID: "oc_1", MessageID: "om_3", ThreadKey: "chat:oc_1", UserID: "ou_me", Text: "add a test"})
	waitFor(t, "the second task", func() bool {
		got, err := coder.Pull(c, 1, time.Second)
		if err == nil && len(got) == 1 {
			if got[0].ThreadID != task.ThreadID {
				t.Errorf("thread %q, want %q", got[0].ThreadID, task.ThreadID)
			}
			return true
		}
		return false
	})
}
