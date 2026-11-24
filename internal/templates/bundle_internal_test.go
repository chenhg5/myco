package templates

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Production embedding must match the committed lock. An unprepared source
// checkout has no bundled apps at all; fixture HTML must never be its fallback.
func TestPreparedBundleMatchesLock(t *testing.T) {
	list, err := Official()
	if err != nil {
		t.Fatal(err)
	}
	if len(builtinZip) == 0 {
		if len(list) != 0 || bundledVersion != "" {
			t.Fatal("unprepared build contains a bundle")
		}
		return
	}
	var lock struct {
		Tag       string   `json:"tag"`
		Builtin   []string `json:"builtin"`
		Artifacts map[string]struct {
			SHA256 string `json:"sha256"`
		} `json:"artifacts"`
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "apps.lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &lock); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(builtinZip)
	if bundledVersion != lock.Tag || hex.EncodeToString(hash[:]) != lock.Artifacts["builtin.zip"].SHA256 {
		t.Fatal("prepared bundle does not match apps.lock.json; run make prepare-apps")
	}
	actual := map[string]bool{}
	for _, item := range list {
		actual[item.Manifest.ID] = true
	}
	if len(actual) != len(lock.Builtin) {
		t.Fatalf("builtin count: got %d, want %d", len(actual), len(lock.Builtin))
	}
	for _, name := range lock.Builtin {
		if !actual["myco/"+name] {
			t.Fatal("missing builtin", name)
		}
	}
}
