package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myco-ai/myco/internal/api"
)

func TestAgentForkNetworkErrorsKeepGeneratedBatchKey(t *testing.T) {
	for _, phase := range []string{"create", "wait"} {
		t.Run(phase, func(t *testing.T) {
			var received api.AgentForkRequest
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/me" || r.URL.Path == "/v1/agents/source" {
					_ = json.NewEncoder(w).Encode(api.Agent{Name: "source"})
					return
				}
				if r.URL.Path == "/v1/agents/fork" {
					if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
						t.Error(err)
					}
					if phase == "wait" {
						_ = json.NewEncoder(w).Encode(api.AgentForkResult{Forks: []*api.AgentFork{{ID: "fork-demo", Name: "copy", State: "planned", MessageID: "msg-demo"}}})
						return
					}
				}
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "unavailable", "message": "temporary failure", "retryable": true})
			}))
			defer server.Close()
			h := &harnessT{t, map[string]string{"MYCO_HUB": server.URL, "MYCO_TOKEN": "fixture-agent-token", "MYCO_CONFIG": filepath.Join(t.TempDir(), "agent.yaml")}}
			code, _, stderr := h.run("agent", "fork", "--from", "source", "--timeout", "1s")
			var out Error
			if err := json.Unmarshal([]byte(stderr), &out); err != nil {
				t.Fatal(err, stderr)
			}
			if code != ExitUnavailable || out.Code != "unavailable" || !out.Retryable || received.RequestID == "" || out.Input["request_id"] != received.RequestID || !strings.Contains(out.Suggestion, "--idempotency-key "+received.RequestID) {
				t.Fatalf("lost recovery identity or error semantics: %d %s", code, stderr)
			}
		})
	}
}
