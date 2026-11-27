package httpapi_test

import (
	"bytes"
	"encoding/json"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/hubserver"
	"github.com/myco-ai/myco/internal/testutil"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Exercise the real router without opening a socket or depending on proxy env.
func TestMachineHealthRuntimeContractWithoutListener(t *testing.T) {
	h, e := hubserver.Build(hubserver.Options{DBPath: ":memory:", AdminToken: adminToken, Policy: app.DefaultMessagePolicy(), OfficialDir: testutil.OfficialDir(t), OnlineWindow: time.Minute, FilesDir: t.TempDir(), Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if e != nil {
		t.Fatal(e)
	}
	defer h.Close()
	call := func(method, path, token string, body any) *httptest.ResponseRecorder {
		t.Helper()
		data, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(data))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.Handler.ServeHTTP(w, r)
		return w
	}
	registered := call("POST", "/v1/agents", adminToken, map[string]any{"name": "worker", "binding": map[string]any{"harness": "codex", "session_id": "test-session"}})
	if registered.Code != 201 {
		t.Fatal(registered.Code, registered.Body)
	}
	var reg struct {
		Token string `json:"token"`
		Agent struct {
			OwnerID string `json:"owner_id"`
		} `json:"agent"`
	}
	if json.Unmarshal(registered.Body.Bytes(), &reg) != nil || reg.Token == "" {
		t.Fatal("missing test credential")
	}
	health := map[string]any{"owner_id": reg.Agent.OwnerID, "at": "2026-01-01T00:00:00Z", "runner_instance": "runner", "complete": true, "alert": true, "load_ratio": .9, "load_metric": "load1_per_core", "cores": 8, "memory_total": 1000, "memory_available": 50, "processes": []any{map[string]any{"pid": 50, "name": "worker", "created": "birth", "cpu": 99, "memory": 10, "agent": "worker", "suspicious": true, "ref": "reference"}}}
	reported := call("PUT", "/v1/me/runtime", reg.Token, map[string]any{"cli": "codex", "machine_health": health})
	if reported.Code != 200 {
		t.Fatal(reported.Code, reported.Body)
	}
	got := call("GET", "/v1/me", reg.Token, nil)
	if got.Code != 200 || !strings.Contains(got.Body.String(), `"machine_health"`) {
		t.Fatal(got.Code, got.Body)
	}
	if strings.Contains(got.Body.String(), `"ref":"reference"`) || strings.Contains(got.Body.String(), `"created":"birth"`) {
		t.Fatal("per-agent projection leaked process details")
	}
	// Agent credentials cannot invoke the administrator/owner control endpoint.
	denied := call(http.MethodPost, "/v1/machines/nonexistent/spawn", reg.Token, map[string]any{"cli": "process-stop", "process_ref": "reference"})
	if denied.Code != 403 {
		t.Fatal("agent could invoke process control", denied.Code, denied.Body)
	}
	health["owner_id"] = "another-owner"
	stale := call("PUT", "/v1/me/runtime", reg.Token, map[string]any{"cli": "codex", "machine_health": health})
	if stale.Code != 403 {
		t.Fatal("stale owner report accepted", stale.Code)
	}
	health["owner_id"] = reg.Agent.OwnerID
	health["load_ratio"] = -1
	rejected := call("PUT", "/v1/me/runtime", reg.Token, map[string]any{"cli": "codex", "machine_health": health})
	if rejected.Code != 400 {
		t.Fatal("invalid health accepted", rejected.Code)
	}
	// Old runners clear the optional health report normally.
	cleared := call("PUT", "/v1/me/runtime", reg.Token, map[string]any{"cli": "codex"})
	if cleared.Code != 200 {
		t.Fatal(cleared.Code)
	}
	got = call("GET", "/v1/me", reg.Token, nil)
	if strings.Contains(got.Body.String(), `"machine_health"`) {
		t.Fatal("stale old-runner health was retained")
	}
}
