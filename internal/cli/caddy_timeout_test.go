package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRaiseCaddyTimeoutsOnlyChangesOurOwnFiles(t *testing.T) {
	dir := t.TempDir()
	ours := "hub.example.com {\n\treverse_proxy 127.0.0.1:7777 {\n\t\ttransport http {\n\t\t\tread_timeout 120s\n\t\t}\n\t}\n}\n"
	edited := "work.example.com {\n\treverse_proxy 127.0.0.1:7778 {\n\t\ttransport http {\n\t\t\tread_timeout 120s\n\t\t}\n\t}\n\tencode gzip\n}\n"
	already := "new.example.com {\n\treverse_proxy 127.0.0.1:7779 {\n\t\ttransport http {\n\t\t\tread_timeout 360s\n\t\t}\n\t}\n}\n"
	files := map[string]string{"default.caddy": ours, "named-work.caddy": edited, "named-new.caddy": already, "notes.txt": ours}
	for n, c := range files {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	changed, before, err := raiseCaddyTimeouts(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 1 || filepath.Base(changed[0]) != "default.caddy" {
		t.Fatalf("changed %v, want only default.caddy", changed)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "default.caddy"))
	if want := "hub.example.com {\n\treverse_proxy 127.0.0.1:7777 {\n\t\ttransport http {\n\t\t\tread_timeout 360s\n\t\t}\n\t}\n}\n"; string(got) != want {
		t.Errorf("default.caddy = %q", got)
	}
	for n, c := range files {
		if n == "default.caddy" {
			continue
		}
		if b, _ := os.ReadFile(filepath.Join(dir, n)); string(b) != c {
			t.Errorf("%s changed: %q", n, b)
		}
	}
	restoreFiles(before)
	if b, _ := os.ReadFile(filepath.Join(dir, "default.caddy")); string(b) != ours {
		t.Errorf("restore: %q", b)
	}
}
