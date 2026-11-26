package provider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const testKey = "sk-test-secret-123"

func TestFetchModelsKeepsChatModelsAndFollowsPages(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/anthropic/v1/models" || r.Header.Get("Authorization") != "Bearer "+testKey || r.Header.Get("x-api-key") != testKey || r.Header.Get("anthropic-version") == "" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		if r.URL.Query().Get("after_id") == "" {
			w.Write([]byte(`{"data":[{"id":"deepseek-v4.1-flash","name":"DeepSeek V4.1 Flash","type":"chat"},{"id":"text-embedding-3-small","type":"embedding"},{"id":"gemini-decision-1","type":"decision"}],"has_more":true,"last_id":"gemini-decision-1"}`))
			return
		}
		w.Write([]byte(`{"data":[{"id":"kimi-k3","display_name":"Kimi K3","type":"model"},{"id":"deepseek-v4.1-flash"}],"has_more":false}`))
	}))
	defer srv.Close()
	ms, err := FetchModels(context.Background(), srv.Client(), Endpoint{Base: srv.URL + "/anthropic", Key: testKey, Anthropic: true})
	if err != nil {
		t.Fatal(err)
	}
	want := []Model{{ID: "deepseek-v4.1-flash", Name: "DeepSeek V4.1 Flash"}, {ID: "kimi-k3", Name: "Kimi K3"}}
	if !reflect.DeepEqual(ms, want) {
		t.Fatalf("models = %+v", ms)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls = %d", calls.Load())
	}
}

