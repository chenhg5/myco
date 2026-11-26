package cli

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/harness"
	"github.com/myco-ai/myco/internal/provider"
)

const vendorKey = "sk-vendor-secret-42"

// A Claude Code agent on another vendor lists that vendor's models with
// its key, reports them without the key, and lists again when asked.
func TestAnAgentOnAnotherVendorReportsItsModels(t *testing.T) {
	var asked atomic.Int32
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/inference/go/v1/models" || r.Header.Get("Authorization") != "Bearer "+vendorKey {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		asked.Add(1)
		w.Write([]byte(`{"data":[{"id":"deepseek-v4.1-flash","name":"DeepSeek V4.1 Flash","type":"chat"},{"id":"kimi-k3","type":"chat"},{"id":"qwen3-embedding-8b","type":"embedding"}]}`))
	}))
	defer vendor.Close()

	var mu sync.Mutex
	var reports []api.Runtime
	refresh := "1"
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/me":
			mu.Lock()
			v := refresh
			mu.Unlock()
			json.NewEncoder(w).Encode(api.Agent{Name: "a", RunSettings: &api.RunSettings{ModelsRefresh: v}})
		default:
			raw, _ := io.ReadAll(r.Body)
			if strings.Contains(string(raw), vendorKey) || strings.Contains(string(raw), vendor.URL) {
				t.Error("the key or endpoint reached the hub")
			}
			var rt api.Runtime
			if json.Unmarshal(raw, &rt) == nil {
				mu.Lock()
				reports = append(reports, rt)
				mu.Unlock()
			}
			w.Write([]byte("{}"))
		}
	}))
	defer hub.Close()
	c, err := client.New(hub.URL, "agent-token")
	if err != nil {
		t.Fatal(err)
	}
	old := modelsEvery
	modelsEvery = 20 * time.Millisecond
	defer func() { modelsEvery = old }()

	home := t.TempDir()
	env := map[string]string{"ANTHROPIC_BASE_URL": vendor.URL + "/inference/go", "ANTHROPIC_AUTH_TOKEN": vendorKey}
	ch := providerChoice{settings: &harness.ProviderSettings{Env: env}, name: "opencode.ai", source: "env_file", model: "deepseek-v4.1-flash"}
	rep := &runtimeReporter{c: c, log: slog.New(slog.NewTextHandler(io.Discard, nil)), base: &api.Runtime{CLI: "claude"}, choice: ch}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rep.startModels(ctx, t.TempDir(), func(ch providerChoice) (providerTarget, bool) {
		return providerTargetOf("claude", home, map[string]string{}, ch)
	}, func() providerChoice { return ch })

	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, rt := range reports {
			if rt.ModelsFrom == "provider" && rt.ModelsAt != nil && len(rt.Models) == 2 && rt.Models[0].ID == "deepseek-v4.1-flash" && rt.Models[0].Name == "DeepSeek V4.1 Flash" {
				return true
			}
		}
		return false
	})
	// Cached: not asked again on every look.
	time.Sleep(100 * time.Millisecond)
	if n := asked.Load(); n != 1 {
		t.Fatalf("asked %d times", n)
	}
	mu.Lock()
	refresh = "2"
	mu.Unlock()
	waitFor(t, func() bool { return asked.Load() == 2 })

	// The run on kimi-k3 gets its tiers (a vendor not in the catalogue
	// has no small model: background work runs on the same).
	got := rep.ModelEnv("kimi-k3")
	want := map[string]string{
		"ANTHROPIC_DEFAULT_OPUS_MODEL": "kimi-k3", "ANTHROPIC_DEFAULT_SONNET_MODEL": "kimi-k3",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL": "kimi-k3", "ANTHROPIC_SMALL_FAST_MODEL": "kimi-k3",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("model env = %v", got)
	}
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestModelEnvKeepsWhatThePersonSet(t *testing.T) {
	ep := provider.Endpoint{Base: "https://opencode.ai/inference/go", Anthropic: true}
	// OpenCode Go's small model, listed for this account.
	onGo := &runtimeReporter{hasTarget: true, target: providerTarget{endpoint: ep, claude: true}, provModels: &provider.ModelList{Models: []provider.Model{{ID: "kimi-k3"}, {ID: "deepseek-v4.1-flash"}}}}
	if got := onGo.ModelEnv("kimi-k3"); got["ANTHROPIC_DEFAULT_SONNET_MODEL"] != "kimi-k3" || got["ANTHROPIC_DEFAULT_HAIKU_MODEL"] != "deepseek-v4.1-flash" || got["ANTHROPIC_SMALL_FAST_MODEL"] != "deepseek-v4.1-flash" {
		t.Fatalf("go env = %v", got)
	}
	list := &provider.ModelList{Models: []provider.Model{{ID: "kimi-k3"}, {ID: "glm-5.3"}}, From: "provider"}
	rep := &runtimeReporter{hasTarget: true, target: providerTarget{endpoint: ep, claude: true, set: map[string]bool{"ANTHROPIC_DEFAULT_SONNET_MODEL": true}}, provModels: list, choice: providerChoice{model: "glm-5.3"}}
	// The small model is not in this account's list: the same model.
	got := rep.ModelEnv("")
	want := map[string]string{"ANTHROPIC_DEFAULT_OPUS_MODEL": "glm-5.3", "ANTHROPIC_DEFAULT_HAIKU_MODEL": "glm-5.3", "ANTHROPIC_SMALL_FAST_MODEL": "glm-5.3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("model env = %v", got)
	}
	// A Claude alias is left to the tiers; nothing is set to it.
	if got := rep.ModelEnv("sonnet"); got["ANTHROPIC_DEFAULT_OPUS_MODEL"] != "" || got["ANTHROPIC_DEFAULT_HAIKU_MODEL"] != "" {
		t.Fatalf("alias env = %v", got)
	}
	// Codex and Anthropic's own: nothing.
	if got := (&runtimeReporter{hasTarget: true, target: providerTarget{endpoint: ep}}).ModelEnv("x"); got != nil {
		t.Fatalf("codex env = %v", got)
	}
	if got := (&runtimeReporter{}).ModelEnv("x"); got != nil {
		t.Fatalf("anthropic env = %v", got)
	}
}

