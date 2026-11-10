package integration_test

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/client"
)

// Skills: an agent describes what it can be asked to do, its owner (or an
// admin) can edit that too, and discovery filters by it. Capabilities from
// before skills carry over as bare skills, and stay the list of names.
func TestSkills(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	admin := h.admin()

	// Registered with capabilities only (an old client): bare skills.
	tok := h.register(api.RegisterRequest{Name: "coder", Capabilities: []string{"code", "github"}})
	coder := h.client(tok)
	a, err := admin.GetAgent(c, "coder")
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Skills) != 2 || a.Skills[0].Name != "code" || a.Skills[0].Summary != "" {
		t.Fatalf("capabilities as skills: %+v", a.Skills)
	}

	// The agent describes its own skills: they apply at once, and the
	// capabilities follow.
	got, err := coder.SetSkills(c, "", []api.Skill{
		{Name: "Code-Review", Summary: "Review Go and\nTypeScript changes", Examples: []string{"review PR 12", ""}},
		{Name: "ios-release", Summary: "Build and upload iOS test builds"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.Capabilities, []string{"code-review", "ios-release"}) {
		t.Fatalf("capabilities: %v", got.Capabilities)
	}
	if got.Skills[0].Summary != "Review Go and TypeScript changes" || !slices.Equal(got.Skills[0].Examples, []string{"review PR 12"}) {
		t.Fatalf("normalized skill: %+v", got.Skills[0])
	}

	// Discovery: the capability filter matches skill names.
	list, err := coder.ListAgents(c, "ios-release", false, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "coder" || len(list[0].Skills) != 2 {
		t.Fatalf("filter by skill: %+v", list)
	}
	if list, _ := coder.ListAgents(c, "code", false, ""); len(list) != 0 {
		t.Fatalf("the old capability is gone: %+v", list)
	}

	// Other changes to the agent keep the skills' descriptions.
	if _, err := coder.SetLabels(c, "", api.LabelsRequest{Add: []string{"group=web"}}); err != nil {
		t.Fatal(err)
	}
	if a, _ := admin.GetAgent(c, "coder"); a.Skills[1].Summary != "Build and upload iOS test builds" {
		t.Fatalf("a label change dropped the summaries: %+v", a.Skills)
	}

	// The admin edits them too.
	if got, err = admin.SetSkills(c, "coder", []api.Skill{{Name: "code-review", Summary: "Reviews"}}); err != nil || len(got.Skills) != 1 {
		t.Fatalf("admin: %v %+v", err, got)
	}

	// Another agent cannot edit coder's skills; invalid input is refused.
	other := h.client(h.register(api.RegisterRequest{Name: "other"}))
	var ce *client.Error
	if _, err := other.SetSkills(c, "coder", []api.Skill{{Name: "x"}}); err == nil {
		t.Fatal("an agent edited another agent's skills")
	}
	for _, bad := range [][]api.Skill{
		{{Name: ""}},
		{{Name: "has space"}},
		{{Name: "a"}, {Name: "A"}},
		{{Name: "a", Summary: strings.Repeat("x", 201)}},
		{{Name: "a", Examples: []string{"1", "2", "3", "4"}}},
	} {
		if _, err := coder.SetSkills(c, "", bad); err == nil || !errors.As(err, &ce) || ce.Code != "invalid_argument" {
			t.Fatalf("accepted %+v: %v", bad, err)
		}
	}

	// An empty list clears them.
	if got, err = coder.SetSkills(c, "", nil); err != nil || len(got.Skills) != 0 || len(got.Capabilities) != 0 {
		t.Fatalf("clear: %v %+v", err, got)
	}
}
