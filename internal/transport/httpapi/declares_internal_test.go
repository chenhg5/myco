package httpapi

import (
	"testing"

	"github.com/myco-ai/myco/internal/api"
)

// A page's app declares all it uses, except a page of myco's own apps kept
// from before declarations (no os, and an older manifest or a newer
// version to upgrade to): that one is asked as before, not refused.
func TestDeclaresAll(t *testing.T) {
	newer := `{"id":"myco/disk","name":"d","version":"1.3.0","kind":"custom","requires":{"hub":"0.22.1"}}`
	older := `{"id":"myco/disk","name":"d","version":"1.2.2","kind":"custom","requires":{"hub":"0.20.7"}}`
	none := `{"id":"myco/disk","name":"d","version":"1.2.2","kind":"custom"}`
	for _, c := range []struct {
		name     string
		t        api.PageTemplate
		manifest string
		want     bool
	}{
		{"a workspace's app", api.PageTemplate{}, older, true},
		{"official, declares", api.PageTemplate{Official: true, OS: []string{"files"}}, newer, true},
		{"official, newest, uses nothing", api.PageTemplate{Official: true}, newer, true},
		{"official, kept from before declarations", api.PageTemplate{Official: true}, older, false},
		{"official, no requires", api.PageTemplate{Official: true}, none, false},
		{"official, a newer version to take", api.PageTemplate{Official: true, Latest: "1.4.0"}, newer, false},
		{"official, declares, a newer version", api.PageTemplate{Official: true, OS: []string{"files"}, Latest: "1.4.0"}, newer, true},
	} {
		if got := declaresAll(&c.t, c.manifest); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}
