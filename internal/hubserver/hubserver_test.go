package hubserver

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", p, nil))
	}
	out := log.String()
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
