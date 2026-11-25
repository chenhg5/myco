package cli

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestWatchOldHubAndValidation(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	h := &harnessT{t, map[string]string{"MYCO_CONFIG": filepath.Join(t.TempDir(), "config.yaml"), "MYCO_HUB": server.URL, "MYCO_TOKEN": "test-token"}}
	for _, args := range [][]string{{"watch", "list"}, {"watch", "start", "--what", "Build", "--until", "task:msg_demo"}, {"watch", "stop", "--id", "wat_demo"}, {"watch", "extend", "--id", "wat_demo", "--for", "30m"}} {
		code, _, stderr := h.run(args...)
		if code != ExitUsage || !strings.Contains(stderr, "upgrade the workspace Hub") {
			t.Fatalf("old Hub %v: %d %s", args, code, stderr)
		}
	}
	for _, args := range [][]string{{"watch", "start", "--what", "Build", "--until", "command:foo"}, {"watch", "start", "--what", "Build", "--until", "task:msg_demo", "--for", "25h"}, {"watch", "extend", "--id", "wat_demo", "--for", "0s"}} {
		code, _, stderr := h.run(args...)
		if code != ExitUsage || !strings.Contains(stderr, "invalid_argument") {
			t.Fatalf("validation %v: %d %s", args, code, stderr)
		}
	}
}
func TestWatchSchemaListsDryRun(t *testing.T) {
	h := &harnessT{t, map[string]string{"MYCO_CONFIG": filepath.Join(t.TempDir(), "config.yaml")}}
	code, out, stderr := h.run("schema", "--all")
	if code != ExitOK {
		t.Fatal(stderr)
	}
	if !strings.Contains(out, "watch") || !strings.Contains(out, "--dry-run") {
		t.Fatalf("missing watch schema: %.200s", out)
	}
}
