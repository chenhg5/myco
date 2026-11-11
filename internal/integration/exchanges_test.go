package integration_test

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/client"

	_ "modernc.org/sqlite"
)

// The app shows a node's exchanges with other nodes grouped by peer, and the
// whole history between two nodes across threads, paged.
func TestPeersAndExchanges(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	lead := h.client(h.register(api.RegisterRequest{Name: "lead", Binding: api.Binding{Harness: "claude", SessionID: "s1"}}))
	box := h.client(h.register(api.RegisterRequest{Name: "box", Binding: api.Binding{Harness: "exec", SessionID: "s2"}}))
	h.register(api.RegisterRequest{Name: "docs", Binding: api.Binding{Harness: "codex", SessionID: "s3"}})

	// Five commands, each its own thread, plus one message to docs and one
	// from the console.
	for i := 0; i < 5; i++ {
		m, err := lead.Send(c, api.SendRequest{To: "box", Type: "task", Body: "uptime"}, false)
		if err != nil {
			t.Fatal(err)
		}
		box.Pull(c, 1, 0)
		box.Reply(c, m.Message.ID, `{"exit_code":0}`, false)
		time.Sleep(2 * time.Millisecond)
	}
	lead.Send(c, api.SendRequest{To: "docs", Type: "note", Body: "fyi"}, false)
	h.admin().ConsoleSend(c, api.SendRequest{To: "lead", Body: "status?"})

	peers, err := h.admin().Peers(c, "lead")
	if err != nil || len(peers) != 2 || peers[0].Agent != "docs" || peers[1].Agent != "box" || peers[1].Messages != 10 || peers[1].Last == nil || peers[1].Last.From != "box" {
		t.Fatalf("peers: %v %+v", err, peers)
	}
	page, err := h.admin().Exchanges(c, "lead", "box", 0, 4)
	if err != nil || len(page.Messages) != 4 || !page.More || page.Messages[0].From != "box" {
		t.Fatalf("first page: %v %+v", err, page)
	}
	older, err := h.admin().Exchanges(c, "box", "lead", page.Messages[3].CreatedAt.UnixMilli(), 50)
	if err != nil || len(older.Messages) != 6 || older.More {
		t.Fatalf("older: %v %d %v", err, len(older.Messages), older.More)
	}
	if _, err := h.client("").Peers(c, "lead"); err == nil {
		t.Fatal("peers need a console credential")
	}
}