func TestFetchModelsAsksWithOneHeaderWhenTwoAreRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "" {
			http.Error(w, "one auth header only", http.StatusBadRequest)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+testKey {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{"object":"list","data":[{"id":"glm-4.6"}]}`))
	}))
	defer srv.Close()
	ms, err := FetchModels(context.Background(), srv.Client(), Endpoint{Base: srv.URL + "/v1", Key: testKey})
	if err != nil || len(ms) != 1 || ms[0].ID != "glm-4.6" {
		t.Fatalf("models = %+v, %v", ms, err)
	}
}

func TestTheKeyIsNotSentToAnotherHostOrPutInErrors(t *testing.T) {
	var leaked atomic.Bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Authorization")+r.Header.Get("x-api-key"), testKey) {
			leaked.Store(true)
		}
		w.Write([]byte(`{"data":[{"id":"stolen"}]}`))
	}))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/v1/models", http.StatusFound)
	}))
	defer srv.Close()
	ms, err := FetchModels(context.Background(), srv.Client(), Endpoint{Base: srv.URL + "/v1", Key: testKey})
	if err == nil || len(ms) != 0 || leaked.Load() {
		t.Fatalf("models = %+v, err = %v, leaked = %v", ms, err, leaked.Load())
	}
	if strings.Contains(err.Error(), testKey) || strings.Contains(err.Error(), "/v1") {
		t.Fatalf("error says too much: %v", err)
	}
}

func TestModelURLsTryTheVendorsOtherInterface(t *testing.T) {
	cases := []struct {
		e    Endpoint
		want []string
	}{
		{Endpoint{Base: "https://opencode.ai/inference/go", Anthropic: true}, []string{"https://opencode.ai/inference/go/v1/models"}},
		{Endpoint{Base: "https://opencode.ai/zen/go/", Anthropic: true}, []string{"https://opencode.ai/zen/go/v1/models", "https://opencode.ai/inference/go/v1/models"}},
		{Endpoint{Base: "https://api.deepseek.com/anthropic", Anthropic: true}, []string{"https://api.deepseek.com/anthropic/v1/models", "https://api.deepseek.com/v1/models"}},
		{Endpoint{Base: "https://open.bigmodel.cn/api/paas/v4"}, []string{"https://open.bigmodel.cn/api/paas/v4/models", "https://open.bigmodel.cn/api/anthropic/v1/models"}},
		{Endpoint{Base: "https://llm.example.com/openai"}, []string{"https://llm.example.com/openai/models", "https://llm.example.com/openai/v1/models"}},
	}
	for _, c := range cases {
		if got := modelURLs(c.e); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %v", c.e.Base, got)
		}
	}
}

type failing struct{}

func (failing) RoundTrip(*http.Request) (*http.Response, error) { return nil, errors.New("offline") }

func TestTheCacheKeepsTheLastListThenFallsBackToTheCatalogue(t *testing.T) {
	var calls atomic.Int32
	up := atomic.Bool{}
	up.Store(true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if !up.Load() {
			http.Error(w, "down", http.StatusBadGateway)
			return
		}
		w.Write([]byte(`{"data":[{"id":"m1"},{"id":"m2"}]}`))
	}))
	defer srv.Close()
	now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	c := &ModelCache{Dir: dir, Client: srv.Client(), Now: func() time.Time { return now }}
	e := Endpoint{Base: srv.URL + "/v1", Key: testKey}
	l, ok := c.Lookup(context.Background(), e, false)
	if !ok || l.From != "provider" || len(l.Models) != 2 || !l.At.Equal(now) {
		t.Fatalf("first = %+v %v", l, ok)
	}
	if _, ok := c.Lookup(context.Background(), e, false); !ok || calls.Load() != 1 {
		t.Fatalf("a fresh list is asked again: %d calls", calls.Load())
	}
	// The cache holds no key.
	files, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	for _, f := range files {
		b, _ := os.ReadFile(f)
		if strings.Contains(string(b), testKey) || strings.Contains(string(b), srv.URL) {
			t.Fatalf("cache file holds the key or endpoint: %s", b)
		}
		if st, _ := os.Stat(f); st.Mode().Perm()&0o077 != 0 && runtime.GOOS != "windows" {
			t.Fatalf("cache mode %v", st.Mode())
		}
	}
	// Stale and the vendor down: the last list, with its time.
	now = now.Add(time.Hour)
	up.Store(false)
	l, ok = c.Lookup(context.Background(), e, false)
	if !ok || len(l.Models) != 2 || !l.At.Equal(now.Add(-time.Hour)) {
		t.Fatalf("stale = %+v %v", l, ok)
	}
	n := calls.Load()
	if _, _ = c.Lookup(context.Background(), e, false); calls.Load() != n {
		t.Fatal("a failed vendor is asked again at once")
	}
	if _, _ = c.Lookup(context.Background(), e, true); calls.Load() == n {
		t.Fatal("asking again by hand does not ask")
	}
	// Never listed: the catalogue for a known vendor, nothing for others.
	off := &ModelCache{Dir: t.TempDir(), Client: &http.Client{Transport: failing{}}}
	l, ok = off.Lookup(context.Background(), Endpoint{Base: "https://opencode.ai/inference/go", Key: testKey, Anthropic: true}, false)
	if !ok || l.From != "catalog" || l.Models[0].ID != "deepseek-v4.1-flash" || !l.At.IsZero() {
		t.Fatalf("catalogue = %+v %v", l, ok)
	}
	if _, ok := off.Lookup(context.Background(), Endpoint{Base: "https://llm.example.com/v1"}, false); ok {
		t.Fatal("an unknown vendor offers a list it never had")
	}
}

func TestClaudeEndpoint(t *testing.T) {
	home := t.TempDir()
	if _, ok := ClaudeEndpoint(home, map[string]string{}, nil); ok {
		t.Fatal("Anthropic's own login is a third party")
	}
	if _, ok := ClaudeEndpoint(home, map[string]string{"ANTHROPIC_BASE_URL": "https://api.anthropic.com", "ANTHROPIC_API_KEY": "k"}, nil); ok {
		t.Fatal("Anthropic's API is a third party")
	}
	os.MkdirAll(filepath.Join(home, ".claude"), 0o700)
	os.WriteFile(filepath.Join(home, ".claude", "settings.json"), []byte(`{"env":{"ANTHROPIC_BASE_URL":"https://opencode.ai/inference/go","ANTHROPIC_AUTH_TOKEN":"from-settings"}}`), 0o600)
	e, ok := ClaudeEndpoint(home, map[string]string{"ANTHROPIC_BASE_URL": "https://api.deepseek.com/anthropic"}, nil)
	if !ok || e.Base != "https://opencode.ai/inference/go" || e.Key != "from-settings" || !e.Anthropic {
		t.Fatalf("settings.json beats the environment: %+v", e)
	}
	e, ok = ClaudeEndpoint(home, nil, map[string]string{"ANTHROPIC_BASE_URL": "https://api.moonshot.cn/anthropic", "ANTHROPIC_AUTH_TOKEN": "pinned"})
	if !ok || e.Base != "https://api.moonshot.cn/anthropic" || e.Key != "pinned" {
		t.Fatalf("a pinned provider beats settings.json: %+v", e)
	}
	if _, ok := ClaudeEndpoint(home, map[string]string{"CLAUDE_CODE_USE_BEDROCK": "1"}, nil); ok {
		t.Fatal("Bedrock is listed")
	}
}

func TestCodexEndpoint(t *testing.T) {
	home := t.TempDir()
	if _, ok := CodexEndpoint(home, map[string]string{}, nil); ok {
		t.Fatal("ChatGPT login is a third party")
	}
	args, env := CustomCodex("https://api.deepseek.com/v1", "pinned-key", "deepseek-chat", "chat")
	e, ok := CodexEndpoint(home, env, args)
	if !ok || e.Base != "https://api.deepseek.com/v1" || e.Key != "pinned-key" || e.Anthropic {
		t.Fatalf("pinned = %+v %v", e, ok)
	}
	os.MkdirAll(filepath.Join(home, ".codex"), 0o700)
	os.WriteFile(filepath.Join(home, ".codex", "config.toml"), []byte("model_provider = \"kimi\"\nmodel = \"kimi-k2\"\n[model_providers.kimi]\nbase_url = \"https://api.moonshot.cn/v1\"\nenv_key = \"MOONSHOT_API_KEY\"\n"), 0o600)
	e, ok = CodexEndpoint(home, map[string]string{"MOONSHOT_API_KEY": "kimi-key"}, nil)
	if !ok || e.Base != "https://api.moonshot.cn/v1" || e.Key != "kimi-key" {
		t.Fatalf("config.toml = %+v %v", e, ok)
	}
	os.WriteFile(filepath.Join(home, ".codex", "config.toml"), []byte("model_provider = \"ollama\"\n"), 0o600)
	if _, ok := CodexEndpoint(home, map[string]string{}, nil); ok {
		t.Fatal("a built-in provider is listed")
	}
}

func TestSmallModel(t *testing.T) {
	go_ := Endpoint{Base: "https://opencode.ai/inference/go", Anthropic: true}
	if got := SmallModel(go_, []Model{{ID: "kimi-k3"}, {ID: "deepseek-v4.1-flash"}}, "kimi-k3"); got != "deepseek-v4.1-flash" {
		t.Fatalf("listed small = %s", got)
	}
	if got := SmallModel(go_, []Model{{ID: "kimi-k3"}}, "kimi-k3"); got != "kimi-k3" {
		t.Fatalf("unlisted small = %s", got)
	}
	if got := SmallModel(Endpoint{Base: "https://api.minimax.io/anthropic"}, nil, "MiniMax-M2"); got != "MiniMax-M2" {
		t.Fatalf("no small = %s", got)
	}
	if got := SmallModel(Endpoint{Base: "https://llm.example.com"}, nil, "x"); got != "x" {
		t.Fatalf("unknown vendor = %s", got)
	}
}

func TestEveryVendorIsFoundAtItsBases(t *testing.T) {
	for _, v := range Vendors {
		for _, h := range v.Hosts {
			for _, p := range []string{v.Anthropic, v.OpenAI} {
				if got, ok := VendorOf("https://" + h + p); !ok || got.ID != v.ID {
					t.Errorf("%s%s: %v %v", h, p, got.ID, ok)
				}
			}
		}
		if len(v.Models) == 0 {
			t.Errorf("%s offers nothing", v.ID)
		}
	}
	if _, ok := VendorOf("https://opencode.ai/zen/v1"); ok {
		t.Error("OpenCode Zen is taken for Go")
	}
}
