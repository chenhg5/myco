package hubserver

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestLogRedactsRecoveryCredential(t *testing.T) {
	var log bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&log, &slog.HandlerOptions{Level: slog.LevelDebug}))
	h := logRequests(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }), logger)
	req := httptest.NewRequest("GET", "/r/myco_resume_secret", nil)
	h.ServeHTTP(httptest.NewRecorder(), req)
	if strings.Contains(log.String(), "myco_resume_secret") || !strings.Contains(log.String(), "/r/<redacted>") {
		t.Fatalf("request log did not redact credential: %s", log.String())
	}
}
