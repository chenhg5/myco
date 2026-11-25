package app

import (
	"testing"

	"github.com/myco-ai/myco/internal/domain"
)

// A template id is one of myco's own only when the hub offers it so.
func TestIsOfficial(t *testing.T) {
	s := &TemplateService{}
	s.SetOfficial([]OfficialTemplate{{Manifest: &domain.Manifest{ID: "myco/notes"}}})
	if !s.IsOfficial("tp_myco_notes") {
		t.Fatal("myco's notes")
	}
	for _, id := range []string{"myco/notes", "tp_myco_disk", "tp_alice_notes", ""} {
		if s.IsOfficial(id) {
			t.Fatalf("%q is not one of myco's apps here", id)
		}
	}
}
