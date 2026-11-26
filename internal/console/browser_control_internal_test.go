package console

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/browserautomation"
	"github.com/myco-ai/myco/internal/browserpolicy"
	"github.com/myco-ai/myco/internal/client"
)

type controlFixtureComponent struct {
	policyFixtureComponent

	mu        sync.Mutex
	openCount int
	stopped   bool
	forced    chan string
	closeHook func(context.Context, api.BrowserControlArgs) error
}

func (f *controlFixtureComponent) Call(ctx context.Context, scope, method, path string, in, out any) error {
	if !strings.HasPrefix(scope, "agent-") && !(scope == "control-discovery" && method == "GET" && path == "/status") {
		return errors.New("wrong control scope")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var value any
	switch {
	case method == "GET" && path == "/status":
		value = map[string]any{"protocol": 1, "features": []string{"agent_control", "agent_replay"}}
	case method == "POST" && path == "/control/open":
		args := in.(api.BrowserControlArgs)
		var actor api.BrowserControlActor
		_ = json.Unmarshal(args["actor"], &actor)
		if actor.ID != "browser-agent" || actor.Name == "" || args.String("grant_id") != "grant" ||
			args.String("request_id") == "" || args.String("profile") != "work" || len(args["expires_at"]) == 0 || args["headless"] != nil {
			return errors.New("host sent invalid native authorization")
		}
		f.openCount++
		value = map[string]any{"session": fmt.Sprintf("session%d", f.openCount), "active_tab": "tab1", "start_url": "https://start.example.com/", "snapshot": map[string]any{"snapshot_id": "snapshot1", "url": "https://example.com/redirected"}}
	case method == "GET" && strings.HasPrefix(path, "/control/events?"):
		value = api.BrowserControlEvents{Next: 1, Events: []api.BrowserControlEvent{{Host: "example.com"}}}
		if f.stopped {
			value = api.BrowserControlEvents{Done: true, Reason: api.BrowserControlOwnerStopped, Next: 2}
		}
	case method == "POST" && path == "/control/snapshot":
		value = map[string]any{"snapshot_id": "snapshot2", "active_tab": "tab1", "nodes": []map[string]string{{"ref": "e1"}}}
	case method == "POST" && path == "/control/act":
		args := in.(api.BrowserControlArgs)
		if args.String("request_id") == "" || args.String("session") != "session1" {
			return errors.New("host lost native request identity")
		}
		if args.String("ref") == "e2" {
			return &browserautomation.ComponentError{Code: "outcome_unknown"}
		}
		value = map[string]any{"active_tab": "tab1", "step_ids": []string{"step1"}, "snapshot_error": map[string]string{"code": "timeout"}}
	case method == "POST" && path == "/control/run":
		value = map[string]any{"ok": true, "state": "succeeded", "steps": []any{}, "output": map[string]any{}, "url": "https://example.com/", "tabs": []any{}}
	case method == "POST" && path == "/control/extract":
		value = map[string]string{"text": "Sample"}
	case method == "POST" && path == "/control/screenshot":
		value = map[string]any{"screenshot": map[string]string{"id": "a_fixture"}, "image": map[string]int{"width": 800, "height": 600}}
	case method == "POST" && path == "/control/close":
		if fields, ok := in.(map[string]string); ok {
			f.forced <- fields["reason"]
			break
		}
		if f.closeHook != nil {
			if err := f.closeHook(ctx, in.(api.BrowserControlArgs)); err != nil {
				return err
			}
		}
		value = map[string]any{"steps": []map[string]string{{"id": "step1", "type": "click"}}, "removed": []string{}, "next": 1, "done": true, "reason": "closed"}
	default:
		return fmt.Errorf("unexpected native call: %s %s", method, path)
	}
	if out == nil {
		return nil
	}
	raw, _ := json.Marshal(value)
	return json.Unmarshal(raw, out)
}

func (f *controlFixtureComponent) Artifact(_ context.Context, scope, id string) ([]byte, string, error) {
	if !strings.HasPrefix(scope, "agent-") || id != "a_fixture" {
		return nil, "", errors.New("wrong screenshot scope")
	}
	return []byte("private-test-image"), "image/png", nil
}

func TestBrowserControlHostLifecycle(t *testing.T) {
	expiry := time.Now().Add(time.Minute)
	revoked := make(chan api.ActionGrantRevoke, 1)
	var openCompleted atomic.Bool
	var registration atomic.Int32
	var generation atomic.Value
	generation.Store("initial")
	watchedGrant := make(chan int, 8)
	var grantLookups atomic.Int32

	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/action-hosts"):
			var input api.ActionHostRegister
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Error(err)
			}
			generation.Store(input.Generation)
			registration.Add(1)
			_ = json.NewEncoder(w).Encode(api.ActionHost{ID: "host", MachineID: input.MachineID, Generation: input.Generation, Provider: "browser", Resource: input.Resource})
		case strings.HasSuffix(r.URL.Path, "/revoke"):
			var input api.ActionGrantRevoke
			_ = json.NewDecoder(r.Body).Decode(&input)
			revoked <- input
			_ = json.NewEncoder(w).Encode(api.ActionGrant{ID: "grant", State: api.GrantRevoked})
		case strings.HasSuffix(r.URL.Path, "/artifacts"):
			_ = json.NewEncoder(w).Encode(api.ActionArtifact{ID: "art_fixture", MediaType: "image/png", ExpiresAt: expiry})
		case strings.HasSuffix(r.URL.Path, "/action-grants"):
			if r.URL.Query().Get("state") != "" {
				t.Error("terminal grant states must be visible")
			}
			lookup := int(grantLookups.Add(1))
			defer func() { watchedGrant <- lookup }()
			switch lookup {
			case 1:
				w.WriteHeader(http.StatusNotFound)
			case 2:
				w.WriteHeader(http.StatusInternalServerError)
			case 3:
				_ = json.NewEncoder(w).Encode(api.ActionGrantList{Grants: []api.ActionGrant{}})
			default:
				_ = json.NewEncoder(w).Encode(api.ActionGrantList{Grants: []api.ActionGrant{{ID: "grant", State: api.GrantActive, ExpiresAt: &expiry}}})
			}

		default:
			if strings.Contains(r.URL.Path, "/action-jobs/") && r.URL.Query().Get("generation") != generation.Load().(string) {
				w.WriteHeader(http.StatusConflict)
				return
			}
			if r.URL.Query().Get("wait") == "30" {
				<-r.Context().Done()
				return
			}
			// A refreshed host generation cannot inspect the old job, which
			// can also disappear once completed. Neither owns the session.
			if strings.HasSuffix(r.URL.Path, "/action-jobs/job1") && openCompleted.Load() {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(api.ActionHostJobState{Job: api.ActionJob{State: api.JobSucceeded}, GrantState: api.GrantActive, GrantExpiresAt: &expiry})
		}
	}))
	defer hub.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := Hub{ID: "hub", URL: hub.URL}
	c, _ := client.New(h.URL, "fixture")
	b := &Backend{Store: NewStore(filepath.Join(t.TempDir(), "hubs.json")), Secrets: &policyFixtureSecrets{token: "fixture"}}
	comp := &controlFixtureComponent{forced: make(chan string, 1)}
	b.browser.component = comp
	scope := browserpolicy.Scope{Workspace: h.URL, Owner: "owner"}
	b.browser.identities = map[string]browserOwnerIdentity{h.ID: {URL: h.URL, Owner: scope.Owner, Credential: c.CredentialFingerprint()}}
	policies, _ := b.browserPolicies()
	policy, err := policies.SetEnabled(scope, "work", true, "Work")
	if err != nil {
		t.Fatal(err)
	}
	p := browserHostPolicy{scope, policy}
	host, err := b.registerBrowserHost(ctx, c, "machine", p)
	if err != nil {
		t.Fatal(err)
	}
	grant := api.ActionGrant{ID: "grant", Agent: "browser-agent", State: api.GrantActive, ExpiresAt: &expiry,
		Permissions: []string{"browser.control"}, Target: api.ActionTarget{Kind: "machine", ID: "machine", Resource: host.Resource}}
	index := 0
	call := func(action, args string, g api.ActionGrant) api.ActionJobComplete {
		index++
		return b.runBrowserJob(ctx, c, host, api.ActionHostJob{
			Job:  api.ActionJob{ID: fmt.Sprintf("job%d", index), Action: "browser.page." + action, Version: 1},
			Args: json.RawMessage(args), Grant: g,
		}, h, p)
	}
	wantCode := func(done api.ActionJobComplete, code string) {
		t.Helper()
		if done.Error == nil || done.Error.Code != code {
			t.Fatalf("want %s, got %+v", code, done)
		}
	}
	open := `{"profile":"work","url":"https://example.com/"}`
	session := `{"session":"session1"}`
	if done := call("open", open, grant); done.State != api.JobSucceeded || done.BrowserHost != "example.com" {
		t.Fatalf("open = %+v", done)
	}
	openCompleted.Store(true)
	previousGeneration := host.Generation
	host, err = b.registerBrowserHost(ctx, c, "machine", p)
	if err != nil || host.Generation == previousGeneration || registration.Load() != 2 {
		t.Fatalf("host was not replaced: %+v %v", host, err)
	}
	if _, err := c.ActionHostJob(ctx, "job1", "host", previousGeneration, 0); err == nil {
		t.Fatal("old generation unexpectedly remained valid")
	}
	if _, err := c.ActionHostJob(ctx, "job1", "host", host.Generation, 0); err == nil {
		t.Fatal("old completed job unexpectedly remained available")
	}
	replayArgs := `{"session":"session1","graph":{"nodes":[{"id":"note","type":"note"}],"edges":[]}}`
	if done := call("run", replayArgs, grant); done.State != api.JobSucceeded {
		t.Fatal("replay failed", done)
	}
	for want := 1; want <= 5; want++ {
		select {
		case got := <-watchedGrant:
			if got != want {
				t.Fatalf("grant lookup order: %d, want %d", got, want)
			}
		case <-time.After(8 * time.Second):
			t.Fatal("session stopped during host replacement or transient grant lookup failure")
		}
	}
	// Two real registrations, a missing job, HTTP 404/500 and a temporary
	// missing grant must not end this session before successful verification.
	if done := call("screenshot", session, grant); done.State != api.JobSucceeded {
		t.Fatal("successful replay ended the session", done)
	}
	wantCode(call("open", open, grant), "busy")
	other := grant
	other.ID = "another-grant"
	wantCode(call("snapshot", session, other), "not_found")
	if done := call("snapshot", session, grant); done.State != api.JobSucceeded || !strings.Contains(string(done.Result), "snapshot2") {
		t.Fatalf("snapshot = %+v", done)
	}
	act := `{"session":"session1","op":"click","ref":"e1"}`
	if done := call("act", act, grant); done.State != api.JobSucceeded || !strings.Contains(string(done.Result), "snapshot_error") {
		t.Fatalf("act = %+v", done)
	}
	wantCode(call("act", `{"session":"session1","op":"click","ref":"e2"}`, grant), "outcome_unknown")
	if done := call("extract", `{"session":"session1","ref":"e1","what":"text"}`, grant); done.State != api.JobSucceeded {
		t.Fatalf("extract = %+v", done)
	}
	if done := call("screenshot", session, grant); done.State != api.JobSucceeded || len(done.ArtifactIDs) != 1 || strings.Contains(string(done.Result), "a_fixture") {
		t.Fatalf("screenshot = %+v", done)
	}
	closed := call("close", session, grant)
	if closed.State != api.JobSucceeded || !strings.Contains(string(closed.Result), `"reason":"closed"`) || !strings.Contains(string(closed.Result), `"steps"`) {
		t.Fatalf("explicit close did not return the draft: %+v", closed)
	}
	if done := call("open", open, grant); done.State != api.JobSucceeded {
		t.Fatalf("profile did not become available after closing: %+v", done)
	}
	second := b.ownedBrowserControl("session2", h.ID, grant.Agent, grant.ID, p.Policy.Profile, p.Policy.Binding)
	if second == nil {
		t.Fatal("second control session was not registered")
	}
	second.mu.Lock()
	second.lastActive = time.Now().Add(-browserControlIdle)
	second.mu.Unlock()
	select {
	case <-second.ctx.Done():
	case <-time.After(8 * time.Second):
		t.Fatal("idle control session did not expire")
	}
	select {
	case reason := <-comp.forced:
		if reason != "idle_timeout" {
			t.Fatalf("unexpected forced close reason %q", reason)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("idle session was not discarded by the component")
	}
	wantCode(call("snapshot", `{"session":"session2"}`, grant), "session_expired")
	if done := call("open", open, grant); done.State != api.JobSucceeded {
		t.Fatalf("profile did not become available after idle expiry: %+v", done)
	}
	comp.mu.Lock()
	comp.stopped = true
	comp.mu.Unlock()
	select {
	case in := <-revoked:
		if in.Reason != api.BrowserControlOwnerStopped {
			t.Fatalf("unexpected revocation reason %s", in.Reason)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("owner stop did not revoke the grant")
	}
	stopped := call("close", `{"session":"session3"}`, grant)
	wantCode(stopped, "session_closed")
	if stopped.Error.Reason != api.BrowserControlOwnerStopped || len(stopped.Result) != 0 {
		t.Fatalf("stopped control exposed its draft: %+v", stopped)
	}
}

func TestBrowserControlCloseKeepsReceiptAfterTerminalEvent(t *testing.T) {
	expiry := time.Now().Add(time.Minute)
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("wait") == "30" {
			<-r.Context().Done()
			return
		}
		json.NewEncoder(w).Encode(api.ActionHostJobState{GrantState: api.GrantActive, GrantExpiresAt: &expiry})
	}))
	defer hub.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := Hub{ID: "hub", URL: hub.URL}
	c, _ := client.New(h.URL, "fixture")
	b := &Backend{Store: NewStore(filepath.Join(t.TempDir(), "hubs.json")), Secrets: &policyFixtureSecrets{token: "fixture"}}
	comp := &controlFixtureComponent{forced: make(chan string, 1)}
	b.browser.component = comp
	scope := browserpolicy.Scope{Workspace: h.URL, Owner: "owner"}
	b.browser.identities = map[string]browserOwnerIdentity{h.ID: {URL: h.URL, Owner: scope.Owner, Credential: c.CredentialFingerprint()}}
	policies, _ := b.browserPolicies()
	policy, err := policies.SetEnabled(scope, "work", true, "Work")
	if err != nil {
		t.Fatal(err)
	}
	p := browserHostPolicy{scope, policy}
	resource := &api.ActionResource{Provider: "browser", ID: "work", Binding: policy.Binding, Name: "Work"}
	host := &api.ActionHost{ID: "host", MachineID: "machine", Generation: "gen", Provider: "browser", Resource: resource}
	grant := api.ActionGrant{ID: "grant", Agent: "browser-agent", State: api.GrantActive, ExpiresAt: &expiry, Permissions: []string{"browser.control"}, Target: api.ActionTarget{Kind: "machine", ID: "machine", Resource: resource}}
	s := b.reserveBrowserControl(ctx, h, p, grant, "session1")
	s.id = "session1"
	var ids []string
	comp.closeHook = func(call context.Context, args api.BrowserControlArgs) error {
		ids = append(ids, args.String("request_id"))
		b.closeBrowserControl(s, "closed", true) // Terminal event overtakes HTTP response.
		select {
		case <-s.ctx.Done():
		case <-time.After(time.Second):
			t.Error("session did not end")
		}
		select {
		case <-call.Done():
			return call.Err()
		case <-time.After(25 * time.Millisecond):
			return nil
		}
	}
	for _, job := range []string{"close-first", "close-retry"} {
		done := b.runBrowserJob(ctx, c, host, api.ActionHostJob{Job: api.ActionJob{ID: job, Action: "browser.page.close", Version: 1}, Args: json.RawMessage(`{"session":"session1"}`), Grant: grant}, h, p)
		if done.State != api.JobSucceeded || !strings.Contains(string(done.Result), `"steps"`) {
			t.Fatalf("close lost retained draft: %+v", done)
		}
	}
	if len(ids) != 2 || ids[0] != "close-first" || ids[1] != ids[0] {
		t.Fatalf("close retry lost original request ID: %v", ids)
	}
	stopped := b.reserveBrowserControl(ctx, h, p, grant, "stopped")
	stopped.id = "stopped"
	b.closeBrowserControl(stopped, api.BrowserControlOwnerStopped, true)
	done := b.runBrowserJob(ctx, c, host, api.ActionHostJob{Job: api.ActionJob{ID: "stop-retry", Action: "browser.page.close", Version: 1}, Args: json.RawMessage(`{"session":"stopped"}`), Grant: grant}, h, p)
	if done.Error == nil || done.Error.Reason != api.BrowserControlOwnerStopped || len(ids) != 2 {
		t.Fatalf("forced stop exported draft: %+v", done)
	}
}
