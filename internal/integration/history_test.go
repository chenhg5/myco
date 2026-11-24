package integration_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/client"
)

// Changes to a node's card are applied at once, by the agent or by people,
// and each is recorded: who, and what changed, field by field. Its owner and
// admins read the history; others do not.
func TestNodeHistory(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	alice := h.admin()
	if _, err := alice.UpdateMember(c, "owner", api.MemberUpdateRequest{Name: "alice"}); err != nil {
		t.Fatal(err)
	}
	member := func(name string) *client.Client {
		inv, err := alice.InviteMember(c, api.InviteRequest{Name: name})
		if err != nil {
			t.Fatal(err)
		}
		p, err := h.client("").Pair(c, inv.Link[strings.Index(inv.Link, "/c/")+3:], name+"-mac")
		if err != nil {
			t.Fatal(err)
		}
		return h.client(p.Token)
	}
	bob, carol := member("bob"), member("carol")
	enr, err := bob.CreateEnrollment(c, api.CreateEnrollmentRequest{NamePrefix: "bob-", MaxUses: 1}, false)
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.client(enr.Token).Enroll(c, api.EnrollRequest{Name: "bob-claude", Binding: api.Binding{Harness: "claude", SessionID: "s1"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	bot := h.client(res.Token)
	str := func(s string) *string { return &s }

	// The agent changes itself; nothing waits for anyone.
	if a, err := bot.UpdateProfile(c, "", api.ProfileRequest{Description: str("Builds the iOS app")}); err != nil || a.Description != "Builds the iOS app" {
		t.Fatalf("own description: %v %+v", err, a)
	}
	if _, err := bot.SetLabels(c, "", api.LabelsRequest{Add: []string{"group=ios"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := bot.SetSkills(c, "", []api.Skill{{Name: "ios-release", Summary: "TestFlight builds"}}); err != nil {
		t.Fatal(err)
	}
	// Setting the same value again records nothing.
	if _, err := bot.UpdateProfile(c, "", api.ProfileRequest{Description: str("Builds the iOS app")}); err != nil {
		t.Fatal(err)
	}
	// Its owner edits it too, and makes it visible to the team.
	if _, err := bob.UpdateProfile(c, "bob-claude", api.ProfileRequest{DisplayName: str("iOS 助手")}); err != nil {
		t.Fatal(err)
	}
	if _, err := bob.SetSkills(c, "bob-claude", []api.Skill{{Name: "ios-release", Summary: "Build and upload TestFlight builds"}, {Name: "crash-triage"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := bob.SetVisibility(c, "bob-claude", "team"); err != nil {
		t.Fatal(err)
	}

	hist, err := bob.AgentHistory(c, "bob-claude", 0)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range hist {
		got = append(got, e.ActorKind+" "+e.Actor+" "+e.Action+" | "+e.Detail)
	}
	want := []string{
		"member bob agent.visibility | team",
		"member bob agent.edit | skills: +crash-triage ~ios-release",
		"member bob agent.edit | display_name: — → iOS 助手",
		"agent bob-claude agent.edit | skills: +ios-release",
		"agent bob-claude agent.edit | labels: +group=ios",
		"agent bob-claude agent.edit | description: Builds the iOS app",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("history:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	// The hub's owner reads it; another member does not.
	if hist, err := alice.AgentHistory(c, "bob-claude", 2); err != nil || len(hist) != 2 {
		t.Fatalf("owner reads: %v %d", err, len(hist))
	}
	if _, err := carol.AgentHistory(c, "bob-claude", 0); err == nil {
		t.Fatal("another member read bob's node history")
	}
}
