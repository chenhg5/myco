package console

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/browserpolicy"
	"github.com/myco-ai/myco/internal/client"
)

type policyFixtureComponent struct {
	calls   atomic.Int32
	version string
}

func (f *policyFixtureComponent) Status() api.BrowserComponentStatus {
	version := f.version
	if version == "" {
		version = "0.1.0"
	}
	return api.BrowserComponentStatus{Protocol: 1, Version: version, UpdateAvailable: version == "0.1.0", Installed: true, Supported: true, Running: true}
}
func (f *policyFixtureComponent) EnsureRunning(context.Context) error { return nil }
func (f *policyFixtureComponent) Install(context.Context) error       { return nil }
func (f *policyFixtureComponent) Stop()                               {}
func (f *policyFixtureComponent) Artifact(context.Context, string, string) ([]byte, string, error) {
	return nil, "", fmt.Errorf("not_found")
}
func (f *policyFixtureComponent) Call(_ context.Context, _ string, method, path string, in, out any) error {
	f.calls.Add(1)
	var value any
	switch {
	case method == "GET" && path == "/status":
		value = api.BrowserStatus{Protocol: 1, Version: "0.1.0", Browsers: []string{"chrome"}, Profiles: []api.BrowserProfile{{ID: "profile_work", Name: "Work", Agents: true}}}
	case method == "POST" && path == "/profiles/profile_work/agents":
		value = api.BrowserProfile{ID: "profile_work", Name: "Work", Agents: true}
	default:
		return fmt.Errorf("unexpected component operation %s %s", method, path)
	}
	raw, _ := json.Marshal(value)
	return json.Unmarshal(raw, out)
}

type policyFixtureSecrets struct{ token string }

func (s *policyFixtureSecrets) Get(string) (string, error)       { return s.token, nil }
func (s *policyFixtureSecrets) Set(_ string, token string) error { s.token = token; return nil }
func (s *policyFixtureSecrets) Delete(string) error              { s.token = ""; return nil }

