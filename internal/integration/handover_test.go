package integration_test

import (
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/client"
)

// Handing a node over: a link lets another session answer for the same
// name. The old session is told it was replaced; the tasks it had not
// finished go to the new one; run settings of another CLI are dropped; the
// new session gets a handover note; switching back works the same way. A
// link works once, and not in an end-to-end encrypted workspace.
func TestHandover(t *testing.T) {
	c := ctx(t)
	db := filepath.Join(t.TempDir(), "hub.db")
	h := startHub(t, db, app.DefaultMessagePolicy())
	admin := h.admin()
	oldTok := h.register(api.RegisterRequest{Name: "docs", Description: "writes the docs", Binding: api.Binding{Harness: "claude", SessionID: "session_old", Workdir: "/home/user/myco"}})
	bossTok := h.register(api.RegisterRequest{Name: "boss"})
	old, boss := h.client(oldTok), h.client(bossTok)
	if _, err := admin.SetRunSettings(c, "docs", api.RunSettings{Model: "opus", Effort: "high"}); err != nil {
		t.Fatal(err)
	}
	// Two tasks: one being worked on (acked), one never pulled.
	t1, err := boss.Send(c, api.SendRequest{To: "docs", Type: "task", Body: "shorten section 4.16"}, false)
	if err != nil {
		t.Fatal(err)
	}
	got, err := old.Pull(c, 10, 0)
	if err != nil || len(got) != 1 {
		t.Fatalf("pull: %v %+v", err, got)
	}
	if err := old.Ack(c, t1.Message.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := old.Progress(c, t1.Message.ID, "half done: 4.16.1 rewritten", false); err != nil {
		t.Fatal(err)
	}
	t2, err := boss.Send(c, api.SendRequest{To: "docs", Type: "task", Body: "then fix the typos"}, false)
	if err != nil {
		t.Fatal(err)
	}

	// Shell nodes cannot be handed over; a dry run makes nothing.
	h.register(api.RegisterRequest{Name: "box-shell", Binding: api.Binding{Harness: "exec", SessionID: "exec:box"}})
	if _, err := admin.CreateHandoverLink(c, "box-shell", api.HandoverLinkRequest{}, false); err == nil {
		t.Fatal("a shell node has nothing to hand over")
	}
	dry, err := admin.CreateHandoverLink(c, "docs", api.HandoverLinkRequest{}, true)
	if err != nil || !dry.DryRun || dry.URL != "" || dry.Handover.From.Binding.SessionID != "session_old" {
		t.Fatalf("dry run: %v %+v", err, dry)
	}
	if list, _ := admin.Handovers(c, "docs"); len(list) != 0 {
		t.Fatalf("a dry run stores nothing: %+v", list)
	}

	link, err := admin.CreateHandoverLink(c, "docs", api.HandoverLinkRequest{Note: "subscription ran out"}, false)
	if err != nil || !strings.Contains(link.URL, "/h/myco_takeover_") || !strings.Contains(link.Command, "curl -fsSL") || link.Handover.State != "waiting" {
		t.Fatalf("link: %v %+v", err, link)
	}
	token := link.URL[strings.LastIndex(link.URL, "/h/")+3:]
	// The script carries the token in the environment, never in argv.
	resp, err := http.Get(link.URL)
	if err != nil {
		t.Fatal(err)
	}
	script, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(script), "MYCO_TAKEOVER='"+token+"'") || !strings.Contains(string(script), "join --hub") {
		t.Fatalf("script %d: %s", resp.StatusCode, script)
	}
	if r, _ := http.Get(h.url + "/h/myco_takeover_nope"); r.StatusCode != 404 {
		t.Fatalf("unknown link: %d", r.StatusCode)
	}
	// Nothing changed yet.
	if _, err := old.Pull(c, 1, 0); err != nil {
		t.Fatalf("before the take-over the old session works: %v", err)
	}

	// Taken over by a Codex session.
	anon, _ := client.New(h.url, "")
	tk, err := anon.TakeOver(c, api.TakeOverRequest{Token: token, Binding: api.Binding{Harness: "codex", SessionID: "rollout-new", Environment: "mac-mini", Workdir: "/Users/g/myco"}})
	if err != nil {
		t.Fatal(err)
	}
	if tk.Agent.Name != "docs" || tk.Token == "" || tk.Handover.State != "done" || tk.Agent.Binding.SessionID != "rollout-new" || tk.Agent.RunSettings != nil {
		t.Fatalf("take-over: %+v (run settings %+v)", tk, tk.Agent.RunSettings)
	}
	// The acked task, and the one the check above leased.
	if len(tk.Handover.OpenTasks) != 2 || tk.Handover.OpenTasks[0] != t1.Message.ID || tk.Handover.OpenTasks[1] != t2.Message.ID {
		t.Fatalf("open tasks back in the queue: %+v", tk.Handover.OpenTasks)
	}
	for _, want := range []string{"you now are the agent docs", "claude session", "/home/user/myco", "subscription ran out",
		"shorten section 4.16", "half done: 4.16.1 rewritten", "then fix the typos", "myco thread get --id"} {
		if !strings.Contains(tk.Brief, want) {
			t.Fatalf("the brief lacks %q:\n%s", want, tk.Brief)
		}
	}
	// The link worked once.
	if _, err := anon.TakeOver(c, api.TakeOverRequest{Token: token, Binding: api.Binding{Harness: "codex", SessionID: "rollout-other"}}); err == nil {
		t.Fatal("a link works once")
	}

	// The old session is told, with its own code; nothing it does counts.
	_, err = old.Pull(c, 1, 0)
	if e, ok := client.AsError(err); !ok || e.Code != "replaced" || e.Status != 401 || !strings.Contains(e.Message, "taken over") || !strings.Contains(e.Suggestion, "--back "+tk.Handover.ID) {
		t.Fatalf("old session: %v", err)
	}
	if _, err := old.Reply(c, t1.Message.ID, "done", false); err == nil {
		t.Fatal("the old session cannot answer a task that went to the new one")
	}

	// The new session gets both tasks.
	nw := h.client(tk.Token)
	got, err = nw.Pull(c, 10, 0)
	if err != nil || len(got) != 2 {
		t.Fatalf("new session pull: %v %+v", err, got)
	}
	ids := map[string]bool{got[0].ID: true, got[1].ID: true}
	if !ids[t1.Message.ID] || !ids[t2.Message.ID] {
		t.Fatalf("tasks: %+v", got)
	}
	if _, err := nw.Reply(c, t1.Message.ID, "shortened", false); err != nil {
		t.Fatal(err)
	}
	me, err := nw.MyHandover(c)
	if err != nil || me.Handover.ID != tk.Handover.ID || !strings.Contains(me.Brief, "docs") {
		t.Fatalf("handover show: %v %+v", err, me)
	}
	list, err := admin.Handovers(c, "docs")
	if err != nil || len(list) != 1 || list[0].From.Binding.Harness != "claude" || list[0].To.Binding.Harness != "codex" || list[0].By != "admin" {
		t.Fatalf("list: %v %+v", err, list)
	}

	// Switching back: the old session takes it back, and hears only what
	// happened since.
	back, err := admin.CreateHandoverLink(c, "docs", api.HandoverLinkRequest{Back: tk.Handover.ID}, false)
	if err != nil {
		t.Fatal(err)
	}
	backTok := back.URL[strings.LastIndex(back.URL, "/h/")+3:]
	tb, err := anon.TakeOver(c, api.TakeOverRequest{Token: backTok, Binding: api.Binding{Harness: "claude", SessionID: "session_old", Workdir: "/home/user/myco"}})
	if err != nil {
		t.Fatal(err)
	}
	if tb.Handover.Back != tk.Handover.ID || !strings.Contains(tb.Brief, "again") || !strings.Contains(tb.Brief, "shortened") || strings.Contains(tb.Brief, "half done") {
		t.Fatalf("switch back: %+v\n%s", tb.Handover, tb.Brief)
	}
	if _, err := nw.Pull(c, 1, 0); err == nil {
		t.Fatal("the codex session is replaced in turn")
	}
	if _, err := h.client(tb.Token).Pull(c, 1, 0); err != nil {
		t.Fatalf("back: %v", err)
	}

	// Not in an end-to-end encrypted workspace.
	if _, err := admin.SetHubSettings(c, api.HubSettings{E2E: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.CreateHandoverLink(c, "docs", api.HandoverLinkRequest{}, false); err == nil || !strings.Contains(err.Error(), "end-to-end") {
		t.Fatalf("e2e: %v", err)
	}
}
