package quota

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
)

func TestPlanDiscovery(t *testing.T) {
	cases := []struct{ base, provider, endpoint string }{
		{"https://open.bigmodel.cn/api/anthropic", "glm", "https://open.bigmodel.cn/api/monitor/usage/quota/limit"},
		{"https://api.z.ai/api/anthropic/v1/", "glm", "https://api.z.ai/api/monitor/usage/quota/limit"},
		{"https://open.bigmodel.cn/api/coding/paas/v4", "glm", "https://open.bigmodel.cn/api/monitor/usage/quota/limit"},
		{"https://api.z.ai/api/coding/paas/v4", "glm", "https://api.z.ai/api/monitor/usage/quota/limit"},
		{"https://api.kimi.com/coding/v1", "kimi", "https://api.kimi.com/coding/v1/usages"},
		{"https://api.kimi.com/coding/", "kimi", "https://api.kimi.com/coding/v1/usages"},
		{"https://api.kimi.com/coding", "kimi", "https://api.kimi.com/coding/v1/usages"},
		{"https://openrouter.ai/api/v1/", "openrouter", "https://openrouter.ai/api/v1/key"},
	}
	for _, tc := range cases {
		t.Run(tc.base, func(t *testing.T) {
			in := Input{CLI: "claude", Home: t.TempDir(), Env: map[string]string{"ANTHROPIC_BASE_URL": tc.base, "ANTHROPIC_AUTH_TOKEN": "plan-test-key"}}
			s := Discover(in, []byte("test-salt"))
			if s.Provider != tc.provider || s.endpoint != tc.endpoint || s.Status != "ok" || s.key != "plan-test-key" {
				t.Fatalf("wrong discovery: %s", s)
			}
			delete(in.Env, "ANTHROPIC_AUTH_TOKEN")
			s = Discover(in, []byte("test-salt"))
			if s.Status != "no_credentials" || s.key != "" {
				t.Fatalf("missing credentials: %s", s)
			}
		})
	}
	for _, base := range []string{
		"https://open.bigmodel.cn/api/paas/v4", "https://api.z.ai/api/v1", "https://api.z.ai/api/anthropic-evil",
		"https://api.kimi.com/v1", "https://api.kimi.com/coding-other/v1", "https://openrouter.ai/v1",
		"https://gateway.example/api/anthropic", "https://api.z.ai.gateway.example/api/anthropic",
		"http://api.z.ai/api/anthropic", "https://api.z.ai:443/api/anthropic", "https://key@api.z.ai/api/anthropic",
		"https://api.z.ai/api/anthropic?redirect=1", "https://openrouter.ai/api%2fv1",
	} {
		t.Run(base, func(t *testing.T) {
			s := Discover(Input{CLI: "claude", Home: t.TempDir(), Env: map[string]string{"ANTHROPIC_BASE_URL": base, "ANTHROPIC_AUTH_TOKEN": "do-not-forward"}}, []byte("salt"))
			if s.Status != "unsupported" || s.key != "" || s.endpoint != "" {
				t.Fatal("unrecognized origin/path retained credentials")
			}
		})
	}
}

