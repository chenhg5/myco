package cli

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunnerHoldNeedsWakeCondition(t *testing.T) {
	h := &harnessT{t, map[string]string{"MYCO_MESSAGE_ID": "msg_parent", "MYCO_RUNNER_HOLD": "1"}}
	code, _, stderr := h.run("message", "hold")
	if code != ExitConflict || !strings.Contains(stderr, "invalid_state") || !strings.Contains(stderr, "wait in the foreground") {
		t.Fatalf("hold returned %d: %s", code, stderr)
	}
	delete(h.env, "MYCO_RUNNER_HOLD")
	code, _, stderr = h.run("message", "hold", "--until", "msg_child")
	if code != ExitConflict || !strings.Contains(stderr, "updated runner") {
		t.Fatalf("legacy runner hold returned %d: %s", code, stderr)
	}
}

func TestRunnerHoldOnOldHubDoesNotFallBack(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages/msg_parent/hold-until" {
			t.Errorf("unexpected fallback to %s", r.URL.Path)
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	h := &harnessT{t, map[string]string{
		"MYCO_CONFIG": filepath.Join(t.TempDir(), "config.yaml"),
		"MYCO_HUB":    server.URL, "MYCO_TOKEN": "fake-agent-token",
		"MYCO_MESSAGE_ID": "msg_parent", "MYCO_RUNNER_HOLD": "1",
	}}
	code, _, stderr := h.run("message", "hold", "--until", "msg_child")
	if code != ExitConflict || !strings.Contains(stderr, "update the hub") {
		t.Fatalf("old hub returned %d: %s", code, stderr)
	}
}