func TestBrowserLocalApprovalUsesNativeOwnerAndExactSnapshot(t *testing.T) {
	var offline, e2e atomic.Bool
	var requests atomic.Int32
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if offline.Load() {
			http.Error(w, "offline", 503)
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/v1/console/me"):
			owner := "owner-one"
			if r.Header.Get("Authorization") == "Bearer other-token" {
				owner = "owner-two"
			}
			json.NewEncoder(w).Encode(api.Whoami{Member: &api.Member{ID: owner, Name: owner}, Device: &api.ConsoleToken{}})
		case strings.HasSuffix(r.URL.Path, "/v1/hub/settings"):
			json.NewEncoder(w).Encode(api.HubSettings{E2E: e2e.Load()})
		case strings.Contains(r.URL.Path, "/v1/pages/"):
			json.NewEncoder(w).Encode(api.Page{ID: filepath.Base(r.URL.Path), Name: "Reports"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer hub.Close()
	home := t.TempDir()
	store := NewStore(filepath.Join(home, "hubs.json"))
	h, err := store.Add("Fixture", hub.URL)
	if err != nil {
		t.Fatal(err)
	}
	secrets := &policyFixtureSecrets{token: "fixture-token"}
	native := &policyFixtureComponent{}
	b := &Backend{Store: store, Secrets: secrets}
	b.browser.component = native
	c, _ := client.New(h.URL, secrets.token)
	ctx := context.Background()
	g := b.grants()
	g.mu.Lock()
	g.perm(h.ID, "pg_test").once["browser"] = true
	g.perm(h.ID, "pg_other").once["browser"] = true
	g.mu.Unlock()
	mux := http.NewServeMux()
	b.browserRoutes(mux)
	confirm := func(action string, args json.RawMessage) string {
		t.Helper()
		body, _ := json.Marshal(browserCall{Page: "pg_test", Instance: "frame", Action: action, Args: args})
		r := httptest.NewRequest("POST", "http://app/app/hubs/"+h.ID+"/browser/confirm", strings.NewReader(string(body)))
		r.Header.Set("X-Myco-Browser-Owner", "1")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("confirm: %d %s", w.Code, w.Body.String())
		}
		var result struct{ Confirmation string }
		json.Unmarshal(w.Body.Bytes(), &result)
		return result.Confirmation
	}
	call := func(action string, args json.RawMessage, nonce string) (any, error) {
		return b.browserCall(ctx, c, h, browserCall{Page: "pg_test", Instance: "frame", Action: action, Args: args, Confirmation: nonce})
	}
	value, err := call("status", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if !value.(api.BrowserStatus).UpdateAvailable || value.(api.BrowserStatus).Version != "0.1.0" {
		t.Fatal("old component status lost update_available")
	}
	if value.(api.BrowserStatus).Profiles[0].Agents {
		t.Fatal("legacy global opt-in was inherited")
	}
	flag := json.RawMessage(`{"id":"profile_work","allowed":true}`)
	if _, err := call("profiles.allowAgents", flag, ""); err == nil {
		t.Fatal("page boolean enabled profile")
	}
	if _, err := call("profiles.allowAgents", flag, confirm("profiles.allowAgents", flag)); err != nil {
		t.Fatal(err)
	}
	// Updating the component must not silently remove an opted-in agent host.
	for version, want := range map[string]int{"0.1.0": 1, "0.1.1": 1, "0.2.0": 1, "0.1.2": 0} {
		native.version = version
		if got := len(b.browserHostPolicies(h)); got != want {
			t.Fatalf("component %s hosts: %d, want %d", version, got, want)
		}
	}
	native.version = ""
	request := browserpolicy.Request{ScriptID: "report", Version: 3, Profile: "profile_work", Script: json.RawMessage(`{"name":"Monthly report","profile":"profile_work","start_url":"https://example.com/reports","params":[],"graph":{"nodes":[{"id":"title","type":"js","code":"return document.title"}],"edges":[]},"on_error":"stop"}`), Scripts: map[string]json.RawMessage{}}
	raw, _ := json.Marshal(request)
	value, err = call("approvals.review", raw, "")
	if err != nil {
		t.Fatal(err)
	}
	review := value.(browserpolicy.Review)
	if len(review.Code) != 1 || review.Code[0].Code != "return document.title" || review.ProfileName != "Work" || len(review.URLs) != 1 {
		t.Fatalf("native review: %+v", review)
	}
	if _, err := call("approvals.request", raw, ""); err == nil {
		t.Fatal("unconfirmed page request persisted approval")
	}
	nonce := confirm("approvals.request", raw)
	changed := request
	changed.Script = json.RawMessage(strings.Replace(string(request.Script), "return document.title", "return document.cookie", 1))
	changedRaw, _ := json.Marshal(changed)
	if _, err := call("approvals.request", changedRaw, nonce); err == nil {
		t.Fatal("nonce approved substituted script")
	}
	nonce = confirm("approvals.request", raw)
	value, err = call("approvals.request", raw, nonce)
	if err != nil {
		t.Fatal(err)
	}
	// A conflicting approval must not leave a temporary native block behind.
	conflictNonce := confirm("approvals.request", changedRaw)
	if _, err := call("approvals.request", changedRaw, conflictNonce); err == nil {
		t.Fatal("changed version approved")
	}
	if b.browserAgentsBlocked(h.ID, request.Profile) {
		t.Fatal("failed approval left profile blocked")
	}
	approval := value.(api.BrowserApproval)
	if approval.Digest != review.Digest || approval.Version != 3 {
		t.Fatalf("approved snapshot: %+v", approval)
	}
	if _, err := call("approvals.request", raw, nonce); err == nil {
		t.Fatal("approval nonce reused")
	}
	value, err = call("approvals.list", json.RawMessage(`{}`), "")
	if err != nil || len(value.([]api.BrowserApproval)) != 1 {
		t.Fatalf("list: %+v %v", value, err)
	}
	encoded, _ := json.Marshal(value)
	if strings.Contains(string(encoded), "document.title") || strings.Contains(string(encoded), "graph") {
		t.Fatal("list disclosed script content")
	}
	otherPage := browserCall{Page: "pg_other", Instance: "frame-other", Action: "approvals.list", Args: json.RawMessage(`{}`)}
	value, err = b.browserCall(ctx, c, h, otherPage)
	if err != nil || len(value.([]api.BrowserApproval)) != 0 {
		t.Fatalf("other page: %+v %v", value, err)
	}
	secrets.token = "other-token"
	other, _ := client.New(h.URL, secrets.token)
	value, err = b.browserCall(ctx, other, h, browserCall{Page: "pg_test", Instance: "frame", Action: "approvals.list", Args: json.RawMessage(`{}`)})
	if err != nil || len(value.([]api.BrowserApproval)) != 0 {
		t.Fatalf("other account: %+v %v", value, err)
	}
	secrets.token = "fixture-token"
	if _, err = call("status", nil, ""); err != nil {
		t.Fatal(err)
	}
	var stopped atomic.Int32
	b.browser.cancelAgents = func(hub, profile string) {
		if hub != h.ID || profile != "profile_work" {
			t.Error("wrong revocation scope")
		}
		stopped.Add(1)
	}
	offline.Store(true)
	before := requests.Load()
	value, err = call("profiles.allowAgents", json.RawMessage(`{"id":"profile_work","allowed":false}`), "")
	if err != nil || value.(api.BrowserProfile).Agents || stopped.Load() != 1 || requests.Load() != before {
		t.Fatalf("offline native revoke: %+v %v stopped=%d hubCalls=%d", value, err, stopped.Load(), requests.Load()-before)
	}
	policies, _ := b.browserPolicies()
	p, err := policies.Policy(browserpolicy.Scope{Workspace: h.URL, Owner: "owner-one"}, "profile_work")
	if err != nil || p.Enabled {
		t.Fatalf("offline persisted opt-out: %+v %v", p, err)
	}
	offline.Store(false)
	beforeNative := native.calls.Load()
	for _, kind := range []string{"js", "extract", "assert", "branch"} {
		runArgs := json.RawMessage(`{"script":{"name":"Unsafe","graph":{"nodes":[{"id":"unsafe-step","type":"` + kind + `","code":"return '${from_page}';"}]}},"profile":"profile_work"}`)
		if _, err := call("runs.start", runArgs, ""); err == nil {
			t.Fatal("human run interpolated untrusted code")
		} else if e, ok := client.AsError(err); !ok || e.Code != "invalid_graph" {
			t.Fatalf("code guard: %v", err)
		}
	}
	if native.calls.Load() != beforeNative {
		t.Fatal("unsafe human code reached component")
	}
	e2e.Store(true)
	if _, err := call("approvals.review", raw, ""); err == nil {
		t.Fatal("encrypted workspace offered agent approval")
	}
}