func TestPlanParsers(t *testing.T) {
	tests := []struct {
		name, body string
		parse      func([]byte, *api.Quota) error
		count      int
		used       float64
	}{
		{"glm", `{"code":200,"success":true,"data":{"limits":[{"type":"TOKENS_LIMIT","unit":3,"number":5,"percentage":25,"nextResetTime":1795900000000},{"type":"TOKENS_LIMIT","unit":6,"number":1,"percentage":50},{"type":"TIME_LIMIT","unit":5,"number":1,"percentage":0}]}}`, parseGLM, 3, .25},
		{"kimi", `{"usage":{"limit":"100","used":"29","remaining":"71","resetTime":"2026-12-01T12:00:00Z"},"limits":[{"window":{"duration":300,"timeUnit":"TIME_UNIT_MINUTE"},"detail":{"limit":"100","remaining":"100"}}]}`, parseKimi, 2, .29},
		{"router", `{"data":{"limit":100,"limit_remaining":74.5,"effective_budget":{"limit_usd":50,"remaining_usd":17.5}}}`, parseOpenRouter, 1, .65},
		{"router-unlimited", `{"data":{"limit":null,"limit_remaining":null}}`, parseOpenRouter, 1, 0},
		{"router-zero-limit", `{"data":{"limit":0,"limit_remaining":0}}`, parseOpenRouter, 1, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var q api.Quota
			if e := tc.parse([]byte(tc.body), &q); e != nil {
				t.Fatal(e)
			}
			if len(q.Windows) != tc.count || q.Windows[0].Used != tc.used {
				t.Fatalf("wrong windows: %+v", q.Windows)
			}
			switch tc.name {
			case "glm":
				if q.Windows[0].ResetsAt.UnixMilli() != 1795900000000 || q.Windows[1].DurationSec != 604800 || q.Windows[2].Name != "monthly_mcp" || q.Windows[2].ModelScope != "" {
					t.Fatal("wrong GLM period/reset/scope")
				}
			case "kimi":
				if q.Windows[1].DurationSec != 18000 || q.Windows[1].Used != 0 {
					t.Fatal("wrong Kimi window")
				}
			case "router":
				if *q.Windows[0].Balance.Remaining != 17.5 || *q.Windows[0].Balance.Limit != 50 || q.Windows[0].ResetsAt != nil {
					t.Fatal("effective balance not preferred")
				}
			case "router-unlimited":
				if !q.Windows[0].Balance.Unlimited {
					t.Fatal("explicit unlimited lost")
				}
			}
		})
	}
}
func TestPlanMalformedResponses(t *testing.T) {
	tests := []struct {
		parse  func([]byte, *api.Quota) error
		bodies []string
	}{
		{parseGLM, []string{`{}`, `{"code":401,"data":{"limits":[]}}`, `{"data":{"limits":[{"type":"TOKENS_LIMIT","unit":6,"number":7,"percentage":0}]}}`, `{"data":{"limits":[{"type":"TOKENS_LIMIT","unit":3,"number":5}]}}`, `{"data":{"limits":[{"type":"TIME_LIMIT","unit":5,"number":1,"percentage":101}]}}`, `{"data":{"limits":[{"type":"CREDIT_LIMIT","unit":3,"number":5,"percentage":0}]}}`, `{"data":{"limits":[{"type":"TOKENS_LIMIT","unit":3,"number":5,"percentage":0,"nextResetTime":"bad"}]}}`}},
		{parseKimi, []string{`{}`, `{"usage":{"limit":100}}`, `{"usage":{"limit":0,"remaining":0}}`, `{"usage":{"limit":100,"used":20,"remaining":90}}`, `{"usage":{"limit":100,"remaining":101}}`, `{"data":{"usages":{"limit5h":{"usedRatio":0}}}}`, `{"usage":{"limit":100,"remaining":20},"limits":[{"window":{"duration":300,"timeUnit":"UNKNOWN"},"detail":{"limit":100,"used":0}}]}`}},
		{parseOpenRouter, []string{`{}`, `{"data":{}}`, `{"data":{"limit":10}}`, `{"data":{"limit":10,"limit_remaining":11}}`, `{"data":{"limit":"NaN","limit_remaining":0}}`, `{"data":{"limit":100,"limit_remaining":74,"effective_budget":{}}}`}},
	}
	for i, tc := range tests {
		for j, b := range tc.bodies {
			t.Run(fmt.Sprintf("%d/%d", i, j), func(t *testing.T) {
				if tc.parse([]byte(b), &api.Quota{}) == nil {
					t.Fatal("malformed quota accepted")
				}
			})
		}
	}
}

type quotaRoundTrip func(*http.Request) (*http.Response, error)

