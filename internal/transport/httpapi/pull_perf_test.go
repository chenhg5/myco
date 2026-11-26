package httpapi_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/hubserver"
	"github.com/myco-ai/myco/internal/perf"
	"github.com/myco-ai/myco/internal/testutil"
)

func TestPullMarksBodyWaitForRequestMetrics(t *testing.T) {
	built, err := hubserver.Build(hubserver.Options{DBPath: ":memory:", AdminToken: adminToken, Policy: app.DefaultMessagePolicy(), OfficialDir: testutil.OfficialDir(t), FilesDir: t.TempDir(), OnlineWindow: time.Minute, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	defer built.Close()
	var waiting atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(perf.WithRoute(r.Context()))
		built.Handler.ServeHTTP(w, r)
		waiting.Store(perf.Waiting(r.Context()))
	}))
	defer srv.Close()
	h := &hub{t: t, url: srv.URL}
	alice := h.register("alice", nil)
	bob := h.register("bob", nil)
	h.send(alice, "bob", "ready")
	h.do("POST", "/v1/inbox/pull", bob, map[string]any{"wait_seconds": 30}).want(t, 200)
	if !waiting.Load() {
		t.Fatal("body long poll was not marked")
	}
	h.do("POST", "/v1/inbox/pull", bob, map[string]any{"wait_seconds": 0}).want(t, 200)
	if waiting.Load() {
		t.Fatal("immediate pull was marked as waiting")
	}
	h.do("POST", "/v1/inbox/pull", bob, `{"wait_seconds":`).want(t, 400)
	if waiting.Load() {
		t.Fatal("invalid body was marked as waiting")
	}
	h.do("POST", "/v1/inbox/pull", "", map[string]any{"wait_seconds": 30}).want(t, 401)
	if waiting.Load() {
		t.Fatal("unauthorized request was marked as waiting")
	}
}
