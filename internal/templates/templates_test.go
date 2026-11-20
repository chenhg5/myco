package templates_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/templates"
)

// Every official template reads, has its page, the agent's
// guide and sample data that its own data structure accepts.
func TestOfficialTemplatesAreWhole(t *testing.T) {
	list, err := templates.Official()
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, o := range list {
		m := o.Manifest
		ids = append(ids, m.ID)
		if m.Category == "" {
			t.Errorf("%s has no category (its shelf in the App)", m.ID)
		}
		if m.Icon == "" || !strings.Contains(string(o.Files[m.Icon]), "<svg") {
			t.Errorf("%s has no icon of its own", m.ID)
		}
		for _, f := range []string{m.Entry, m.Agent, m.Sample} {
			if len(o.Files[f]) == 0 {
				t.Errorf("%s has no %s", m.ID, f)
			}
		}
		// The App shows what it looks like: a screenshot, light and dark.
		if len(o.Files["shots/1.webp"]) == 0 || len(o.Files["shots/1.dark.webp"]) == 0 {
			t.Errorf("%s has no screenshots (node templates/shots.mjs %s)", m.ID, strings.TrimPrefix(m.ID, "myco/"))
		}
		// Its page loads what is in it, nothing from the internet.
		page := string(o.Files[m.Entry])
		if !strings.Contains(page, "<script") || strings.Contains(page, "http://") || strings.Contains(page, "https://") {
			t.Errorf("%s: its page should load its own scripts only", m.ID)
		}
		var sample map[string][]map[string]json.RawMessage
		if err := json.Unmarshal(o.Files[m.Sample], &sample); err != nil {
			t.Fatalf("%s sample: %v", m.ID, err)
		}
		for coll, recs := range sample {
			col, ok := m.Data[coll]
			if !ok {
				t.Errorf("%s sample: no collection %s", m.ID, coll)
				continue
			}
			for _, r := range recs {
				delete(r, "id")
				raw, _ := json.Marshal(r)
				if err := col.CheckRecord(coll, string(raw)); err != nil {
					t.Errorf("%s sample %s: %v", m.ID, coll, err)
				}
			}
		}
	}
	var want []string
	for _, id := range templates.Order {
		for _, x := range ids {
			if x == id {
				want = append(want, id)
			}
		}
	}
	if len(ids) < 4 || strings.Join(ids, ",") != strings.Join(want, ",") {
		t.Fatalf("official templates: %v", ids)
	}
}
