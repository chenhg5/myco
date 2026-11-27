package im

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
)

type post struct {
	ref  Ref
	id   string
	text string
	kind string // post, status, update, direct
}

type fakeAdapter struct {
	mu    sync.Mutex
	posts []post
	n     int
}

func (f *fakeAdapter) Run(ctx context.Context, _ func(Inbound)) error { <-ctx.Done(); return nil }
func (f *fakeAdapter) add(kind string, ref Ref, id, text string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if id == "" {
		f.n++
		id = fmt.Sprintf("m%d", f.n)
	}
	f.posts = append(f.posts, post{ref, id, text, kind})
	return id
}
func (f *fakeAdapter) Post(_ context.Context, ref Ref, text string) (string, error) {
	return f.add("post", ref, "", text), nil
}
func (f *fakeAdapter) Status(_ context.Context, ref Ref, text string) (string, error) {
	return f.add("status", ref, "", text), nil
}
func (f *fakeAdapter) Update(_ context.Context, ref Ref, id, text string) error {
	f.add("update", ref, id, text)
	return nil
}
func (f *fakeAdapter) Direct(_ context.Context, user, text string) error {
	f.add("direct", Ref{ChatID: user}, "", text)
	return nil
}
func (f *fakeAdapter) last(kind string) post {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.posts) - 1; i >= 0; i-- {
		if f.posts[i].kind == kind {
			return f.posts[i]
		}
	}
	return post{}
}
func (f *fakeAdapter) count() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.posts) }

type fakeHub struct {
	mu       sync.Mutex
	sent     []api.SendRequest
	steers   []string
	stopped  []string
	closed   []string
	replied  []string
	tasks    map[string]*api.Message
	replies  map[string]chan *api.Message
	prog     map[string]chan api.Message
	inbox    chan api.Message
	steerErr error
}

func newFakeHub() *fakeHub {
	return &fakeHub{tasks: map[string]*api.Message{}, replies: map[string]chan *api.Message{},
		prog: map[string]chan api.Message{}, inbox: make(chan api.Message, 10)}
}

