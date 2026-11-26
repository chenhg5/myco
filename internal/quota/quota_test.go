package quota

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
)

func writeFixture(t *testing.T, path, data string) {
	t.Helper()
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(path, []byte(data), 0600); e != nil {
		t.Fatal(e)
	}
}
func TestSourceIsolationAndEffectiveProvider(t *testing.T) {
	home := t.TempDir()
	salt := []byte("local-random-identity")
	auth := filepath.Join(home, ".codex", "auth.json")
	writeFixture(t, auth, `{"tokens":{"account_id":"account-a","access_token":"not-a-real-token"}}`)
	in := Input{CLI: "codex", Home: home, Env: map[string]string{}}
	a := Discover(in, salt)
	b := Discover(in, salt)
	if a.Ref != b.Ref || a.Provider != "codex" || a.Status != "ok" {
		t.Fatalf("native discovery mismatch: %s", a.Status)
	}
	writeFixture(t, auth, `{"tokens":{"account_id":"account-b","access_token":"other-token"}}`)
	if Discover(in, salt).Ref == a.Ref {
		t.Fatal("account switch retained reference")
	}
	in.Env = map[string]string{"OPENAI_API_KEY": "api-key"}
	if got := Discover(in, salt); got.Status != "unsupported" || got.Ref == a.Ref {
		t.Fatal("API key was identified as a subscription")
	}
	in = Input{CLI: "claude", Home: home, Pinned: true, Host: "api.minimax.io", Env: map[string]string{"ANTHROPIC_BASE_URL": "https://api.minimax.io/anthropic", "ANTHROPIC_AUTH_TOKEN": "plan-key"}}
	a = Discover(in, salt)
	if a.Provider != "minimax" || a.Status != "ok" {
		t.Fatal("effective MiniMax provider not found")
	}
	in.Env["ANTHROPIC_AUTH_TOKEN"] = "next-key"
	if Discover(in, salt).Ref == a.Ref {
		t.Fatal("key change did not change binding")
	}
	in.Env["ANTHROPIC_BASE_URL"] = "https://gateway.example/anthropic"
	if got := Discover(in, salt); got.Status != "unsupported" {
		t.Fatal("gateway key would be forwarded")
	}
	in.Env["ANTHROPIC_BASE_URL"] = "https://api.minimax.io:444/anthropic"
	if Discover(in, salt).Status != "unsupported" {
		t.Fatal("nonstandard origin accepted")
	}
	in.Env = map[string]string{"ANTHROPIC_BASE_URL": "https://api.minimax.io/anthropic"}
	if Discover(in, salt).Status != "no_credentials" {
		t.Fatal("missing plan key not reported")
	}
}
func TestOpenCodeJSONCUsesOnlySelectedProvider(t *testing.T) {
	home := t.TempDir()
	writeFixture(t, filepath.Join(home, ".config", "opencode", "opencode.jsonc"), `{
 // Keep a URL and a trailing comma
 "model":"opencode-go/model", "provider":{"opencode-go":{"options":{"apiKey":"{env:GO_KEY}","baseURL":"https://opencode.ai/inference/go",},},},
 }`)
	in := Input{CLI: "opencode", Home: home, Env: map[string]string{"GO_KEY": "local-test-key"}}
	s := Discover(in, []byte("salt"))
	if s.Provider != "opencode_go" || s.Status != "ok" || s.key != "local-test-key" {
		t.Fatalf("Go not discovered: %s", s.Status)
	}
	in.Model = "other/model"
	if got := Discover(in, []byte("salt")); got.Status != "unsupported" {
		t.Fatal("unselected Go key was used")
	}
}
func TestQuotaParsersDoNotInventMissingWindows(t *testing.T) {
	q := api.Quota{}
	if e := parseCodex([]byte(`{"rateLimits":{"primary":{"usedPercent":82,"windowDurationMins":10080,"resetsAt":1767225600}}}`), &q); e != nil {
		t.Fatal(e)
	}
	if len(q.Windows) != 1 || q.Windows[0].DurationSec != 604800 || math.Abs(q.Windows[0].Used-.82) > 1e-9 {
		t.Fatal(q.Windows)
	}
	q = api.Quota{}
	if e := parseGo([]byte(`{"usage":{"rolling":{"percent":0,"resetsAt":null}}}`), &q); e != nil || len(q.Windows) != 1 || q.Windows[0].Used != 0 {
		t.Fatal("explicit zero not accepted")
	}
	q = api.Quota{}
	if e := parseGo([]byte(`{"usage":{"rolling":{"status":"ok"}}}`), &q); e == nil {
		t.Fatal("missing percent treated as zero")
	}
	q = api.Quota{}
	if e := parseMiniMax([]byte(`{"base_resp":{"status_code":0},"model_remains":[{"model_name":"general","current_interval_remaining_percent":18,"end_time":1767225600000,"current_weekly_status":3,"current_weekly_remaining_percent":100}]}`), &q); e != nil || len(q.Windows) != 1 || math.Abs(q.Windows[0].Used-.82) > 1e-9 || q.Windows[0].ResetsAt.Unix() != 1767225600 {
		t.Fatal("MiniMax remaining direction or missing weekly semantics")
	}
	q = api.Quota{}
	if e := parseMiniMax([]byte(`{"new_schema":true}`), &q); e == nil {
		t.Fatal("unknown schema accepted")
	}
}
func TestClaudeCacheBindingAndObservationTime(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".claude.json")
	writeFixture(t, path, `{"oauthAccount":{"accountUuid":"a"},"cachedUsageUtilization":{"accountUuid":"a","fetchedAtMs":1795600000000,"utilization":{"five_hour":{"utilization":25,"resets_at":"2026-11-26T10:00:00Z"},"seven_day_opus":{"utilization":70}}}}`)
	src := Discover(Input{CLI: "claude", Home: home, Env: map[string]string{}}, []byte("salt"))
	q := Collect(context.Background(), src)
	if q.Status != "ok" || q.At.UnixMilli() != 1795600000000 || len(q.Windows) != 2 {
		t.Fatalf("cache not collected: %s", q.Status)
	}
	writeFixture(t, path, `{"oauthAccount":{"accountUuid":"b"},"cachedUsageUtilization":{"accountUuid":"a","fetchedAtMs":1795600000000,"utilization":{"five_hour":{"utilization":25}}}}`)
	next := Discover(Input{CLI: "claude", Home: home, Env: map[string]string{}}, []byte("salt"))
	if next.Ref == src.Ref {
		t.Fatal("account changed but reference stayed")
	}
	q = Collect(context.Background(), next)
	if len(q.Windows) != 0 || q.Status == "ok" {
		t.Fatal("another account's cache leaked")
	}
}
func TestSharedCacheFailuresAndRetryAfter(t *testing.T) {
	var requests atomic.Int32
	var denied atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("Authorization") != "Bearer private-plan-key" {
			t.Error("missing auth")
		}
		if denied.Load() {
			w.Header().Set("Retry-After", "1800")
			w.WriteHeader(429)
			w.Write([]byte(`{"secret":"never report this"}`))
			return
		}
		w.Write([]byte(`{"usage":{"weekly":{"percent":82,"resetsAt":"2026-11-30T00:00:00Z"}}}`))
	}))
	defer srv.Close()
	dir := t.TempDir()
	a, e := NewStore(dir)
	if e != nil {
		t.Fatal(e)
	}
	b, e := NewStore(dir)
	if e != nil {
		t.Fatal(e)
	}
	src := Source{Provider: "opencode_go", Plan: "go", Kind: "official_api", Status: "ok", Ref: ref(a.salt, "go"), endpoint: srv.URL, key: "private-plan-key"}
	q := a.Get(context.Background(), src)
	if q.Status != "ok" {
		t.Fatal(q.Status)
	}
	other := b.Get(context.Background(), src)
	if requests.Load() != 1 || other.At != q.At {
		t.Fatal("same login sampled twice")
	}
	denied.Store(true)
	var c cached
	readJSON(a.path(src), &c)
	c.Next = time.Time{}
	if e = a.write(a.path(src), c); e != nil {
		t.Fatal(e)
	}
	failure := b.Get(context.Background(), src)
	if failure.Status != "throttled" || failure.At != q.At || len(failure.Windows) != 1 {
		t.Fatal("failed read erased successful observation")
	}
	readJSON(a.path(src), &c)
	if c.Next.Before(failure.CheckedAt.Add(29 * time.Minute)) {
		t.Fatal("Retry-After not honored")
	}
	raw, e := os.ReadFile(a.path(src))
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(raw), "private-plan-key") || strings.Contains(string(raw), "never report") {
		t.Fatal("secret saved")
	}
	if e = json.Unmarshal(raw, &c); e != nil {
		t.Fatal(e)
	}
	// Another login never inherits this snapshot.
	src.Ref = ref(a.salt, "another-login")
	src.Status = "no_credentials"
	fresh := b.Get(context.Background(), src)
	if len(fresh.Windows) != 0 || !fresh.At.IsZero() {
		t.Fatal("cross-account reuse")
	}
}
func TestOfficialQueryRejectsRedirects(t *testing.T) {
	var reached atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached.Store(true) }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer redirect.Close()
	q := Collect(context.Background(), Source{Provider: "opencode_go", Kind: "official_api", Status: "ok", endpoint: redirect.URL, key: "private-test-key"})
	if reached.Load() || q.Status == "ok" {
		t.Fatal("credential-bearing redirect followed")
	}
}

