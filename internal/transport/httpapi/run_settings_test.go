package httpapi_test

import (
	"context"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/client"
)

func TestRunSettings(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	coder := h.register("coder", map[string]any{"binding": map[string]any{"harness": "claude", "session_id": "s-coder"}})
	other := h.register("other", nil)

	// The admin (or the agent's owner) chooses; the agent sees it.
	h.do("PUT", "/v1/agents/coder/run-settings", adminToken, map[string]any{"model": "sonnet", "effort": "high"}).want(t, 200)
	got := h.do("GET", "/v1/me", coder, nil).want(t, 200)
	if str(got.body, "run_settings", "model") != "sonnet" || str(got.body, "run_settings", "effort") != "high" {
		t.Fatalf("run settings: %s", got.raw)
	}
	// Efforts are per CLI.
	h.do("PUT", "/v1/agents/coder/run-settings", adminToken, map[string]any{"model": "sonnet", "effort": "minimal"}).wantErr(t, 400, "invalid_argument")
	h.do("PUT", "/v1/agents/other/run-settings", adminToken, map[string]any{"effort": "high"}).wantErr(t, 400, "invalid_argument")
	// Another agent cannot choose for it; the agent can for itself.
	h.do("PUT", "/v1/agents/coder/run-settings", other, map[string]any{"model": "x"}).wantErr(t, 403, "permission_denied")
	h.do("PUT", "/v1/me/run-settings", coder, map[string]any{"model": "opus", "effort": ""}).want(t, 200)
	got = h.do("GET", "/v1/agents/coder", adminToken, nil).want(t, 200)
	if str(got.body, "run_settings", "model") != "opus" || str(got.body, "run_settings", "effort") != "" {
		t.Fatalf("after the agent's own change: %s", got.raw)
	}
	// Clearing both follows the machine again: no run_settings at all.
	h.do("PUT", "/v1/agents/coder/run-settings", adminToken, map[string]any{"model": "", "effort": ""}).want(t, 200)
	if got := h.do("GET", "/v1/agents/coder", adminToken, nil).want(t, 200); got.body["run_settings"] != nil {
		t.Fatalf("cleared: %s", got.raw)
	}
	admin, _ := client.New(h.url, adminToken)
	entries, err := admin.Audit(context.Background(), client.AuditFilter{})
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range entries {
		if e.Action == "agent.run_settings" && e.Target == "coder" {
			n++
		}
	}
	if n != 3 {
		t.Fatalf("audit entries: %d %+v", n, entries)
	}
}

func TestRuntimeCarriesEffortAndQuota(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	coder := h.register("coder", map[string]any{"binding": map[string]any{"harness": "codex", "session_id": "s-coder"}})
	rt := map[string]any{"cli": "codex", "model": "m", "model_source": "app", "effort": "high", "effort_source": "app",
		"quota": map[string]any{"source": "codex", "at": "2026-11-13T09:00:00Z", "windows": []any{
			map[string]any{"name": "primary", "label": "7d", "used": 0.07, "resets_at": "2026-11-20T09:00:00Z"}}}}
	h.do("PUT", "/v1/me/runtime", coder, rt).want(t, 200)
	got := h.do("GET", "/v1/agents/coder", adminToken, nil).want(t, 200)
	for _, want := range []string{`"effort":"high"`, `"effort_source":"app"`, `"label":"7d"`, `"used":0.07`, `"source":"codex"`} {
		if !strings.Contains(string(got.raw), want) {
			t.Fatalf("runtime lacks %s: %s", want, got.raw)
		}
	}
}