func (h *fakeHub) Send(_ context.Context, req api.SendRequest) (*api.Message, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sent = append(h.sent, req)
	id := fmt.Sprintf("msg_%d", len(h.sent))
	th := req.ThreadID
	if th == "" {
		th = id
	}
	m := &api.Message{ID: id, To: req.To, ThreadID: th, Type: "task", Status: "queued", Body: req.Body}
	h.tasks[id] = m
	h.replies[id] = make(chan *api.Message, 1)
	h.prog[id] = make(chan api.Message, 10)
	return m, nil
}
func (h *fakeHub) Steer(_ context.Context, task, body string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.steerErr != nil {
		return h.steerErr
	}
	h.steers = append(h.steers, task+": "+body)
	return nil
}
func (h *fakeHub) Stop(_ context.Context, task string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.stopped = append(h.stopped, task)
	return nil
}
func (h *fakeHub) Get(_ context.Context, id string) (*api.Message, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	m := *h.tasks[id]
	return &m, nil
}
func (h *fakeHub) WaitReply(ctx context.Context, task string, d time.Duration, progress func(api.Message)) (*api.Message, error) {
	h.mu.Lock()
	rc, pc := h.replies[task], h.prog[task]
	h.mu.Unlock()
	t := time.NewTimer(d)
	defer t.Stop()
	for {
		select {
		case r := <-rc:
			return r, nil
		case p := <-pc:
			progress(p)
		case <-t.C:
			return nil, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}
func (h *fakeHub) Pull(ctx context.Context, wait time.Duration) ([]api.Message, error) {
	select {
	case m := <-h.inbox:
		return []api.Message{m}, nil
	case <-time.After(wait):
		return nil, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (h *fakeHub) Close(_ context.Context, id string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = append(h.closed, id)
	return nil
}
func (h *fakeHub) Reply(_ context.Context, id, body string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.replied = append(h.replied, id)
	return nil
}
func (h *fakeHub) nsent() int { h.mu.Lock(); defer h.mu.Unlock(); return len(h.sent) }

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// stateDir is a bridge's state directory for a test. Not t.TempDir: the
// bridge's goroutines (process, watch) may still save their state just after
// a test ends, and TempDir fails the test when its cleanup meets a file
// written meanwhile. A late write here is harmless.
func stateDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "myco-im-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func newTestBridge(t *testing.T) (*Bridge, *fakeAdapter, *fakeHub, context.CancelFunc) {
	t.Helper()
	ad, hub := &fakeAdapter{}, newFakeHub()
	b, err := New(Config{Name: "fs", Platform: "feishu", Owner: "ou_owner", OwnerName: "glenn", Agent: "coder"}, ad, hub, stateDir(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	b.statusEvery = 0
	b.waitEach = 50 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = b.Run(ctx) }()
	eventually(t, "running", func() bool { return b.ctx.Load() != nil })
	return b, ad, hub, cancel
}

func TestTaskProgressAndReply(t *testing.T) {
	b, ad, hub, cancel := newTestBridge(t)
	defer cancel()
	b.Handle(Inbound{ChatID: "oc_1", MessageID: "om_1", ThreadKey: "chat:oc_1", UserID: "ou_owner", Text: "fix the build"})
	eventually(t, "task sent", func() bool { return hub.nsent() == 1 })
	req := hub.sent[0]
	if req.To != "coder" || !strings.HasPrefix(req.Body, "fix the build") || !strings.Contains(req.Body, "飞书") || !strings.Contains(req.Body, "glenn") {
		t.Fatalf("sent %+v", req)
	}
	eventually(t, "status card", func() bool { return ad.last("status").text != "" })
	if s := ad.last("status"); !strings.Contains(s.text, "收到") || strings.Contains(s.text, "coder") || s.ref.ReplyTo != "om_1" || s.ref.Thread {
		t.Fatalf("status %+v", s)
	}
	hub.prog["msg_1"] <- api.Message{Body: "running tests"}
	eventually(t, "progress update", func() bool { return strings.Contains(ad.last("update").text, "running tests") })
	hub.prog["msg_1"] <- api.Message{Body: "Which branch?\n1. main\n2. dev", Ask: true}
	eventually(t, "question posted", func() bool { return strings.Contains(ad.last("post").text, "Which branch?") })

	// A reply while the task is open steers it (and answers the question).
	b.Handle(Inbound{ChatID: "oc_1", MessageID: "om_2", ThreadKey: "chat:oc_1", UserID: "ou_owner", Text: "main"})
	eventually(t, "steer", func() bool { hub.mu.Lock(); defer hub.mu.Unlock(); return len(hub.steers) == 1 })
	if !strings.HasPrefix(hub.steers[0], "msg_1: main") {
		t.Fatalf("steer %q", hub.steers[0])
	}

	hub.replies["msg_1"] <- &api.Message{Body: "**Fixed** in abc123"}
	eventually(t, "answer posted", func() bool { return ad.last("post").text == "**Fixed** in abc123" })
	eventually(t, "task closed", func() bool { c := b.conv("chat:oc_1"); return c != nil && c.Task == "" })

	// The next message is a new task in the same myco thread.
	b.Handle(Inbound{ChatID: "oc_1", MessageID: "om_3", ThreadKey: "chat:oc_1", UserID: "ou_owner", Text: "now add a test"})
	eventually(t, "second task", func() bool { return hub.nsent() == 2 })
	if hub.sent[1].ThreadID != "msg_1" || hub.sent[1].To != "coder" {
		t.Fatalf("second %+v", hub.sent[1])
	}
}

func TestRedeliveredMessageOnce(t *testing.T) {
	b, _, hub, cancel := newTestBridge(t)
	defer cancel()
	in := Inbound{ChatID: "oc_1", MessageID: "om_1", ThreadKey: "chat:oc_1", UserID: "ou_owner", Text: "hi"}
	b.Handle(in)
	b.Handle(in)
	eventually(t, "sent", func() bool { return hub.nsent() == 1 })
	time.Sleep(50 * time.Millisecond)
	if n := hub.nsent(); n != 1 {
		t.Fatalf("sent %d times", n)
	}
}

func TestStrangersAndGroups(t *testing.T) {
	b, ad, hub, cancel := newTestBridge(t)
	defer cancel()
	// Group talk without a mention is not for the bot.
	b.Handle(Inbound{ChatID: "oc_g", Group: true, MessageID: "om_1", ThreadKey: "msg:om_1", UserID: "ou_owner", Text: "lunch?"})
	// Someone else is told how to be let in, and nothing is sent.
	b.Handle(Inbound{ChatID: "oc_g", Group: true, MessageID: "om_2", ThreadKey: "msg:om_2", UserID: "ou_bob", Text: "do it", Mentioned: true})
	eventually(t, "refusal", func() bool { return strings.Contains(ad.last("post").text, "ou_bob") })
	if hub.nsent() != 0 || ad.count() != 1 {
		t.Fatalf("sent %d, posts %d", hub.nsent(), ad.count())
	}
	// The owner mentioning the bot in a group: answered in the topic.
	b.Handle(Inbound{ChatID: "oc_g", Group: true, MessageID: "om_3", ThreadKey: "msg:om_3", UserID: "ou_owner", Text: "check PR 9", Mentioned: true})
	eventually(t, "sent", func() bool { return hub.nsent() == 1 })
	if hub.sent[0].To != "coder" || !strings.HasPrefix(hub.sent[0].Body, "check PR 9") {
		t.Fatalf("sent %+v", hub.sent[0])
	}
	eventually(t, "status", func() bool { return ad.last("status").ref.Thread })
	// Inside that topic, no mention is needed.
	b.Handle(Inbound{ChatID: "oc_g", Group: true, MessageID: "om_4", ThreadKey: "msg:om_3", UserID: "ou_owner", Text: "also #10"})
	eventually(t, "steer", func() bool { hub.mu.Lock(); defer hub.mu.Unlock(); return len(hub.steers) == 1 })
}

func TestCommands(t *testing.T) {
	b, ad, hub, cancel := newTestBridge(t)
	defer cancel()
	key := "chat:oc_1"
	in := func(id, text string) Inbound {
		return Inbound{ChatID: "oc_1", MessageID: id, ThreadKey: key, UserID: "ou_owner", Text: text}
	}
	b.Handle(in("1", "/help"))
	eventually(t, "help", func() bool { return strings.Contains(ad.last("post").text, "/stop") })
	b.Handle(in("2", "draft a post"))
	eventually(t, "sent", func() bool { return hub.nsent() == 1 })
	if hub.sent[0].To != "coder" {
		t.Fatalf("to %q", hub.sent[0].To)
	}
	b.Handle(in("3", "!stop"))
	eventually(t, "stopped", func() bool { hub.mu.Lock(); defer hub.mu.Unlock(); return len(hub.stopped) == 1 })
	b.Handle(in("4", "/new"))
	eventually(t, "forgot", func() bool { return b.conv(key) == nil })
	// Others are not commands: a path, or a command of another bot.
	b.Handle(in("5", "/path/to/file is broken"))
	eventually(t, "not a command", func() bool { return hub.nsent() == 2 })
	if hub.sent[1].ThreadID != "" || !strings.HasPrefix(hub.sent[1].Body, "/path/to/file") {
		t.Fatalf("sent %+v", hub.sent[1])
	}
	b.Handle(in("6", "/to reviewer check it"))
	eventually(t, "steer", func() bool { hub.mu.Lock(); defer hub.mu.Unlock(); return len(hub.steers) == 1 })
	if !strings.Contains(hub.steers[0], "/to reviewer check it") {
		t.Fatalf("steer %q", hub.steers[0])
	}
}

func TestSteerOnClosedTaskStartsNewOne(t *testing.T) {
	b, _, hub, cancel := newTestBridge(t)
	defer cancel()
	b.Handle(Inbound{ChatID: "c", MessageID: "1", ThreadKey: "chat:c", UserID: "ou_owner", Text: "one"})
	eventually(t, "sent", func() bool { return hub.nsent() == 1 })
	hub.mu.Lock()
	hub.steerErr = &client.Error{Status: 409, Code: "invalid_state", Message: "task is already done"}
	hub.mu.Unlock()
	b.Handle(Inbound{ChatID: "c", MessageID: "2", ThreadKey: "chat:c", UserID: "ou_owner", Text: "two"})
	eventually(t, "new task", func() bool { return hub.nsent() == 2 })
	if hub.sent[1].ThreadID != "msg_1" {
		t.Fatalf("thread %q", hub.sent[1].ThreadID)
	}
}

func TestFailedTask(t *testing.T) {
	b, ad, hub, cancel := newTestBridge(t)
	defer cancel()
	b.Handle(Inbound{ChatID: "c", MessageID: "1", ThreadKey: "chat:c", UserID: "ou_owner", Text: "one"})
	eventually(t, "sent", func() bool { return hub.nsent() == 1 })
	hub.mu.Lock()
	hub.tasks["msg_1"].Status, hub.tasks["msg_1"].Error = "failed", "runner died"
	hub.mu.Unlock()
	eventually(t, "failure shown", func() bool { return strings.Contains(ad.last("update").text, "runner died") })
	eventually(t, "closed", func() bool { c := b.conv("chat:c"); return c != nil && c.Task == "" })
}

func TestInboxForwardsToOwner(t *testing.T) {
	_, ad, hub, cancel := newTestBridge(t)
	defer cancel()
	hub.inbox <- api.Message{ID: "msg_x", From: "ops", FromDisplay: "Ops", Type: "note", Body: "deploy done"}
	eventually(t, "forwarded", func() bool { return strings.Contains(ad.last("direct").text, "deploy done") })
	if d := ad.last("direct"); d.ref.ChatID != "ou_owner" || !strings.Contains(d.text, "Ops") {
		t.Fatalf("direct %+v", d)
	}
	eventually(t, "closed", func() bool { hub.mu.Lock(); defer hub.mu.Unlock(); return len(hub.closed) == 1 })
	hub.inbox <- api.Message{ID: "msg_y", From: "ops", Type: "task", Body: "approve?"}
	eventually(t, "task answered", func() bool { hub.mu.Lock(); defer hub.mu.Unlock(); return len(hub.replied) == 1 })
}

func TestStateSurvivesRestart(t *testing.T) {
	dir := stateDir(t)
	ad, hub := &fakeAdapter{}, newFakeHub()
	cfg := Config{Name: "fs", Platform: "slack", Owner: "U1", Agent: "coder"}
	b, _ := New(cfg, ad, hub, dir, nil)
	b.Handle(Inbound{ChatID: "D1", MessageID: "1", ThreadKey: "chat:D1", UserID: "U1", Text: "one"})
	eventually(t, "sent", func() bool { return hub.nsent() == 1 })
	eventually(t, "saved", func() bool { c := b.conv("chat:D1"); return c != nil && c.Task == "msg_1" })
	b2, err := New(cfg, ad, hub, dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if c := b2.conv("chat:D1"); c == nil || c.Task != "msg_1" || c.ThreadID != "msg_1" {
		t.Fatalf("restored %+v", c)
	}
	if !strings.Contains(hub.sent[0].Body, "U1 on Slack") {
		t.Fatalf("body %q", hub.sent[0].Body)
	}
}

func TestConfigFiles(t *testing.T) {
	dir := t.TempDir()
	c := &Config{Name: "work", Platform: "feishu", AppID: "cli_1", AppSecret: "s3cret", Profile: "/x.yaml", Owner: "ou_1"}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := Save(dir, c); err != nil {
		t.Fatal(err)
	}
	names, _ := List(dir)
	if len(names) != 1 || names[0] != "work" {
		t.Fatalf("list %v", names)
	}
	got, err := Load(dir, "work")
	if err != nil || got.AppSecret != "s3cret" {
		t.Fatalf("load %+v %v", got, err)
	}
	if r := fmt.Sprint(got.Redacted()); strings.Contains(r, "s3cret") {
		t.Fatalf("redacted shows the secret: %s", r)
	}
	if !got.Allowed("ou_1") || got.Allowed("ou_2") || got.Allowed("") {
		t.Fatal("allowed")
	}
	if (&Config{Name: "Bad Name"}).Validate() == nil || (&Config{Name: "s", Platform: "slack", BotToken: "x", Profile: "p"}).Validate() == nil {
		t.Fatal("validate accepted bad configs")
	}
	if err := Remove(dir, "work"); err != nil {
		t.Fatal(err)
	}
}

func TestParseCommand(t *testing.T) {
	for in, want := range map[string]command{
		"hello":           {rest: "hello"},
		"/claim ab12":     {name: "claim", arg: "ab12"},
		"!new go":         {name: "new", rest: "go"},
		"/STOP":           {name: "stop"},
		"/usr/bin is odd": {rest: "/usr/bin is odd"},
	} {
		if got := parseCommand(in); got != want {
			t.Errorf("%q: %+v, want %+v", in, got, want)
		}
	}
}

func TestClaim(t *testing.T) {
	ad, hub := &fakeAdapter{}, newFakeHub()
	var saved Config
	b, _ := New(Config{Name: "s", Platform: "slack", Claim: "abc123", ClaimExpires: time.Now().Add(time.Hour), Agent: "coder"}, ad, hub, "", nil)
	b.OnConfig = func(c Config) error { saved = c; return nil }
	b.Handle(Inbound{ChatID: "D1", MessageID: "1", ThreadKey: "chat:D1", UserID: "U9", Text: "hi"})
	eventually(t, "unclaimed", func() bool { return strings.Contains(ad.last("post").text, "no owner") })
	b.Handle(Inbound{ChatID: "D1", MessageID: "2", ThreadKey: "chat:D1", UserID: "U9", Text: "/claim nope"})
	eventually(t, "bad code", func() bool { return strings.Contains(ad.last("post").text, "not right") })
	b.Handle(Inbound{ChatID: "D1", MessageID: "3", ThreadKey: "chat:D1", UserID: "U9", Text: "/claim abc123"})
	eventually(t, "claimed", func() bool { return strings.Contains(ad.last("post").text, "you own") })
	if saved.Owner != "U9" || saved.Claim != "" {
		t.Fatalf("saved %+v", saved)
	}
	b.Handle(Inbound{ChatID: "D1", MessageID: "4", ThreadKey: "chat:D1", UserID: "U9", Text: "go"})
	eventually(t, "task", func() bool { return hub.nsent() == 1 })
}

func TestClaimLocksAfterFailsAndExpires(t *testing.T) {
	ad, hub := &fakeAdapter{}, newFakeHub()
	var saved Config
	b, _ := New(Config{Name: "s", Platform: "slack", Claim: "abc123", ClaimExpires: time.Now().Add(time.Hour), Agent: "coder"}, ad, hub, "", nil)
	b.OnConfig = func(c Config) error { saved = c; return nil }
	for i := 1; i <= MaxClaimFails; i++ {
		b.Handle(Inbound{ChatID: "D1", MessageID: fmt.Sprint("w", i), ThreadKey: "chat:D1", UserID: "U9", Text: "/claim wrong"})
		eventually(t, "counted", func() bool { b.cfgMu.Lock(); defer b.cfgMu.Unlock(); return b.cfg.ClaimFails == i })
	}
	if saved.ClaimFails != MaxClaimFails {
		t.Fatalf("fails saved: %d", saved.ClaimFails)
	}
	eventually(t, "gone", func() bool { return strings.Contains(ad.last("post").text, "no longer works") })
	// The right code no longer works either.
	b.Handle(Inbound{ChatID: "D1", MessageID: "right", ThreadKey: "chat:D1", UserID: "U9", Text: "/claim abc123"})
	time.Sleep(50 * time.Millisecond)
	if saved.Owner != "" || b.owner() != "" {
		t.Fatal("claimed with a locked code")
	}

	// An expired code does not work.
	c := Config{Name: "s", Claim: "abc123", ClaimExpires: time.Now().Add(-time.Minute)}
	if c.ClaimUsable(time.Now()) {
		t.Fatal("expired code usable")
	}
	// A new one does, for a day.
	code := c.NewClaim(time.Now())
	if len(code) != 10 || !c.ClaimUsable(time.Now()) || c.ClaimUsable(time.Now().Add(ClaimTTL+time.Minute)) || c.ClaimFails != 0 {
		t.Fatalf("new claim %q %+v", code, c)
	}
}
