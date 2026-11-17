package cli

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/config"
	"github.com/chenhg5/myco/internal/harness"
)

func TestCLIConfigModel(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "claude"), 0o700)
	os.MkdirAll(filepath.Join(dir, "codex"), 0o700)
	os.WriteFile(filepath.Join(dir, "claude", "settings.json"), []byte(`{"model":"claude-opus-5-5","theme":"dark"}`), 0o600)
	os.WriteFile(filepath.Join(dir, "codex", "config.toml"), []byte("model = \"gpt-6-sol\"\napproval = \"never\"\n\n[profiles.fast]\nmodel = \"gpt-6-mini\"\n"), 0o600)
	env := map[string]string{"CLAUDE_CONFIG_DIR": filepath.Join(dir, "claude"), "CODEX_HOME": filepath.Join(dir, "codex")}
	get := func(k string) string { return env[k] }
	if m := cliConfigModel("claude", get); m != "claude-opus-5-5" {
		t.Errorf("claude: %q", m)
	}
	if m := cliConfigModel("codex", get); m != "gpt-6-sol" {
		t.Errorf("codex: %q (a profile's model must not win)", m)
	}
	os.WriteFile(filepath.Join(dir, "codex", "config.toml"), []byte("[profiles.fast]\nmodel = \"gpt-6-mini\"\n"), 0o600)
	if m := cliConfigModel("codex", get); m != "" {
		t.Errorf("codex without a top-level model: %q", m)
	}
	if m := cliConfigModel("exec", get); m != "" {
		t.Errorf("exec: %q", m)
	}
}

// The app tells from an agent's reported max_access whether its machine lets
// agents open a terminal; a change to machine.yaml is reported again without
// restarting the runner.
func TestRunnerReportsAShellAccessChange(t *testing.T) {
	var mu sync.Mutex
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/v1/me/runtime" {
			var rt api.Runtime
			_ = json.NewDecoder(r.Body).Decode(&rt)
			mu.Lock()
			got = append(got, rt.MaxAccess)
			mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()
	c, err := client.New(srv.URL, "tok")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(config.MachinePath(cfg), []byte("hub: http://x\nmachine: m1\nenroll_token: t\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	e := &env{defaultPath: cfg}
	rep := &runtimeReporter{c: c, log: slog.New(slog.NewTextHandler(io.Discard, nil)), base: &api.Runtime{CLI: "machine", MaxAccess: string(e.machineMaxAccess())}}
	rep.flush(context.Background())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var reads atomic.Int32
	go rep.watchMaxAccess(ctx, 10*time.Millisecond, e.machineMaxAccess(), func() harness.Access { reads.Add(1); return e.machineMaxAccess() })
	if _, err := config.SetShellAccess(cfg, true); err != nil {
		t.Fatal(err)
	}
	last := func() string {
		mu.Lock()
		defer mu.Unlock()
		if len(got) == 0 {
			return ""
		}
		return got[len(got)-1]
	}
	deadline := time.Now().Add(3 * time.Second)
	for last() != "full" {
		if time.Now().After(deadline) {
			t.Fatalf("reported %v", got)
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	first := got[0]
	mu.Unlock()
	if first != "workspace" {
		t.Fatalf("first report %q", first)
	}
	if !config.ShellAllowed(cfg) || reads.Load() == 0 {
		t.Fatal("shell access not read")
	}
}
