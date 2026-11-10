package integration_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/client"
)

// A coordinator agent edits the cards of its owner's other agents (not
// their permissions, not other people's agents); only people make an agent
// a coordinator, and its edits are recorded under its own name.
func TestCoordinatorEditsTeamCards(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	glenn := h.admin()
	if _, err := glenn.UpdateMember(c, "owner", api.MemberUpdateRequest{Name: "glenn"}); err != nil {
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
	inv, err := glenn.InviteMember(c, api.InviteRequest{Name: "laohu"})
	if err != nil {
		t.Fatal(err)
	}
	p, err := h.client("").Pair(c, inv.Link[strings.Index(inv.Link, "/c/")+3:], "laohu-mac")
	if err != nil {
		t.Fatal(err)
	}
	laohu := h.client(p.Token)

	lead := enroll(glenn, "lead")
	enroll(glenn, "worker")
	enroll(laohu, "laohu-bot")
	desc := "Builds the iOS app"

	// Not a coordinator yet: refused.
	if _, err := lead.UpdateProfile(c, "worker", api.ProfileRequest{Description: &desc}); err == nil {
		t.Fatal("a plain agent edited another agent's card")
	}
	// An agent cannot make itself a coordinator.
	if _, err := lead.SetCoordinator(c, "lead", true); err == nil {
		t.Fatal("an agent made itself a coordinator")
	}
	if a, err := glenn.SetCoordinator(c, "lead", true); err != nil || !a.Coordinator {
		t.Fatalf("owner makes lead a coordinator: %v %+v", err, a)
	}

	// Now it edits glenn's other agents' cards.
	if a, err := lead.UpdateProfile(c, "worker", api.ProfileRequest{Description: &desc}); err != nil || a.Description != desc {
		t.Fatalf("coordinator edits description: %v %+v", err, a)
	}
	if _, err := lead.SetSkills(c, "worker", []api.Skill{{Name: "ios-release", Summary: "TestFlight builds"}}); err != nil {
		t.Fatalf("coordinator edits skills: %v", err)
	}
	if _, err := lead.SetLabels(c, "worker", api.LabelsRequest{Add: []string{"group=myco-dev"}}); err != nil {
		t.Fatalf("coordinator edits labels: %v", err)
	}
	// Not permissions, and not someone else's agents.
	anyone := []string{}
	if _, err := lead.UpdateProfile(c, "worker", api.ProfileRequest{AllowFrom: &anyone}); err == nil {
		t.Fatal("a coordinator changed allow_from")
	}
	if _, err := lead.SetVisibility(c, "worker", "private"); err == nil {
		t.Fatal("a coordinator changed visibility")
	}
	if _, err := lead.SetSkills(c, "laohu-bot", []api.Skill{{Name: "x"}}); err == nil {
		t.Fatal("a coordinator edited another member's agent")
	}

	// Its edits carry its name.
	hist, err := glenn.AgentHistory(c, "worker", 0)
	if err != nil || len(hist) != 3 {
		t.Fatalf("history: %v %+v", err, hist)
	}
	for _, e := range hist {
		if e.ActorKind != "agent" || e.Actor != "lead" || e.Action != "agent.edit" {
			t.Fatalf("recorded as %+v", e)
		}
	}

	// Switched off: refused again.
	if _, err := glenn.SetCoordinator(c, "lead", false); err != nil {
		t.Fatal(err)
	}
	if _, err := lead.SetSkills(c, "worker", nil); err == nil {
		t.Fatal("still a coordinator after switching it off")
	}
}
