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
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/config"
	"github.com/myco-ai/myco/internal/harness"
	"github.com/myco-ai/myco/internal/quota"
	"github.com/myco-ai/myco/internal/runner"
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

func TestRuntimeProviderReportsOnlyHost(t *testing.T) {
	var seen atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if strings.Contains(string(raw), "private-provider-key") || strings.Contains(string(raw), "/v1/private-path") {
			t.Error("provider details leaked to hub")
		}
		var rt api.Runtime
		_ = json.Unmarshal(raw, &rt)
		if rt.ProviderHost != "127.0.0.1:3425" || rt.ProviderSource != "env_file" {
			t.Errorf("runtime provider host/source missing")
		}
		seen.Store(true)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()
	c, err := client.New(srv.URL, "agent-token")
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "agent.env")
	os.WriteFile(file, []byte("OPENAI_BASE_URL=http://127.0.0.1:3425/v1/private-path\nOPENAI_API_KEY=private-provider-key\n"), 0600)
	rep := &runtimeReporter{c: c, log: slog.New(slog.NewTextHandler(io.Discard, nil)), base: &api.Runtime{CLI: "codex"}, choice: resolveProvider("codex", "", file)}
	rep.flush(context.Background())
	if !seen.Load() {
		t.Fatal("no runtime sent")
	}
}

func TestRuntimeReporterSendsQueueAndAdmission(t *testing.T) {
	reports := make(chan api.Runtime, 16)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var rt api.Runtime
		if err := json.NewDecoder(r.Body).Decode(&rt); err != nil {
			t.Error(err)
		}
		reports <- rt
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c, _ := client.New(srv.URL, "test")
	rep := &runtimeReporter{c: c, log: slog.New(slog.NewTextHandler(io.Discard, nil)), base: &api.Runtime{CLI: "codex", Model: "example"}}
	pool := runner.NewSlotPool(1)
	a := pool.Request("a", "a", "a")
	b := pool.Request("b", runner.SlotOwner("b", "same"), "queued")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go rep.watchSlots(ctx, pool, runner.SlotOwner("b", "same"))
	wait := func(queued bool) {
		t.Helper()
		deadline := time.After(3 * time.Second)
		for {
			select {
			case rt := <-reports:
				if rt.CLI != "codex" || rt.Model != "example" {
					t.Fatal("lost base runtime")
				}
				if rt.RunnerSlots != nil && (len(rt.RunnerSlots.Queue) > 0) == queued {
					return
				}
			case <-deadline:
				t.Fatal("runtime queue update not sent")
				return
			}
		}
	}
	wait(true)
	a.Done()
	wait(false)
	b.Done()
}

// A slow sample cannot install itself after the agent changed its provider.
func TestQuotaLateResultDoesNotCrossBinding(t *testing.T) {
	r := &runtimeReporter{}
	first := api.Quota{Provider: "codex", ProfileRef: "one", Status: "ok", At: time.Now(), Windows: []api.QuotaWindow{{Name: "weekly", Used: .82}}}
	r.quotaSource = quota.Source{Provider: "minimax", Ref: "two"}
	r.quota = &api.Quota{Provider: "minimax", ProfileRef: "two", Status: "no_credentials", Windows: []api.QuotaWindow{}}
	r.setQuota(first.ProfileRef, &first)
	if r.quota.ProfileRef != "two" || len(r.quota.Windows) != 0 {
		t.Fatal("late result crossed account binding")
	}
}

func TestOldTranscriptQuotaCannotBecomeNewLoginSample(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".codex")
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(dir, "auth.json"), []byte(`{"tokens":{"account_id":"new-login","access_token":"fixture-token"}}`), 0600); e != nil {
		t.Fatal(e)
	}
	store, e := quota.NewStore(filepath.Join(home, "quota"))
	if e != nil {
		t.Fatal(e)
	}
	input := func(providerChoice) quota.Input {
		return quota.Input{CLI: "codex", Home: home, Env: map[string]string{}}
	}
	src := store.Discover(input(providerChoice{}))
	r := &runtimeReporter{quotaStore: store, quotaInput: input, quotaSource: src, runQuotaRef: src.Ref, runQuotaStarted: time.Now(), runQuotaIdentity: src, quota: &api.Quota{ProfileRef: src.Ref, Windows: []api.QuotaWindow{}}}
	r.Quota(harness.Quota{Source: "codex", At: time.Now().Add(-time.Hour), Windows: []harness.QuotaWindow{{Name: "primary", Used: .99}}})
	if len(r.quota.Windows) != 0 {
		t.Fatal("old transcript attached to new login")
	}
}
