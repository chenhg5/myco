package templates_test

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/templates"
)

// Explicit network acceptance for a cutover/release, never ordinary Go tests.
func TestLiveCatalogSources(t *testing.T) {
	if os.Getenv("APPS_LIVE_ACCEPTANCE") != "1" {
		t.Skip("set APPS_LIVE_ACCEPTANCE=1 for public release download acceptance")
	}
	var first [32]byte
	for i, base := range []string{templates.DownloadURL(func(string) string { return "" }), "https://github.com/chenhg5/myco/releases/download"} {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		state := t.TempDir()
		catalog := &templates.Catalog{Download: base, StateDir: state, HubVersion: "v99.0.0"}
		if err := catalog.Load(); err != nil {
			cancel()
			t.Fatal(err)
		}
		result := catalog.Sync(ctx)
		cancel()
		if result.Error != "" || result.Apps < 11 {
			t.Fatalf("%s: %+v", base, result)
		}
		archive, err := os.ReadFile(filepath.Join(state, "official-apps.zip"))
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(archive)
		if i == 0 {
			first = hash
		} else if hash != first {
			t.Fatalf("legacy mirror differs from apps source: %x != %x", hash, first)
		}
		t.Logf("%s: %d apps, %d bytes, SHA256 %x", base, result.Apps, len(archive), hash)
	}
}
