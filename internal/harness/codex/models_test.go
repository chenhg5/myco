package codex

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCachedModelsKeepCodexOrderAndDropHidden(t *testing.T) {
	dir := t.TempDir()
	cache := `{"fetched_at": "2026-11-20T08:00:00Z", "etag": "x", "client_version": "0.160.0", "models": [
	  {"slug": "old-mini", "display_name": "Old mini", "visibility": "list", "priority": 9},
	  {"slug": "astra", "display_name": "GPT Astra", "visibility": "list", "priority": 1,
	   "supported_reasoning_levels": [{"effort": "low", "description": "fast"}, {"effort": "high"}]},
	  {"slug": "internal", "visibility": "hide", "priority": 0},
	  {"slug": "no-api", "visibility": "list", "supported_in_api": false},
	  {"slug": "plain"}]}`
	if err := os.WriteFile(filepath.Join(dir, "models_cache.json"), []byte(cache), 0o600); err != nil {
		t.Fatal(err)
	}
	got := ModelsIn(dir)
	want := []Model{{ID: "astra", Name: "GPT Astra", Efforts: []string{"low", "high"}}, {ID: "old-mini", Name: "Old mini"}, {ID: "plain"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("models:\n got %+v\nwant %+v", got, want)
	}
	if ModelsIn(t.TempDir()) != nil || parseModels([]byte("not json")) != nil {
		t.Fatal("no cache, or a broken one, gives no list")
	}
}

func TestStderrProblemsAreShown(t *testing.T) {
	for _, l := range []string{"stream error: stream disconnected before completion; retrying 2/5 in 1.6s", "ERROR: unexpected status 401 Unauthorized"} {
		if _, ok := stderrProblem(l); !ok {
			t.Fatalf("%q should show", l)
		}
	}
	for _, l := range []string{"Reading prompt from stdin...", "", "   "} {
		if _, ok := stderrProblem(l); ok {
			t.Fatalf("%q should not show", l)
		}
	}
}
