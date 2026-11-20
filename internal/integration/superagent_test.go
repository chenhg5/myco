package integration_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/client"
)

// A person names one of their agents their superagent: its own token then
// works on the App's routes, as them: some at once, some held until they
// approve (carried out once), the rest never; it never speaks as them, and
// it cannot approve its own requests.
func TestSuperagentPowersAndApprovals(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	admin := h.admin()
	if _, err := admin.UpdateMember(c, "owner", api.MemberUpdateRequest{Name: "glenn"}); err != nil {
		t.Fatal(err)
	}
	ct, err := admin.CreateConsoleToken(c, "glenn-mac")
	if err != nil {
		t.Fatal(err)
	}
	glenn := h.client(ct.Token)
	shroom := h.client(h.register(api.RegisterRequest{Name: "shroom", Binding: api.Binding{Harness: "claude", SessionID: "s1"}}))
	h.register(api.RegisterRequest{Name: "helper", Binding: api.Binding{Harness: "codex", SessionID: "s2"}})

	// Before: an ordinary agent.
	if _, err := shroom.Members(c, false); err == nil {
		t.Fatal("an ordinary agent read the members")
	}
	if me, err := shroom.MeSuperagent(c); err != nil || me.Superagent {
		t.Fatalf("me: %+v %v", me, err)
	}
	if _, err := shroom.SetSuperagent(c, "shroom", false); err == nil {
		t.Fatal("an agent named itself a superagent")
	}
	if _, err := glenn.SetSuperagent(c, "shroom", false); err != nil {
		t.Fatal(err)
	}
	if sa, err := glenn.Superagent(c); err != nil || sa.Agent == nil || sa.Agent.Name != "shroom" {
		t.Fatalf("superagent: %+v %v", sa, err)
	}
	if me, err := shroom.MeSuperagent(c); err != nil || !me.Superagent || me.Owner != "glenn" {
		t.Fatalf("me: %+v %v", me, err)
	}

	// Allowed: reading, and making a project (its owner's).
	if ms, err := shroom.Members(c, false); err != nil || len(ms) == 0 {
		t.Fatalf("members: %v %v", ms, err)
	}
	p, err := shroom.CreateProject(c, api.CreateProjectRequest{Name: "launch", Title: "发布"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Owner != "glenn" {
		t.Fatalf("project owner: %+v", p)
	}
	// Never: tokens, inviting people, speaking as its owner.
	if _, err := shroom.CreateConsoleToken(c, "x"); err == nil {
		t.Fatal("a superagent made an app token")
	}
	if _, err := shroom.InviteMember(c, api.InviteRequest{Name: "eve"}); err == nil {
		t.Fatal("a superagent invited someone")
	}
	if _, err := shroom.ConsoleSend(c, api.SendRequest{To: "helper", Type: "task", Body: "as glenn"}); err == nil {
		t.Fatal("a superagent spoke as its owner")
	}

	// Held: deleting the project waits for glenn.
	shroom.Reason = "上线完了，项目不用了"
	err = shroom.DeleteProject(c, "launch")
	if e, ok := client.AsError(err); !ok || e.Code != "approval_pending" {
		t.Fatalf("delete: %v", err)
	}
	shroom.Reason = ""
	pending, err := glenn.Approvals(c, "pending")
	if err != nil || len(pending) != 1 || pending[0].What != "删除项目 launch" || pending[0].Reason != "上线完了，项目不用了" {
		t.Fatalf("pending: %+v %v", pending, err)
	}
	if _, err := shroom.DecideApproval(c, pending[0].ID, true); err == nil {
		t.Fatal("a superagent approved its own request")
	}
	if mine, err := shroom.MyApprovals(c); err != nil || len(mine) != 1 || mine[0].Status != "pending" {
		t.Fatalf("its approvals: %+v %v", mine, err)
	}
	done, err := glenn.DecideApproval(c, pending[0].ID, true)
	if err != nil || done.Status != "done" || done.Result != 200 {
		t.Fatalf("approve: %+v %v", done, err)
	}
	if _, err := glenn.DecideApproval(c, pending[0].ID, true); err == nil {
		t.Fatal("an approval was carried out twice")
	}
	if _, err := glenn.GetProject(c, "launch", false); err == nil {
		t.Fatal("the project is still there")
	}
	// It hears how it went (a note from glenn).
	ms, err := shroom.Pull(c, 5, time.Second)
	if err != nil || len(ms) == 0 || !strings.Contains(ms[len(ms)-1].Body, "同意了：删除项目 launch") {
		t.Fatalf("note: %+v %v", ms, err)
	}

	// Denied: nothing happens.
	if _, err := shroom.RemoveAgent(c, "helper", false); err == nil {
		t.Fatal("removing an agent was not held")
	}
	pending, _ = glenn.Approvals(c, "pending")
	if len(pending) != 1 {
		t.Fatalf("pending: %+v", pending)
	}
	if d, err := glenn.DecideApproval(c, pending[0].ID, false); err != nil || d.Status != "denied" {
		t.Fatalf("deny: %+v %v", d, err)
	}
	if ags, err := admin.ListAgents(c, "", false); err != nil || !hasAgent(ags, "helper") {
		t.Fatalf("helper is gone: %v", err)
	}

	// The audit log says it was through the superagent.
	entries, err := admin.Audit(c, client.AuditFilter{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	via := false
	for _, e := range entries {
		if strings.Contains(e.Actor, "经 superagent shroom") {
			via = true
		}
	}
	if !via {
		t.Fatalf("audit: %+v", entries)
	}

	// Taken away: an ordinary agent again.
	if err := glenn.ClearSuperagent(c); err != nil {
		t.Fatal(err)
	}
	if _, err := shroom.Members(c, false); err == nil {
		t.Fatal("still a superagent after it was taken away")
	}
}

func hasAgent(list []api.Agent, name string) bool {
	for _, a := range list {
		if a.Name == name {
			return true
		}
	}
	return false
}

// A superagent link (a person's): the agent that joins through it, from a
// cloud session or any computer, is that person's superagent at once.
func TestSuperagentJoinsThroughALink(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	admin := h.admin()
	if _, err := admin.UpdateMember(c, "owner", api.MemberUpdateRequest{Name: "glenn"}); err != nil {
		t.Fatal(err)
	}
	ct, err := admin.CreateConsoleToken(c, "glenn-mac")
	if err != nil {
		t.Fatal(err)
	}
	glenn := h.client(ct.Token)
	// An agent cannot make one.
	other := h.client(h.register(api.RegisterRequest{Name: "other", Binding: api.Binding{Harness: "claude", SessionID: "s0"}}))
	if _, err := other.CreateEnrollment(c, api.CreateEnrollmentRequest{Kind: "link", MaxUses: 1, Superagent: true}, false); err == nil {
		t.Fatal("an agent made a superagent link")
	}
	link, err := glenn.CreateEnrollment(c, api.CreateEnrollmentRequest{Kind: "link", MaxUses: 1, TTLSeconds: 3600, Superagent: true}, false)
	if err != nil {
		t.Fatal(err)
	}
	// The cloud session joins: its machine, then itself.
	m, err := h.client(link.Token).Join(c, "cloud-box", false)
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.client(m.Token).Enroll(c, api.EnrollRequest{Name: "cloud-box-chief", Binding: api.Binding{Harness: "claude-cloud", SessionID: "sess-1"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if sa, err := glenn.Superagent(c); err != nil || sa.Agent == nil || sa.Agent.Name != res.Agent.Name {
		t.Fatalf("superagent after joining: %+v %v", sa, err)
	}
	if me, err := h.client(res.Token).MeSuperagent(c); err != nil || !me.Superagent || me.Owner != "glenn" {
		t.Fatalf("the agent should know it is glenn's superagent: %+v %v", me, err)
	}
}