func TestProviderTargetOfACodexCustomProvider(t *testing.T) {
	args, env := provider.CustomCodex("https://api.deepseek.com/v1", "k", "", "chat")
	tg, ok := providerTargetOf("codex", t.TempDir(), map[string]string{}, providerChoice{settings: &harness.ProviderSettings{Env: env, ConfigArgs: args}})
	if !ok || tg.endpoint.Base != "https://api.deepseek.com/v1" || tg.claude {
		t.Fatalf("target = %+v %v", tg.endpoint.Base, ok)
	}
	if _, ok := providerTargetOf("claude", t.TempDir(), map[string]string{}, providerChoice{}); ok {
		t.Fatal("a Claude login has a vendor target")
	}
}

// The Codex cache is reported as the CLI's own list; a provider list wins.
func TestTheRuntimeSaysWhereItsModelsCameFrom(t *testing.T) {
	var last atomic.Value
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var rt api.Runtime
		_ = json.NewDecoder(r.Body).Decode(&rt)
		last.Store(rt)
		w.Write([]byte("{}"))
	}))
	defer hub.Close()
	c, _ := client.New(hub.URL, "agent-token")
	rep := &runtimeReporter{c: c, log: slog.New(slog.NewTextHandler(io.Discard, nil)), base: &api.Runtime{CLI: "codex"},
		models: func() []api.RuntimeModel { return []api.RuntimeModel{{ID: "gpt-6"}} }}
	rep.flush(context.Background())
	if rt := last.Load().(api.Runtime); rt.ModelsFrom != "cli" || rt.ModelsAt != nil {
		t.Fatalf("cli list = %+v", rt)
	}
	at := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	rep.mu.Lock()
	rep.provModels = &provider.ModelList{Models: []provider.Model{{ID: "deepseek-chat"}}, From: "provider", At: at}
	rep.mu.Unlock()
	rep.flush(context.Background())
	if rt := last.Load().(api.Runtime); rt.ModelsFrom != "provider" || len(rt.Models) != 1 || rt.Models[0].ID != "deepseek-chat" || rt.ModelsAt == nil || !rt.ModelsAt.Equal(at) {
		t.Fatalf("provider list = %+v", rt)
	}
}
