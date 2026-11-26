package cli

import (
	"encoding/json"
	"github.com/myco-ai/myco/internal/api"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestWatchCommandFlags(t *testing.T) {
	var received api.WatchStartRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/me" {
			_ = json.NewEncoder(w).Encode(api.Agent{Name: "watcher"})
			return
		}
		if r.URL.Path != "/v1/watches" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Error(err)
		}
		_ = json.NewEncoder(w).Encode(api.Watch{ID: "wat_demo", Kind: received.Kind, Command: received.Command, Match: received.Match, EverySec: received.EverySec})
	}))
	defer server.Close()
	h := &harnessT{t, map[string]string{"MYCO_CONFIG": filepath.Join(t.TempDir(), "config.yaml"), "MYCO_HUB": server.URL, "MYCO_TOKEN": "fixture-agent-token"}}
	code, out, errText := h.run("watch", "start", "--what", "ready", "--cmd", "echo ready", "--match", "^ready$", "--every", "2s")
	if code != ExitOK || received.Kind != "command" || received.Command != "echo ready" || received.Match != "^ready$" || received.EverySec != 2 || received.ForSec != 1800 {
		t.Fatalf("flags: %d %+v %s %s", code, received, out, errText)
	}
	for _, args := range [][]string{{"--cmd", "echo ready", "--until", "task:x"}, {"--cmd", "echo ready", "--match", "["}, {"--cmd", "echo ready", "--every", "500ms"}, {"--until", "task:x", "--match", "ready"}} {
		code, _, stderr := h.run(append([]string{"watch", "start", "--what", "ready"}, args...)...)
		if code != ExitUsage || !strings.Contains(stderr, "invalid_argument") {
			t.Fatalf("invalid flags %v: %d %s", args, code, stderr)
		}
	}
}

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
