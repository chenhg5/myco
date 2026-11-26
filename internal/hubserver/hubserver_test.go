package hubserver

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/perf"
)

func TestRequestLogRedactsRecoveryCredential(t *testing.T) {
	var log bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&log, &slog.HandlerOptions{Level: slog.LevelDebug}))
	h := logRequests(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }), logger, 0, nil)
	req := httptest.NewRequest("GET", "/r/myco_resume_secret", nil)
	h.ServeHTTP(httptest.NewRecorder(), req)
	if strings.Contains(log.String(), "myco_resume_secret") || !strings.Contains(log.String(), "/r/<redacted>") {
		t.Fatalf("request log did not redact credential: %s", log.String())
	}
}

// At the usual info level a hub logs what is slow (not long polls, which
// wait on purpose) and what failed on its side, with the error it gave.
func TestRequestLogSlowAndFailed(t *testing.T) {
	var log bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&log, &slog.HandlerOptions{Level: slog.LevelInfo}))
	h := logRequests(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/slow", "/poll":
			time.Sleep(30 * time.Millisecond)
			w.Write([]byte(`{"messages":[]}`))
		case "/fail":
			w.WriteHeader(500)
			w.Write([]byte(`{"code":"internal","message":"database is locked"}`))
		default:
			w.Write([]byte("ok"))
		}
	}), logger, 20*time.Millisecond, nil)
	for _, p := range []string{"/fast", "/slow", "/poll?wait_seconds=30", "/fail"} {
		req := httptest.NewRequest("GET", p, nil)
		if p == "/slow" {
			req.Header.Set("X-Request-Id", "app-1") // the caller's: kept
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if id := rec.Header().Get("X-Request-Id"); id == "" || (p == "/slow" && id != "app-1") {
			t.Fatalf("%s: the request's id is answered back: %q", p, id)
		}
	}
	out := log.String()
	if !strings.Contains(out, "id=app-1") {
		t.Fatalf("a log line names the request's id: %s", out)
	}
	if !strings.Contains(out, `msg="slow request"`) || !strings.Contains(out, "path=/slow") || !strings.Contains(out, "bytes=15") {
		t.Fatalf("a slow request is logged with its size: %s", out)
	}
	if strings.Contains(out, "path=/poll") || strings.Contains(out, "path=/fast") {
		t.Fatalf("long polls and fast requests are not slow: %s", out)
	}
	if !strings.Contains(out, `msg="request failed"`) || !strings.Contains(out, "database is locked") || !strings.Contains(out, "status=500") {
		t.Fatalf("a failed request is logged with its error: %s", out)
	}
}

// The handler marks a decoded body wait without the middleware reading or
// consuming the body. Both logging and the request histogram honor it;
// an immediate pull is still measured, and server failures are still logged.
func TestBodyWaitExcludedFromSlowLogAndMetrics(t *testing.T) {
	for _, tc := range []struct {
		name   string
		wait   bool
		status int
	}{
		{"waiting", true, 200}, {"immediate", false, 200}, {"failed wait", true, 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			log := slog.New(slog.NewTextHandler(&out, nil))
			rec := perf.New()
			h := logRequests(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				perf.SetRoute(r.Context(), "POST /v1/inbox/pull")
				if tc.wait {
					perf.MarkWaiting(r.Context())
				}
				time.Sleep(2 * time.Millisecond)
				w.WriteHeader(tc.status)
			}), log, time.Millisecond, rec)
			// Workspace routing does not change whether the handler waited.
			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/w/demo/v1/inbox/pull", strings.NewReader(`{"wait_seconds":30}`)))
			stats := rec.Report(time.Minute, 0)
			if tc.wait && len(stats) != 0 {
				t.Fatalf("long poll in metrics: %v", stats)
			}
			if !tc.wait && (len(stats) != 1 || stats[0].Count != 1) {
				t.Fatalf("immediate pull not measured: %v", stats)
			}
			if strings.Contains(out.String(), `msg="slow request"`) == tc.wait {
				t.Fatalf("incorrect slow log: %s", out.String())
			}
			if (tc.status == 500) != strings.Contains(out.String(), `msg="request failed"`) {
				t.Fatalf("incorrect server error log: %s", out.String())
			}
		})
	}
}
