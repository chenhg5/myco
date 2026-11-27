package hubserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/infra/sqlite"
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
	if r.Database == nil || r.Database.MaxOpenConnections != 1 || r.Database.StartedAt.IsZero() || r.Database.At.Before(r.Database.StartedAt) {
		t.Fatalf("missing workspace database snapshot: %+v", r.Database)
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

// Saturate a real single-connection pool. The owner-only metric endpoint
// must remain readable while another operation waits for that connection.
func TestAdminPerfReportsPoolWaitWithoutAcquiringConnection(t *testing.T) {
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	h, err := Build(Options{DBPath: ":memory:", AdminToken: "pool-admin", Policy: app.DefaultMessagePolicy(), OfficialDir: testutil.OfficialDir(t), FilesDir: t.TempDir(), Log: quiet, Perf: perf.NewTimings()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	started := time.Now().UTC()
	h.Server.SetDatabaseStats(func() api.DatabasePoolStats {
		st := db.Stats()
		return api.DatabasePoolStats{At: time.Now().UTC(), StartedAt: started, MaxOpenConnections: st.MaxOpenConnections, OpenConnections: st.OpenConnections, InUse: st.InUse, Idle: st.Idle, WaitCount: st.WaitCount, WaitDurationMS: float64(st.WaitDuration) / float64(time.Millisecond)}
	})
	conn, err := db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, e := db.ExecContext(ctx, "SELECT 1"); done <- e }()
	deadline := time.Now().Add(2 * time.Second)
	for db.Stats().WaitCount == 0 {
		if time.Now().After(deadline) {
			t.Fatal("operation did not enter the connection wait queue")
		}
		time.Sleep(time.Millisecond)
	}
	get := func(token string) (int, api.PerfReport) {
		t.Helper()
		r := httptest.NewRequest("GET", "/v1/admin/perf?minutes=1", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		h.Handler.ServeHTTP(w, r)
		var out api.PerfReport
		if w.Code == 200 {
			if e := json.Unmarshal(w.Body.Bytes(), &out); e != nil {
				t.Fatal(e)
			}
		}
		return w.Code, out
	}
	if code, _ := get("not-admin"); code != 401 {
		t.Fatalf("pool visible without admin token: %d", code)
	}
	code, out := get("pool-admin")
	d := out.Database
	if code != 200 || d == nil || d.MaxOpenConnections != 1 || d.InUse != 1 || d.Idle != 0 || d.WaitCount != 1 {
		t.Fatalf("pool snapshot while saturated: code=%d database=%+v", code, d)
	}
	cancel()
	if e := <-done; !errors.Is(e, context.Canceled) {
		t.Fatalf("wait error: %v", e)
	}
	_, out = get("pool-admin")
	if out.Database.WaitCount != 1 || out.Database.WaitDurationMS <= 0 {
		t.Fatalf("canceled wait not counted: %+v", out.Database)
	}
	// Optional metric providers are scoped to a Server, never a global.
	h.Server.SetDatabaseStats(nil)
	_, out = get("pool-admin")
	if out.Database != nil {
		t.Fatal("database field must be optional")
	}
}
