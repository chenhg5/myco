package integration_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/client"
)

// Projects: people create them and add their nodes; a project's
// coordinators edit its content and its members' cards and add nodes of
// their own owner; members read the project and its documents; who sees a
// project follows its visibility.
func TestProjects(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	alice := h.admin()
	if _, err := alice.UpdateMember(c, "owner", api.MemberUpdateRequest{Name: "alice"}); err != nil {
		t.Fatal(err)
	}
	enroll := func(owner *client.Client, name string) *client.Client {
		enr, err := owner.CreateEnrollment(c, api.CreateEnrollmentRequest{NamePrefix: name, MaxUses: 1}, false)
		if err != nil {
			t.Fatal(err)
		}
		res, err := h.client(enr.Token).Enroll(c, api.EnrollRequest{Name: name, Binding: api.Binding{Harness: "claude", SessionID: "s-" + name}}, false)
		if err != nil {
			t.Fatal(err)
		}
		return h.client(res.Token)
	}
	inv, err := alice.InviteMember(c, api.InviteRequest{Name: "bob"})
	if err != nil {
		t.Fatal(err)
	}
	p, err := h.client("").Pair(c, inv.Link[strings.Index(inv.Link, "/c/")+3:], "bob-mac")
	if err != nil {
		t.Fatal(err)
	}
	bob := h.client(p.Token)
	lead, worker := enroll(alice, "lead"), enroll(alice, "worker")
	enroll(alice, "helper")
	bobBot := enroll(bob, "bob-bot")
	enroll(bob, "bob-other")

	// Agents do not create projects; people do.
	if _, err := lead.CreateProject(c, api.CreateProjectRequest{Name: "x"}); err == nil {
		t.Fatal("an agent created a project")
	}
	pr, err := alice.CreateProject(c, api.CreateProjectRequest{Name: "myco", Title: "myco 开发", Goal: "ship projects"})
	if err != nil || pr.Visibility != "members" || pr.Owner != "alice" || !pr.CanManage {
		t.Fatalf("create: %v %+v", err, pr)
	}
	// Members-only: bob does not see it yet.
	if l, _ := bob.ListProjects(c, false); len(l) != 0 {
		t.Fatalf("bob sees %+v", l)
	}
	coord, member := "coordinator", "member"
	if _, err := alice.PutProjectMember(c, "myco", "lead", api.ProjectMemberRequest{Role: &coord}); err != nil {
		t.Fatal(err)
	}
	note := "后端"
	if _, err := alice.PutProjectMember(c, "myco", "worker", api.ProjectMemberRequest{Note: &note}); err != nil {
		t.Fatal(err)
	}
	// The hub's owner manages every node, so he may add bob's too; he
	// takes it out again so bob can add it himself below.
	if _, err := alice.PutProjectMember(c, "myco", "bob-bot", api.ProjectMemberRequest{}); err != nil {
		t.Fatalf("the hub's owner adds a member's node: %v", err)
	}
	if _, err := alice.RemoveProjectMember(c, "myco", "bob-bot"); err != nil {
		t.Fatalf("remove bob-bot again: %v", err)
	}
	// Open to the team: bob sees it and adds his own node.
	team := "team"
	if _, err := alice.UpdateProject(c, "myco", api.UpdateProjectRequest{Visibility: &team}); err != nil {
		t.Fatal(err)
	}
	if _, err := bob.PutProjectMember(c, "myco", "bob-bot", api.ProjectMemberRequest{}); err != nil {
		t.Fatalf("bob adds his node: %v", err)
	}
	if _, err := bob.PutProjectMember(c, "myco", "helper", api.ProjectMemberRequest{}); err == nil {
		t.Fatal("bob added alice's node")
	}

	// The coordinator edits the content and the members' cards.
	brief := "repo: myco-ai/myco; run make check before pushing"
	if got, err := lead.UpdateProject(c, "myco", api.UpdateProjectRequest{Brief: &brief}); err != nil || got.Brief != brief || !got.CanEdit || got.CanManage {
		t.Fatalf("coordinator edits the brief: %v %+v", err, got)
	}
	if _, err := lead.PutProjectDoc(c, "myco", "conventions", "# Conventions\nCommits end with a co-author line."); err != nil {
		t.Fatalf("coordinator writes a doc: %v", err)
	}
	desc := "Builds the App"
	if _, err := lead.UpdateProfile(c, "worker", api.ProfileRequest{Description: &desc}); err != nil {
		t.Fatalf("coordinator edits a member's card: %v", err)
	}
	if _, err := lead.SetSkills(c, "bob-bot", []api.Skill{{Name: "ios-release"}}); err != nil {
		t.Fatalf("coordinator edits bob's member node: %v", err)
	}
	if _, err := lead.SetSkills(c, "bob-other", []api.Skill{{Name: "x"}}); err == nil {
		t.Fatal("coordinator edited a node outside the project")
	}
	// It adds alice's other node, not bob's; it does not change roles or visibility.
	if _, err := lead.PutProjectMember(c, "myco", "helper", api.ProjectMemberRequest{}); err != nil {
		t.Fatalf("coordinator adds a node of its owner: %v", err)
	}
	if _, err := lead.PutProjectMember(c, "myco", "bob-other", api.ProjectMemberRequest{}); err == nil {
		t.Fatal("coordinator added another member's node")
	}
	if _, err := lead.PutProjectMember(c, "myco", "worker", api.ProjectMemberRequest{Role: &coord}); err == nil {
		t.Fatal("coordinator made another coordinator")
	}
	members := "members"
	if _, err := lead.UpdateProject(c, "myco", api.UpdateProjectRequest{Visibility: &members}); err == nil {
		t.Fatal("coordinator changed the visibility")
	}

	// A plain member reads everything, edits nothing.
	if _, err := worker.UpdateProject(c, "myco", api.UpdateProjectRequest{Brief: &brief}); err == nil {
		t.Fatal("a member edited the brief")
	}
	mine, err := worker.ListProjects(c, true)
	if err != nil || len(mine) != 1 || mine[0].Name != "myco" {
		t.Fatalf("worker's projects: %v %+v", err, mine)
	}
	full, err := bobBot.GetProject(c, "myco", true)
	if err != nil || len(full.Docs) != 1 || !strings.Contains(full.Docs[0].Body, "co-author") || len(full.Members) != 4 {
		t.Fatalf("pull: %v %+v", err, full)
	}
	for _, m := range full.Members {
		if m.Name == "worker" && (m.Note != "后端" || m.Agent == nil || m.Agent.Description != "Builds the App") {
			t.Fatalf("worker as seen: %+v", m)
		}
		if m.Name == "lead" && m.Role != "coordinator" {
			t.Fatalf("lead as seen: %+v", m)
		}
	}
	// Taking a coordinator back to member is people's call too, and works.
	if _, err := alice.PutProjectMember(c, "myco", "lead", api.ProjectMemberRequest{Role: &member}); err != nil {
		t.Fatal(err)
	}
	if _, err := lead.SetSkills(c, "worker", nil); err == nil {
		t.Fatal("no longer a coordinator, still edits cards")
	}
	// Removing a node takes it out of its projects.
	if _, err := bob.RemoveAgent(c, "bob-bot", false); err != nil {
		t.Fatal(err)
	}
	if got, _ := alice.GetProject(c, "myco", false); len(got.Members) != 3 {
		t.Fatalf("after removing a node: %+v", got.Members)
	}
	if err := alice.DeleteProject(c, "myco"); err != nil {
		t.Fatal(err)
	}
}
