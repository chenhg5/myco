package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/testutil"
)

func TestUnknownCommandUpgradeHint(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	oldVersion := Version
	Version = "v0.18.8"
	t.Cleanup(func() { Version = oldVersion })

	for _, tc := range []struct {
		name       string
		version    string
		args       []string
		wantHint   bool
		wantCalled bool
	}{
		{"new hub, top level", "v0.18.26", []string{"future", "pull"}, true, true},
		{"new hub, subcommand", "v0.18.26", []string{"agent", "future"}, true, true},
		{"old hub", "v0.18.7", []string{"future", "pull"}, false, true},
		{"same hub", "v0.18.8", []string{"future", "pull"}, false, true},
		{"legacy hub without version", "", []string{"future", "pull"}, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var called atomic.Bool
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called.Store(true)
				if r.URL.Path != "/v1/health" {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				json.NewEncoder(w).Encode(map[string]any{"ok": true, "version": "v1", "server_version": tc.version})
			}))
			defer srv.Close()
			h := &harnessT{env: map[string]string{"MYCO_HUB": srv.URL, "MYCO_CONFIG": filepath.Join(t.TempDir(), "config.yaml")}}
			code, _, stderr := h.run(tc.args...)
			var got Error
			if err := json.Unmarshal([]byte(stderr), &got); err != nil {
				t.Fatal(err)
			}
			if code != ExitUsage || got.Exit != ExitUsage || got.Code != "invalid_usage" || got.Retryable {
				t.Fatalf("error contract changed: code=%d error=%+v", code, got)
			}
			if called.Load() != tc.wantCalled || strings.Contains(got.Suggestion, "myco upgrade") != tc.wantHint {
				t.Fatalf("called=%v, suggestion=%q", called.Load(), got.Suggestion)
			}
			if tc.wantHint && (!strings.Contains(got.Suggestion, "v0.18.8") || !strings.Contains(got.Suggestion, tc.version)) {
				t.Fatalf("versions missing from suggestion: %q", got.Suggestion)
			}
		})
	}
}

func TestUnknownCommandHubUnavailable(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	oldVersion := Version
	Version = "v0.18.8"
	t.Cleanup(func() { Version = oldVersion })

	for _, tc := range []struct {
		name string
		hub  string
	}{
		{"unconfigured", ""},
		{"unreachable", "http://127.0.0.1:1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &harnessT{env: map[string]string{"MYCO_HUB": tc.hub, "MYCO_CONFIG": filepath.Join(t.TempDir(), "config.yaml")}}
			code, _, stderr := h.run("future", "pull")
			var got Error
			if err := json.Unmarshal([]byte(stderr), &got); err != nil {
				t.Fatal(err)
			}
			if code != ExitUsage || got.Code != "invalid_usage" || got.Retryable || strings.Contains(got.Suggestion, "myco upgrade") {
				t.Fatalf("unexpected error: code=%d error=%+v", code, got)
			}
		})
	}
}

func TestUnknownCommandHealthTimeout(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()
	h := &harnessT{env: map[string]string{"MYCO_HUB": srv.URL, "MYCO_CONFIG": filepath.Join(t.TempDir(), "config.yaml")}}
	start := time.Now()
	code, _, stderr := h.run("future", "pull")
	if code != ExitUsage || strings.Contains(stderr, "myco upgrade") || time.Since(start) > 3*time.Second {
		t.Fatalf("timeout changed usage error or exceeded bound: code=%d elapsed=%s error=%s", code, time.Since(start), stderr)
	}
}
