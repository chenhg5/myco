package templates_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/myco-ai/myco/internal/templates"
	"github.com/myco-ai/myco/internal/testutil"
)

// Service tests exercise real manifest defaults and schema validation with tiny
// fixed files; app rendering and screenshot checks live in myco-ai/apps.
func TestFixedOfficialFixtures(t *testing.T) {
	list, err := templates.FromDir(os.DirFS(testutil.OfficialDir(t)))
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 4 {
		t.Fatalf("want four service fixtures, got %d", len(list))
	}
	for _, item := range list {
		if len(item.Files[item.Manifest.Entry]) == 0 || len(item.Files[item.Manifest.Agent]) == 0 {
			t.Fatal("incomplete fixture", item.Manifest.ID)
		}
		var sample map[string][]map[string]json.RawMessage
		if err := json.Unmarshal(item.Files[item.Manifest.Sample], &sample); err != nil {
			t.Fatal(err)
		}
		for collection, records := range sample {
			schema := item.Manifest.Data[collection]
			for _, record := range records {
				delete(record, "id")
				body, _ := json.Marshal(record)
				if err := schema.CheckRecord(collection, string(body)); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}