func (f quotaRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPlanRequestsStayOnOwnOrigin(t *testing.T) {
	for _, tc := range []struct{ provider, endpoint, auth string }{
		{"glm", "https://open.bigmodel.cn/api/monitor/usage/quota/limit", "test-key"},
		{"glm", "https://api.z.ai/api/monitor/usage/quota/limit", "test-key"},
		{"kimi", "https://api.kimi.com/coding/v1/usages", "Bearer test-key"},
		{"openrouter", "https://openrouter.ai/api/v1/key", "Bearer test-key"},
	} {
		t.Run(tc.endpoint, func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: quotaRoundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.String() != tc.endpoint || r.Header.Get("Authorization") != tc.auth {
					t.Error("wrong origin/auth")
				}
				return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader("private response must not appear in error")), Header: make(http.Header)}, nil
			})}
			q := api.Quota{Status: "ok"}
			s := Source{Provider: tc.provider, endpoint: tc.endpoint, key: "test-key"}
			e := apiQuotaClient(context.Background(), s, &q, client)
			if e == nil || q.Status != "auth_required" || strings.Contains(e.Error(), "private") {
				t.Fatal("incorrect auth failure")
			}
			for _, endpoint := range []string{"https://gateway.example/quota", "http://api.kimi.com/coding/v1/usages", "https://openrouter.ai/api/v1/key"} {
				if endpoint == tc.endpoint {
					continue
				}
				s.endpoint = endpoint
				if apiQuotaClient(context.Background(), s, &q, client) == nil {
					t.Fatal("unexpected origin accepted")
				}
			}
			if calls != 1 {
				t.Fatal("key sent to invalid origin")
			}
		})
	}
}
func TestPlanHTTPFailuresAndRedirects(t *testing.T) {
	for _, status := range []int{401, 403, 429, 500, 302} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			c := &http.Client{Transport: quotaRoundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: status, Header: http.Header{"Location": []string{"https://gateway.example/key"}, "Retry-After": []string{"600"}}, Body: io.NopCloser(strings.NewReader("{}"))}, nil
			}), CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
			q := api.Quota{Status: "ok", CheckedAt: time.Now()}
			e := apiQuotaClient(context.Background(), Source{Provider: "kimi", endpoint: "https://api.kimi.com/coding/v1/usages", key: "test-key"}, &q, c)
			if e == nil || calls != 1 {
				t.Fatal("unexpected success/redirect")
			}
			want := "transient_error"
			if status == 401 || status == 403 {
				want = "auth_required"
			}
			if status == 429 {
				want = "throttled"
				r, ok := e.(rateLimited)
				if !ok || r.retry.Before(q.CheckedAt.Add(600*time.Second)) {
					t.Fatal("Retry-After ignored")
				}
			}
			if q.Status != want {
				t.Fatalf("got %s want %s", q.Status, want)
			}
		})
	}
}

func TestPlanCachePreservesBalanceAfterFailure(t *testing.T) {
	// Existing shared-cache merge must carry the new balance object unchanged.
	store, e := NewStore(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	src := Source{Provider: "openrouter", Ref: ref([]byte("salt"), "router"), Kind: "official_api", Status: "ok", endpoint: "https://invalid.example/key"}
	n, left := 100.0, 12.0
	old := api.Quota{Source: "openrouter", Provider: "openrouter", ProfileRef: src.Ref, Status: "ok", At: time.Now().Add(-time.Minute), CheckedAt: time.Now().Add(-time.Minute), Windows: []api.QuotaWindow{{Name: "balance", Used: .88, Balance: &api.QuotaBalance{Currency: "USD", Limit: &n, Remaining: &left}}}}
	if e = store.write(store.path(src), cached{Quota: old}); e != nil {
		t.Fatal(e)
	}
	got := store.Get(context.Background(), src)
	if got.Status == "ok" || !got.At.Equal(old.At) || len(got.Windows) != 1 || got.Windows[0].Balance == nil || *got.Windows[0].Balance.Remaining != 12 {
		t.Fatal("failed collection discarded previous balance")
	}
	data, _ := json.Marshal(got)
	if strings.Contains(string(data), "key") {
		t.Fatal("credential leaked")
	}
}

// Keep the real HTTP redirect semantics exercised separately from mock transport.
func TestPlanRedirectDoesNotForwardAuthorization(t *testing.T) {
	hits := 0
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits++ }))
	defer target.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer origin.Close()
	q := api.Quota{Status: "ok"}
	_ = apiQuota(context.Background(), Source{Provider: "opencode_go", endpoint: origin.URL, key: "never-forward"}, &q)
	if hits != 0 {
		t.Fatal("credential redirect followed")
	}
}