func TestIncrementalEventsRetainOtherWindowsAndTheirAge(t *testing.T) {
	store, e := NewStore(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	src := Source{Provider: "claude", Ref: ref(store.salt, "claude"), Kind: "cache"}
	at := time.Now().Add(-time.Hour).UTC()
	q := api.Quota{Source: "claude", Provider: "claude", ProfileRef: src.Ref, Status: "ok", SourceKind: "event", At: at, CheckedAt: at, Windows: []api.QuotaWindow{{Name: "five_hour", Used: .2}}}
	if store.Event(src, q) == nil {
		t.Fatal("first event lost")
	}
	q.At = at.Add(time.Minute)
	q.CheckedAt = q.At
	q.Windows = []api.QuotaWindow{{Name: "seven_day", Used: .5}}
	merged := store.Event(src, q)
	if merged == nil || len(merged.Windows) != 2 || !merged.At.Equal(at) {
		t.Fatal("single-window event erased other window or made it fresh")
	}
	q.At = at.Add(2 * time.Minute)
	q.CheckedAt = q.At
	q.Windows = []api.QuotaWindow{{Name: "five_hour", Used: .3}}
	merged = store.Event(src, q)
	if merged == nil || len(merged.Windows) != 2 || !merged.At.Equal(at.Add(time.Minute)) {
		t.Fatal("oldest included observation did not advance")
	}
	q.At = at
	q.Windows = []api.QuotaWindow{{Name: "five_hour", Used: .1}}
	merged = store.Event(src, q)
	for _, w := range merged.Windows {
		if w.Name == "five_hour" && w.Used != .3 {
			t.Fatal("late event overwrote a newer window")
		}
	}
}

func TestFullCacheDoesNotRewindANewerNativeWindow(t *testing.T) {
	store, e := NewStore(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(t.TempDir(), "profile.json")
	src := Source{Provider: "claude", Ref: ref(store.salt, "same-login"), Kind: "cache", Status: "ok", endpoint: path}
	at := time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond)
	q := api.Quota{Source: "claude", Provider: "claude", ProfileRef: src.Ref, Status: "ok", SourceKind: "event", At: at, CheckedAt: time.Now().UTC(), Windows: []api.QuotaWindow{{Name: "five_hour", Used: .2}}}
	store.Event(src, q)
	q.At = at.Add(time.Minute)
	q.Windows = []api.QuotaWindow{{Name: "seven_day", Used: .5}}
	store.Event(src, q)
	q.At = at.Add(2 * time.Minute)
	q.Windows = []api.QuotaWindow{{Name: "five_hour", Used: .3}}
	store.Event(src, q)
	raw, _ := json.Marshal(map[string]any{"oauthAccount": map[string]any{"accountUuid": "demo"}, "cachedUsageUtilization": map[string]any{"accountUuid": "demo", "fetchedAtMs": at.Add(90 * time.Second).UnixMilli(), "utilization": map[string]any{"five_hour": map[string]any{"utilization": 10}, "seven_day": map[string]any{"utilization": 60}}}})
	writeFixture(t, path, string(raw))
	var saved cached
	readJSON(store.path(src), &saved)
	saved.Next = time.Time{}
	if e = store.write(store.path(src), saved); e != nil {
		t.Fatal(e)
	}
	result := store.Get(context.Background(), src)
	if len(result.Windows) != 2 || !result.At.Equal(at.Add(90*time.Second)) {
		t.Fatal("wrong merged observation age")
	}
	for _, w := range result.Windows {
		if w.Name == "five_hour" && w.Used != .3 || w.Name == "seven_day" && w.Used != .6 {
			t.Fatal("a stale full cache rewound a newer live event")
		}
	}
}

func TestMalformedCachedNumbersAreNotDisplayedAsZeroUsed(t *testing.T) {
	store, e := NewStore(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	src := Source{Provider: "minimax", Ref: ref(store.salt, "missing-key"), Status: "no_credentials"}
	raw := `{"quota":{"provider":"minimax","profile_ref":"` + src.Ref + `","status":"ok","windows":[{"name":"five_hour","used":"not-a-number"}]},"next":"2099-01-01T00:00:00Z"}`
	writeFixture(t, store.path(src), raw)
	q := store.Get(context.Background(), src)
	if q.Status != "no_credentials" || len(q.Windows) != 0 {
		t.Fatal("bad cache was reported as a full allowance")
	}
}

func TestMatchingForeignClaudeCacheCannotBeAssignedToAnOldReference(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".claude.json")
	writeFixture(t, path, `{"oauthAccount":{"accountUuid":"first-fixture"}}`)
	src := Discover(Input{CLI: "claude", Home: home, Env: map[string]string{}}, []byte("salt"))
	raw, _ := json.Marshal(map[string]any{"oauthAccount": map[string]any{"accountUuid": "second-fixture"}, "cachedUsageUtilization": map[string]any{"accountUuid": "second-fixture", "fetchedAtMs": time.Now().UnixMilli(), "utilization": map[string]any{"five_hour": map[string]any{"utilization": 80}}}})
	writeFixture(t, path, string(raw))
	q := Collect(context.Background(), src)
	if q.Status == "ok" || len(q.Windows) != 0 {
		t.Fatal("foreign cached account was assigned to old reference")
	}
}

func TestSelectedGoGatewayDoesNotForwardItsKey(t *testing.T) {
	home := t.TempDir()
	writeFixture(t, filepath.Join(home, ".config", "opencode", "opencode.json"), `{"model":"opencode-go/demo","provider":{"opencode-go":{"options":{"baseURL":"https://gateway.example/go","apiKey":"gateway-only-fixture"}}}}`)
	src := Discover(Input{CLI: "opencode", Home: home, Env: map[string]string{}}, []byte("salt"))
	if src.Status != "unsupported" || src.key != "" {
		t.Fatal("gateway credentials selected for official endpoint")
	}
}

func TestProjectAccountOverrideCannotUseGlobalLogin(t *testing.T) {
	for _, cli := range []string{"codex", "claude", "opencode"} {
		t.Run(cli, func(t *testing.T) {
			home := t.TempDir()
			work := filepath.Join(home, "project", "nested")
			if e := os.MkdirAll(work, 0700); e != nil {
				t.Fatal(e)
			}
			in := Input{CLI: cli, Home: home, Workdir: work, Env: map[string]string{}}
			var path, body string
			switch cli {
			case "codex":
				path = ".codex/config.toml"
				body = `model_provider="gateway"`
			case "claude":
				path = ".claude/settings.local.json"
				body = `{"env":{"ANTHROPIC_AUTH_TOKEN":"project-fixture"}}`
			case "opencode":
				path = "opencode.jsonc"
				body = `{"provider":{"opencode-go":{"options":{"apiKey":"project-fixture"}}}}`
			}
			writeFixture(t, filepath.Join(home, "project", path), body)
			if !projectAccountOverride(in) || Discover(in, []byte("salt")).Status != "unsupported" {
				t.Fatal("project account override was assigned global credentials")
			}
			if cli == "claude" {
				writeFixture(t, filepath.Join(home, "project", path), `{"permissions":{"allow":[]},"env":{"TERM":"xterm"}}`)
				if projectAccountOverride(in) {
					t.Fatal("unrelated project settings blocked collection")
				}
			}
		})
	}
}

func TestOpenCodeZenIsNotAGoPlan(t *testing.T) {
	src := Discover(Input{CLI: "opencode", Home: t.TempDir(), Model: "opencode/demo", Env: map[string]string{"OPENCODE_BASE_URL": "https://opencode.ai/inference/v1", "OPENCODE_API_KEY": "zen-fixture"}}, []byte("salt"))
	if src.Status != "unsupported" || src.key != "" {
		t.Fatal("Zen connection was assigned Go plan quota")
	}
}
