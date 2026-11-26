package hubserver

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/perf"
	"github.com/myco-ai/myco/internal/testutil"
)

// The hub's owner sees how long requests (by route pattern) and statements
// (by name) took; ids in paths never show, and no one else may look.
func TestAdminPerf(t *testing.T) {
	timings := perf.NewTimings()
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	h, err := Build(Options{DBPath: ":memory:", AdminToken: "perf-admin-token", Policy: app.DefaultMessagePolicy(), OfficialDir: testutil.OfficialDir(t),
		OnlineWindow: time.Minute, FilesDir: t.TempDir(), Log: quiet, Perf: timings})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	srv := httptest.NewServer(logRequests(h.Handler, quiet, -1, timings.Requests))
	defer srv.Close()
	get := func(path, token string) (int, []byte) {
		t.Helper()
		req, _ := http.NewRequest("GET", srv.URL+path, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		r, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Body.Close()
		b, _ := io.ReadAll(r.Body)
		return r.StatusCode, b
	}
	get("/v1/agents", "perf-admin-token")
	get("/v1/threads/msg_secretid123", "perf-admin-token")
	get("/v1/messages?wait_seconds=0", "perf-admin-token") // (a long poll: left out)

	code, body := get("/v1/admin/perf?minutes=5&limit=50", "perf-admin-token")
	if code != 200 {
		t.Fatalf("perf: %d %s", code, body)
	}
	var r api.PerfReport
	if err := json.Unmarshal(body, &r); err != nil {
		t.Fatal(err)
	}
	routes := map[string]bool{}
	for _, s := range r.Requests {
		routes[s.Name] = true
		if s.Count < 1 || s.MaxMS < s.P50MS {
			t.Fatalf("request numbers: %+v", s)
		}
	}
	if !routes["GET /v1/agents"] || !routes["GET /v1/threads/{id}"] || r.Minutes != 5 {
		t.Fatalf("routes: %v (%d min)", routes, r.Minutes)
	}
	for name := range routes {
		if strings.Contains(name, "wait") || strings.HasPrefix(name, "GET /v1/messages") && !strings.Contains(name, "{") {
			t.Fatalf("a long poll was counted: %s", name)
		}
	}
	listed := false
	for _, s := range r.Statements {
		listed = listed || strings.HasPrefix(s.Name, "AgentRepo.List: SELECT agents #")
	}
	if !listed {
		t.Fatalf("statements: %+v", r.Statements)
	}
	if strings.Contains(string(body), "secretid") {
		t.Fatalf("an id from a path was kept: %s", body)
	}

	if code, _ := get("/v1/admin/perf", ""); code != 401 {
		t.Fatalf("without a token: %d", code)
	}
	if code, _ := get("/v1/admin/perf?minutes=0", "perf-admin-token"); code != 400 {
		t.Fatalf("minutes=0: %d", code)
	}
	if code, _ := get("/v1/admin/perf?limit=9999", "perf-admin-token"); code != 400 {
		t.Fatalf("limit=9999: %d", code)
	}
}
