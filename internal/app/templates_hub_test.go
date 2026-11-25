package app

import (
	"testing"

	"github.com/myco-ai/myco/internal/domain"
)

// A template made for a newer hub is left out by older ones; builds whose
// version is unknown (dev) take everything.
func TestHubTooOld(t *testing.T) {
	for _, c := range []struct {
		have, want string
		old        bool
	}{
		{"v0.20.6", "0.20.6", false}, {"v0.20.5", "0.20.6", true}, {"v0.20.5-12-gabc", "0.20.6", true},
		{"v0.21.0", "0.20.6", false}, {"dev", "9.9.9", false}, {"v0.20.5", "", false},
	} {
		if got := HubTooOld(c.have, c.want); got != c.old {
			t.Errorf("HubTooOld(%q, %q) = %v", c.have, c.want, got)
		}
	}
	if got := domain.RequiredHub([]byte(`{"id": "x", "requires": {"hub": "v1.2.3"}, "future": {"a": 1}}`)); got != "1.2.3" {
		t.Errorf("RequiredHub reads past fields it does not know: %q", got)
	}
}