// Offline nodes pile up; cleanup removes those unseen for N days that the
// caller manages, and keeps hand-registered identities.
func TestCleanupOfflineNodes(t *testing.T) {
	c := ctx(t)
	db := filepath.Join(t.TempDir(), "hub.db")
	h := startHub(t, db, app.DefaultMessagePolicy())
	for _, n := range []string{"old-1", "old-2", "recent"} {
		h.register(api.RegisterRequest{Name: n, Binding: api.Binding{Harness: "claude", SessionID: "s-" + n}})
	}
	h.register(api.RegisterRequest{Name: "script"}) // no harness
	raw, err := sql.Open("sqlite", db)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	old := time.Now().Add(-40 * 24 * time.Hour).UnixMilli()
	for _, n := range []string{"old-1", "old-2", "script"} {
		if _, err := raw.Exec(`UPDATE agents SET last_seen_at=?, created_at=? WHERE name=?`, old, old, n); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := raw.Exec(`UPDATE agents SET last_seen_at=? WHERE name='recent'`, time.Now().Add(-2*24*time.Hour).UnixMilli()); err != nil {
		t.Fatal(err)
	}

	preview, err := h.admin().CleanupAgents(c, 30, true)
	if err != nil || len(preview.Agents) != 2 || preview.Removed {
		t.Fatalf("preview: %v %+v", err, preview)
	}
	if _, err := h.admin().GetAgent(c, "old-1"); err != nil {
		t.Fatal("a preview must not remove anything")
	}
	done, err := h.admin().CleanupAgents(c, 30, false)
	if err != nil || len(done.Agents) != 2 || !done.Removed {
		t.Fatalf("cleanup: %v %+v", err, done)
	}
	for n, want := range map[string]bool{"old-1": false, "old-2": false, "recent": true, "script": true} {
		_, err := h.admin().GetAgent(c, n)
		if (err == nil) != want {
			t.Errorf("%s: exists=%v, want %v", n, err == nil, want)
		}
	}
	var ce *client.Error
	if _, err := h.admin().CleanupAgents(c, 0, true); err == nil {
		t.Fatal("offline_days 0 must be refused")
	} else if !errors.As(err, &ce) || ce.Code != "invalid_argument" {
		t.Fatalf("offline_days 0: %v", err)
	}
	log, _ := h.admin().Audit(c, client.AuditFilter{})
	n := 0
	for _, e := range log {
		if e.Action == "agent.remove" {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("audit: %d removals", n)
	}
}

// A view that has a conversation asks only for what changed since: new
// messages, and old ones whose state changed.
func TestUpdatedSince(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	lead := h.client(h.register(api.RegisterRequest{Name: "lead", Binding: api.Binding{Harness: "claude", SessionID: "s1"}}))
	box := h.client(h.register(api.RegisterRequest{Name: "box", Binding: api.Binding{Harness: "exec", SessionID: "s2"}}))

	first, err := lead.Send(c, api.SendRequest{To: "box", Type: "task", Body: "uptime"}, false)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	mark := time.Now().UnixMilli()
	time.Sleep(5 * time.Millisecond)
	second, _ := lead.Send(c, api.SendRequest{To: "box", Type: "task", Body: "df -h", ThreadID: first.Message.ThreadID}, false)

	// Exchanges: only the message sent after the mark.
	ex, err := h.admin().ExchangesSince(c, "lead", "box", mark)
	if err != nil || ex.Since != mark || len(ex.Messages) != 1 || ex.Messages[0].ID != second.Message.ID {
		t.Fatalf("exchanges since: %v %+v", err, ex)
	}
	// Thread: the same.
	th, err := h.admin().ThreadSince(c, first.Message.ThreadID, mark)
	if err != nil || th.Since != mark || len(th.Messages) != 1 || th.Messages[0].ID != second.Message.ID {
		t.Fatalf("thread since: %v %+v", err, th)
	}

	// The first task is answered: it changed, so it comes again, with its
	// reply; the oldest change first.
	time.Sleep(5 * time.Millisecond)
	mark2 := time.Now().UnixMilli()
	time.Sleep(5 * time.Millisecond)
	box.Pull(c, 2, 0)
	if _, err := box.Reply(c, first.Message.ID, `{"exit_code":0}`, false); err != nil {
		t.Fatal(err)
	}
	ex, err = h.admin().ExchangesSince(c, "lead", "box", mark2)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]string{}
	for _, m := range ex.Messages {
		ids[m.ID] = m.Status
	}
	if ids[first.Message.ID] != "done" || len(ex.Messages) < 2 {
		t.Fatalf("an answered task comes again with its new state: %+v", ex.Messages)
	}

	// Nothing changed: nothing comes.
	if ex, err := h.admin().ExchangesSince(c, "lead", "box", time.Now().Add(time.Minute).UnixMilli()); err != nil || len(ex.Messages) != 0 {
		t.Fatalf("no change: %v %+v", err, ex)
	}
	// Without it: the whole page, as before (Since 0).
	if page, err := h.admin().Exchanges(c, "lead", "box", 0, 50); err != nil || page.Since != 0 || len(page.Messages) < 3 {
		t.Fatalf("full page: %v %+v", err, page)
	}
	// The scope still applies.
	if _, err := h.client("").ExchangesSince(c, "lead", "box", mark); err == nil {
		t.Fatal("exchanges since need a console credential")
	}
}

// A pulled message names its sender as the app shows it.
func TestPullCarriesSenderDisplayName(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	lead := h.client(h.register(api.RegisterRequest{Name: "lead", DisplayName: "Lead · Studio Mac"}))
	box := h.client(h.register(api.RegisterRequest{Name: "box"}))
	if _, err := lead.Send(c, api.SendRequest{To: "box", Type: "task", Body: "hi"}, false); err != nil {
		t.Fatal(err)
	}
	msgs, err := box.Pull(c, 1, 0)
	if err != nil || len(msgs) != 1 || msgs[0].FromDisplay != "Lead · Studio Mac" {
		t.Fatalf("%v %+v", err, msgs)
	}
}
