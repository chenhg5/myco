package integration_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/client"
)

// What a person hears about: an agent asking them needs them (announced,
// counted on the badge) until they answer; an answer is kept but, by
// default, not announced; their preferences decide, once, on the hub.
func TestNotices(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	worker := h.client(h.register(api.RegisterRequest{Name: "worker"}))
	me := h.admin()

	if _, err := worker.Notices(c, 0, 0, 0); err == nil {
		t.Fatal("an agent must not read a person's notices")
	}
	feed, err := me.Notices(c, 0, 0, 0)
	if err != nil || len(feed.Notices) != 0 || feed.Badge != 0 {
		t.Fatalf("a new hub has no notices: %v %+v", err, feed)
	}

	sent, err := me.ConsoleSend(c, api.SendRequest{To: "worker", Type: "task", Body: "tidy the docs"})
	if err != nil {
		t.Fatal(err)
	}
	task := sent.Message.ID
	if _, err := worker.Pull(c, 1, 0); err != nil {
		t.Fatal(err)
	}
	if err := worker.Ack(c, task); err != nil {
		t.Fatal(err)
	}
	if _, _, err := worker.Ask(c, task, "Delete the old pages?\n1. Yes\n2. No", nil, false); err != nil {
		t.Fatal(err)
	}
	// The hub works notices out on its own; a waiting read wakes for it.
	feed, err = me.Notices(c, 0, 0, 5*time.Second)
	if err != nil || len(feed.Notices) != 1 {
		t.Fatalf("the question must be told: %v %+v", err, feed)
	}
	ask := feed.Notices[0]
	if ask.Kind != "asking" || ask.Tier != "needs_you" || !ask.Announce || ask.Target != "agent:worker" || ask.Actor != "worker" {
		t.Fatalf("question notice: %+v", ask)
	}
	if ask.Message == nil || ask.Message.Body == "" || !ask.Message.Ask {
		t.Fatalf("the notice carries its message: %+v", ask.Message)
	}
	if feed.Badge != 1 || feed.Last != ask.Seq {
		t.Fatalf("badge and cursor: %+v", feed)
	}

	// Answering settles it.
	if _, err := me.ConsoleSteer(c, task, "2"); err != nil {
		t.Fatal(err)
	}
	waitBadge(t, me, 0)

	// The reply is kept, not announced (results are off by default).
	if _, err := worker.Reply(c, task, "kept them", false); err != nil {
		t.Fatal(err)
	}
	feed, err = me.Notices(c, ask.Seq, 0, 5*time.Second)
	if err != nil || len(feed.Notices) != 1 {
		t.Fatalf("the answer must be kept: %v %+v", err, feed)
	}
	if n := feed.Notices[0]; n.Kind != "answered" || n.Announce || n.Quiet != "tier" || n.ResolvedAt != nil {
		t.Fatalf("answer notice: %+v", n)
	}
	last := feed.Last

	// Preferences: answers on, but quiet hours all day long keep it quiet.
	p, err := me.SetNoticePrefs(c, api.NoticePrefs{Tiers: map[string]bool{"results": true}})
	if err != nil || !p.Tiers["results"] {
		t.Fatalf("set prefs: %v %+v", err, p)
	}
	if _, err := me.SetNoticePrefs(c, api.NoticePrefs{Quiet: &api.NoticeQuiet{From: 0, To: 0, Zone: "Nowhere/Land"}}); err == nil {
		t.Fatal("an unknown zone is refused")
	}
	sendAndReply(t, me, worker, "one more")
	feed, err = me.Notices(c, last, 0, 5*time.Second)
	if err != nil || len(feed.Notices) != 1 || !feed.Notices[0].Announce || feed.Notices[0].MessageID == "" {
		t.Fatalf("with answers on, it is announced: %v %+v", err, feed)
	}
	last = feed.Last
	if _, err := me.SetNoticePrefs(c, api.NoticePrefs{Tiers: map[string]bool{"results": true}, Quiet: &api.NoticeQuiet{From: 0, To: 1439, Zone: "UTC"}}); err != nil {
		t.Fatal(err)
	}
	sendAndReply(t, me, worker, "and another")
	feed, err = me.Notices(c, last, 0, 5*time.Second)
	if err != nil || len(feed.Notices) != 1 || feed.Notices[0].Announce || feed.Notices[0].Quiet != "quiet_hours" {
		t.Fatalf("quiet hours keep it quiet: %v %+v", err, feed)
	}

	// Seen in the App: answers there are settled.
	n, err := me.ReadNotices(c, "agent:worker", 0)
	if err != nil || n == 0 {
		t.Fatalf("read: %v %d", err, n)
	}
	all, err := me.Notices(c, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range all.Notices {
		if x.ReadAt == nil || (x.Kind == "answered" && x.ResolvedAt == nil) {
			t.Fatalf("after reading: %+v", x)
		}
	}
	got, err := me.NoticePrefs(c)
	if err != nil || got.Quiet == nil || got.Quiet.To != 1439 {
		t.Fatalf("prefs read back: %v %+v", err, got)
	}
}

func sendAndReply(t *testing.T, me, worker *client.Client, body string) string {
	t.Helper()
	c := ctx(t)
	sent, err := me.ConsoleSend(c, api.SendRequest{To: "worker", Type: "task", Body: body})
	if err != nil {
		t.Fatal(err)
	}
	// (Whatever else waits, steers among them, comes along.)
	if _, err := worker.Pull(c, 10, 0); err != nil {
		t.Fatal(err)
	}
	if err := worker.Ack(c, sent.Message.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := worker.Reply(c, sent.Message.ID, "done: "+body, false); err != nil {
		t.Fatal(err)
	}
	return sent.Message.ID
}

// waitBadge waits for the hub to settle what it was told.
func waitBadge(t *testing.T, me *client.Client, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		feed, err := me.Notices(ctx(t), 0, 0, 0)
		if err == nil && feed.Badge == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("badge: want %d, got %v %+v", want, err, feed)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