func TestPlanCollectionRejectsMalformedWithoutPartialWindows(t *testing.T) {
	previous := http.DefaultTransport
	defer func() { http.DefaultTransport = previous }()
	cases := []struct{ provider, endpoint, body string }{
		{"glm", "https://api.z.ai/api/monitor/usage/quota/limit", `{"data":{"limits":[{"type":"TOKENS_LIMIT","unit":3,"number":5,"percentage":25},{"type":"NEW_LIMIT","percentage":0}]}}`},
		{"kimi", "https://api.kimi.com/coding/v1/usages", `{"usage":{"limit":100,"used":10},"limits":[{"window":{"duration":300,"timeUnit":"TIME_UNIT_MINUTE"},"detail":{}}]}`},
		{"openrouter", "https://openrouter.ai/api/v1/key", `{"data":{"limit":100,"limit_remaining":40,"effective_budget":{}}}`},
	}
	for _, tc := range cases {
		t.Run(tc.provider, func(t *testing.T) {
			calls := 0
			http.DefaultTransport = quotaRoundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.String() != tc.endpoint {
					t.Error("unexpected request destination")
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})
			src := Source{Provider: tc.provider, Kind: "official_api", Status: "ok", endpoint: tc.endpoint, key: "test-key"}
			q := Collect(context.Background(), src)
			if q.Status != "schema_changed" || !q.At.IsZero() || len(q.Windows) != 0 {
				t.Fatal("malformed snapshot exposed partial/fresh numbers")
			}
			src.Status = "no_credentials"
			src.key = ""
			if q = Collect(context.Background(), src); q.Status != "no_credentials" || calls != 1 {
				t.Fatal("missing credential made a request")
			}
		})
	}
}

func TestPlanDiscoveryUsesSelectedConnectionKey(t *testing.T) {
	for _, cli := range []string{"codex", "opencode"} {
		t.Run(cli, func(t *testing.T) {
			home := t.TempDir()
			in := Input{CLI: cli, Home: home, Env: map[string]string{"SELECTED_KEY": "selected-key", "ANTHROPIC_AUTH_TOKEN": "unrelated-key", "OPENAI_API_KEY": "unrelated-openai"}}
			if cli == "codex" {
				writeFixture(t, filepath.Join(home, ".codex", "config.toml"), "model_provider = \"router\"\n[model_providers.router]\nbase_url = \"https://openrouter.ai/api/v1\"\nenv_key = \"SELECTED_KEY\"\n")
			} else {
				writeFixture(t, filepath.Join(home, ".config", "opencode", "opencode.json"), `{"model":"router/model","provider":{"router":{"options":{"baseURL":"https://openrouter.ai/api/v1","apiKey":"{env:SELECTED_KEY}"}},"other":{"options":{"apiKey":"wrong-key"}}}}`)
			}
			s := Discover(in, []byte("salt"))
			if s.Provider != "openrouter" || s.key != "selected-key" || s.Status != "ok" {
				t.Fatal("wrong effective connection key")
			}
		})
	}
}

func TestPlanResponseSizeIsBounded(t *testing.T) {
	q := api.Quota{Status: "ok"}
	c := &http.Client{Transport: quotaRoundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"data":{"limit":null,"limit_remaining":null}}` + strings.Repeat(" ", 1<<20)))}, nil
	})}
	if apiQuotaClient(context.Background(), Source{Provider: "openrouter", endpoint: "https://openrouter.ai/api/v1/key", key: "test-key"}, &q, c) == nil {
		t.Fatal("oversized/truncated body accepted")
	}
}
