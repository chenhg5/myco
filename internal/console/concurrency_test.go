package console_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/config"
	"github.com/myco-ai/myco/internal/console"
)

// This computer's limit is machine.yaml's runner.max_concurrent, written
// by the App and read by the runner (runner-slots.json says what it does).
func TestLocalConcurrency(t *testing.T) {
	r := newRig(t)
	home := t.TempDir()
	l := &console.Local{Home: home, Device: "test-mac", LookPath: func(string) (string, error) { return "", errors.New("not found") }}
	b := &console.Backend{Store: console.NewStore(r.store), Secrets: r.secrets, Local: l, Shell: "mac", Version: "v1"}
	r.app.Config.Handler = b.Handler()

	if code, m, _ := r.do("GET", "/app/local/concurrency", nil); code != 200 || m["joined"] != false || m["runner"] != nil || m["default"] != float64(config.DefaultMaxConcurrent) {
		t.Fatalf("before joining: %d %v", code, m)
	}
	if code, m, _ := r.do("POST", "/app/local/concurrency", map[string]int{"max_concurrent": 12}); code != 409 || m["error"] != "not_configured" {
		t.Fatalf("set before joining: %d %v", code, m)
	}
	cfg := filepath.Join(home, ".myco", "config.yaml")
	os.MkdirAll(filepath.Dir(cfg), 0o700)
	os.WriteFile(config.MachinePath(cfg), []byte("hub: https://hub.example.com\nmachine: test-mac\nenroll_token: secret\nfuture_field: keep me\n"), 0o600)

	for _, n := range []int{0, 65} {
		if code, m, _ := r.do("POST", "/app/local/concurrency", map[string]int{"max_concurrent": n}); code != 400 || m["error"] != "invalid_argument" {
			t.Fatalf("%d: %d %v", n, code, m)
		}
	}
	code, m, _ := r.do("POST", "/app/local/concurrency", map[string]int{"max_concurrent": 12})
	if code != 200 || m["setting"] != float64(12) || m["joined"] != true || m["runner"] != nil {
		t.Fatalf("set: %d %v", code, m)
	}
	got, _ := os.ReadFile(config.MachinePath(cfg))
	for _, want := range []string{"max_concurrent: 12", "future_field: keep me", "enroll_token: secret"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("machine.yaml lost %q:\n%s", want, got)
		}
	}
	// The runner says it took it.
	config.SaveRunnerSlots(cfg, &config.RunnerSlotsState{PID: 1, Limit: 12, Source: config.ConcurrencyFromSetting, Flag: 3, InUse: 4, Waiting: 1, At: time.Now()})
	_, m, _ = r.do("GET", "/app/local/concurrency", nil)
	rn, _ := m["runner"].(map[string]any)
	if rn == nil || rn["limit"] != float64(12) || rn["in_use"] != float64(4) || rn["waiting"] != float64(1) || rn["source"] != "setting" {
		t.Fatalf("with a runner: %v", m)
	}
	// One that stopped long ago says nothing.
	config.SaveRunnerSlots(cfg, &config.RunnerSlotsState{PID: 1, Limit: 12, At: time.Now().Add(-time.Hour)})
	if _, m, _ = r.do("GET", "/app/local/concurrency", nil); m["runner"] != nil {
		t.Fatalf("stale runner: %v", m)
	}
}
