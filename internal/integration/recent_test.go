package integration_test

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/client"
)

// The recent list: people talking to agents, agents handing work to each
// other and agents running commands on a shell node, newest first, each
// person seeing only what they may read — through the list and through the
// older endpoints alike. Filters apply on the hub, before paging.
func TestRecentListVisibilityAndFilters(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	glenn := h.admin()
	if _, err := glenn.UpdateMember(c, "owner", api.MemberUpdateRequest{Name: "glenn"}); err != nil {
		t.Fatal(err)
	}
	member := func(name string) *client.Client {
		inv, err := glenn.InviteMember(c, api.InviteRequest{Name: name})
		if err != nil {
			t.Fatal(err)
		}
		p, err := h.client("").Pair(c, inv.Link[strings.Index(inv.Link, "/c/")+3:], name+"-mac")
		if err != nil {
			t.Fatal(err)
		}
		return h.client(p.Token)
	}
	laohu, matthew := member("laohu"), member("matthew")

	glennBot := h.client(h.register(api.RegisterRequest{Name: "glenn-bot", Binding: api.Binding{Harness: "claude", SessionID: "s-g"}}))
	enr, err := laohu.CreateEnrollment(c, api.CreateEnrollmentRequest{NamePrefix: "laohu-", MaxUses: 1}, false)
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.client(enr.Token).Enroll(c, api.EnrollRequest{Name: "laohu-claude", Binding: api.Binding{Harness: "claude", SessionID: "s-l"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	laohuBot := h.client(res.Token)
	box := h.client(h.register(api.RegisterRequest{Name: "box", AllowFrom: []string{"glenn-bot", "laohu-claude"}, Binding: api.Binding{Harness: "exec", SessionID: "exec:box"}}))
	tick := func() { time.Sleep(3 * time.Millisecond) }

	// An old note to glenn-bot tagged "release" later on (only an old row
	// will match that tag).
	if _, err := glenn.ConsoleSend(c, api.SendRequest{To: "glenn-bot", Type: "note", Body: "old note"}); err != nil {
		t.Fatal(err)
	}
	glennBot.Pull(c, 5, 0) // reading it is activity too: do it now
	tick()
	// laohu asks his agent from the app (person ↔ agent).
	ask, err := laohu.ConsoleSend(c, api.SendRequest{To: "laohu-claude", Body: "check the release list"})
	if err != nil {
		t.Fatal(err)
	}
	tick()
	// His agent asks glenn's agent, who answers (agent ↔ agent).
	handoff, err := laohuBot.Send(c, api.SendRequest{To: "glenn-bot", Body: "which fields does the orders API return?"}, false)
	if err != nil {
		t.Fatal(err)
	}
	glennBot.Pull(c, 5, 0)
	if _, err := glennBot.Reply(c, handoff.Message.ID, "id, items, total", false); err != nil {
		t.Fatal(err)
	}
	tick()
	// glenn's agent runs three commands on the shell node: one row.
	for i := 0; i < 3; i++ {
		m, err := glennBot.Send(c, api.SendRequest{To: "box", Body: "uptime"}, false)
		if err != nil {
			t.Fatal(err)
		}
		box.Pull(c, 1, 0)
		box.Reply(c, m.Message.ID, `{"exit_code":0,"stdout":"up 3 days"}`, false)
		tick()
	}
	// laohu's agent runs a failing command there.
	fail, err := laohuBot.Send(c, api.SendRequest{To: "box", Body: "go vet ./..."}, false)
	if err != nil {
		t.Fatal(err)
	}
	box.Pull(c, 1, 0)
	box.Reply(c, fail.Message.ID, `{"exit_code":1,"stderr":"vet: bad"}`, false)

	type row struct{ kind, key, state string }
	rows := func(cl *client.Client, scope, label, q string) []row {
		t.Helper()
		l, err := cl.Recent(c, scope, label, q, 0, 50)
		if err != nil {
			t.Fatal(err)
		}
		var out []row
		for _, it := range l.Items {
			key := strings.Join(it.Participants, ">")
			if it.Kind == "pair" {
				key = it.Other + "|" + it.Node
			}
			out = append(out, row{it.Kind, key, it.State})
		}
		return out
	}
	keys := func(rs []row) []string {
		var out []string
		for _, r := range rs {
			out = append(out, r.key)
		}
		return out
	}

	// The owner sees everything, newest first; the shell commands of one
	// caller are one row, and the old note is there too.
	all := rows(glenn, "", "", "")
	want := []string{"laohu-claude|box", "glenn-bot|box", "laohu-claude>glenn-bot", "@console:laohu>laohu-claude", "@console:glenn>glenn-bot"}
	if !slices.Equal(keys(all), want) {
		t.Fatalf("owner, all: %v\nwant %v", keys(all), want)
	}
	if all[0].kind != "pair" || all[0].state != "done" || all[2].kind != "thread" || all[2].state != "done" || all[3].state != "queued" {
		t.Fatalf("kinds and states: %+v", all)
	}

	// laohu sees what he or his node takes part in; not glenn-bot's commands
	// or glenn's note.
	if got := keys(rows(laohu, "", "", "")); !slices.Equal(got, []string{"laohu-claude|box", "laohu-claude>glenn-bot", "@console:laohu>laohu-claude"}) {
		t.Fatalf("laohu: %v", got)
	}
	// matthew owns nothing and wrote nothing: an empty list, and the older
	// endpoints give him nothing either.
	if got := rows(matthew, "", "", ""); len(got) != 0 {
		t.Fatalf("matthew sees %v", got)
	}
	if cs, err := matthew.Conversations(c, "", "", 0, 0); err != nil || len(cs) != 0 {
		t.Fatalf("matthew, conversations: %v %+v", err, cs)
	}
	if _, err := matthew.Thread(c, ask.Message.ThreadID); err == nil {
		t.Fatal("matthew must not read laohu's thread")
	}
	var ce *client.Error
	if _, err := matthew.Exchanges(c, "laohu-claude", "glenn-bot", 0, 0); err == nil || !errors.As(err, &ce) || ce.Code != "permission_denied" {
		t.Fatalf("matthew, exchanges: %v", err)
	}
	if ps, err := matthew.Peers(c, "laohu-claude"); err != nil || len(ps) != 0 {
		t.Fatalf("matthew, peers: %v %+v", err, ps)
	}
	ov, err := matthew.Overview(c, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(ov.Recent) != 0 {
		t.Fatalf("matthew, overview recent: %+v", ov.Recent)
	}
	for _, a := range ov.Agents {
		if len(a.Current) != 0 {
			t.Fatalf("matthew, current work of %s: %+v", a.Agent.Name, a.Current)
		}
	}
	// laohu still reads his own thread and exchanges.
	if th, err := laohu.Thread(c, ask.Message.ThreadID); err != nil || len(th) != 1 {
		t.Fatalf("laohu, own thread: %v %+v", err, th)
	}
	if ex, err := laohu.Exchanges(c, "laohu-claude", "glenn-bot", 0, 0); err != nil || len(ex.Messages) != 2 {
		t.Fatalf("laohu, own exchange: %v %+v", err, ex)
	}

	// "Mine" for the owner: his console and his nodes (box is his too, so
	// laohu's commands on it count); laohu's talk with his own agent does not.
	if got := keys(rows(glenn, "mine", "", "")); !slices.Equal(got, []string{"laohu-claude|box", "glenn-bot|box", "laohu-claude>glenn-bot", "@console:glenn>glenn-bot"}) {
		t.Fatalf("owner, mine: %v", got)
	}

	// Groups and tags, by the nodes' current labels.
	if _, err := laohu.SetLabels(c, "laohu-claude", api.LabelsRequest{Add: []string{"group=web"}}); err != nil {
		t.Fatal(err)
	}
	if got := keys(rows(glenn, "", "group:web", "")); !slices.Equal(got, []string{"laohu-claude|box", "laohu-claude>glenn-bot", "@console:laohu>laohu-claude"}) {
		t.Fatalf("group:web: %v", got)
	}
	// "No group": rows with a node that has none (glenn-bot, box).
	if got := keys(rows(glenn, "", "group:", "")); !slices.Equal(got, []string{"laohu-claude|box", "glenn-bot|box", "laohu-claude>glenn-bot", "@console:glenn>glenn-bot"}) {
		t.Fatalf("group: (none): %v", got)
	}
	// A tag only an old row has is found on the first page, even with a
	// small page: the hub filters before paging.
	if _, err := glenn.SetLabels(c, "glenn-bot", api.LabelsRequest{Add: []string{"release"}}); err != nil {
		t.Fatal(err)
	}
	if l, err := glenn.Recent(c, "", "tag:release", "note", 0, 1); err != nil || len(l.Items) != 1 || l.Items[0].Last.Body != "old note" {
		t.Fatalf("an old row by tag: %v %+v", err, l)
	}
	// Moving laohu's node to another group moves its rows.
	if _, err := laohu.SetLabels(c, "laohu-claude", api.LabelsRequest{Remove: []string{"group=web"}, Add: []string{"group=api"}}); err != nil {
		t.Fatal(err)
	}
	if got := rows(glenn, "", "group:web", ""); len(got) != 0 {
		t.Fatalf("group:web after the change: %v", got)
	}
	// Filters add up: laohu, group api, searching for the shell node.
	if got := keys(rows(laohu, "mine", "group:api", "box")); !slices.Equal(got, []string{"laohu-claude|box"}) {
		t.Fatalf("combined: %v", got)
	}

	// Paging: no row twice, none missing.
	var seen []string
	var before int64
	for {
		l, err := glenn.Recent(c, "", "", "", before, 2)
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range l.Items {
			seen = append(seen, it.ThreadID+it.Other+it.Node)
		}
		if !l.More {
			break
		}
		before = l.Items[len(l.Items)-1].UpdatedAt.UnixMilli()
	}
	if len(seen) != len(want) {
		t.Fatalf("paging gave %d rows, want %d: %v", len(seen), len(want), seen)
	}
}
