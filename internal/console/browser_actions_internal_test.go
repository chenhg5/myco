package console

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/browserautomation"
	"github.com/myco-ai/myco/internal/browserpolicy"
	"github.com/myco-ai/myco/internal/client"
)

type actionBrowserFixture struct {
	startError       error
	resumePending    bool
	resumeHeartbeats int
	next             int

	policyFixtureComponent
	mu                             sync.Mutex
	state                          string
	starts, cancels, closed, shots int
	script                         json.RawMessage
	wait                           bool
}

func (f *actionBrowserFixture) Call(ctx context.Context, scope, method, path string, in, out any) error {
	f.mu.Lock()
	var value any
	switch {
	case method == "POST" && path == "/runs":
		if f.startError != nil {
			f.mu.Unlock()
			return f.startError
		}
		if !strings.HasPrefix(scope, "agent-") {
			f.mu.Unlock()
			return fmt.Errorf("human scope reused")
		}
		raw, _ := json.Marshal(in)
		var req struct {
			Script   json.RawMessage   `json:"script"`
			ScriptID string            `json:"script_id"`
			Version  int               `json:"script_version"`
			Profile  string            `json:"profile"`
			Params   map[string]string `json:"params"`
			Headless bool              `json:"headless"`
		}
		if browserDecode(raw, &req) != nil {
			f.mu.Unlock()
			return fmt.Errorf("wrong component protocol")
		}
		f.script = req.Script
		f.starts++
		f.state = "paused"
		f.next = 2
		value = map[string]string{"id": "run_fixture"}
	case method == "GET" && strings.HasPrefix(path, "/runs/run_fixture?"):
		if f.wait {
			f.mu.Unlock()
			<-ctx.Done()
			return ctx.Err()
		}
		if f.resumePending && strings.Contains(path, "after=3") && f.resumeHeartbeats == 0 {
			f.resumePending = false
			f.state = "succeeded"
			f.next = 4
		}
		if f.resumePending && strings.Contains(path, "after=3") && f.resumeHeartbeats > 0 {
			f.resumeHeartbeats--
		}
		value = map[string]any{"state": f.state, "node": "confirm", "question": "Publish?", "next": f.next, "output": map[string]string{"title": "Report"}, "steps": []map[string]string{{"shot": "a_fixture"}}}
	case method == "POST" && path == "/runs/run_fixture/resume":
		f.state = "paused"
		f.next = 3
		f.resumePending = true
		f.resumeHeartbeats = 1
		raw, _ := json.Marshal(in)
		var input struct {
			Choice string `json:"choice"`
		}
		json.Unmarshal(raw, &input)
		if input.Choice == "cancel" {
			f.state = "cancelled"
			f.resumePending = false
		}
		value = true
	case method == "POST" && path == "/runs/run_fixture/cancel":
		f.cancels++
		f.state = "cancelled"
		value = true
	case method == "DELETE" && path == "/sessions":
		f.closed++
		value = true
	default:
		f.mu.Unlock()
		return fmt.Errorf("unexpected native call: %s %s", method, path)
	}
	f.mu.Unlock()
	raw, _ := json.Marshal(value)
	return json.Unmarshal(raw, out)
}
func (f *actionBrowserFixture) Artifact(_ context.Context, scope, id string) ([]byte, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !strings.HasPrefix(scope, "agent-") || id != "a_fixture" {
		return nil, "", fmt.Errorf("wrong artifact ownership")
	}
	f.shots++
	return []byte("private-test-image"), "image/png", nil
}

func TestBrowserHostUsesApprovedSnapshotAndOriginalGrant(t *testing.T) {
	var hub *httptest.Server
	expiry := time.Now().Add(time.Minute)
	hub = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/artifacts") {
			json.NewEncoder(w).Encode(api.ActionArtifact{ID: "art_fixture", MediaType: "image/png", Bytes: 18, ExpiresAt: expiry})
			return
		}
		// The generic job watcher is a real client HTTP call, including long polls.
		if r.URL.Query().Get("wait") != "" && r.URL.Query().Get("wait") != "0" {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(10 * time.Millisecond):
			}
		}
		json.NewEncoder(w).Encode(api.ActionHostJobState{GrantState: api.GrantActive, GrantExpiresAt: &expiry})
	}))
	defer hub.Close()
	h := Hub{ID: "hub", URL: hub.URL}
	secrets := &policyFixtureSecrets{token: "fixture"}
	store := NewStore(filepath.Join(t.TempDir(), "hubs.json"))
	b := &Backend{Store: store, Secrets: secrets}
	comp := &actionBrowserFixture{}
	b.browser.component = comp
	c, _ := client.New(h.URL, "fixture")
	scope := browserpolicy.Scope{Workspace: h.URL, Owner: "owner", Page: "pg_test"}
	b.browser.identities = map[string]browserOwnerIdentity{h.ID: {URL: h.URL, Owner: scope.Owner, Credential: c.CredentialFingerprint()}}
	policies, _ := b.browserPolicies()
	script := json.RawMessage(`{"name":"Report","params":[{"name":"month","type":"text"},{"name":"password","type":"secret"}],"graph":{"nodes":[{"id":"confirm","type":"confirm"}],"edges":[]}}`)
	req := browserpolicy.Request{Profile: "work", ScriptID: "report", Version: 3, Script: script}
	if _, _, err := policies.ApproveConfirmed(scope, req, time.Now()); err != nil {
		t.Fatal(err)
	}
	policy, err := policies.SetEnabled(scope, "work", true, "Work")
	if err != nil {
		t.Fatal(err)
	}
	scope.Page = ""
	p := browserHostPolicy{scope, policy}
	resource := &api.ActionResource{Provider: "browser", ID: "work", Binding: policy.Binding, Name: "Work"}
	host := &api.ActionHost{ID: "host", MachineID: "machine", Generation: "gen", Provider: "browser", Resource: resource}
	grant := api.ActionGrant{ID: "grant", Agent: "agent", State: api.GrantActive, ExpiresAt: &expiry, Permissions: []string{"browser.view", "browser.run"}, Target: api.ActionTarget{Kind: "machine", ID: "machine", Resource: resource}}
	call := func(action, args string, g api.ActionGrant) api.ActionJobComplete {
		return b.runBrowserJob(context.Background(), c, host, api.ActionHostJob{Job: api.ActionJob{ID: "job", Action: action, Version: 1}, Args: json.RawMessage(args), Grant: g}, h, p)
	}
	for _, args := range []string{`{"script_id":"report","version":3,"start_at":"confirm"}`, `{"script_id":"report","version":3,"params":{"password":"secret"}}`, `{"script_id":"report","version":3,"params":{"unknown":"value"}}`} {
		done := call("browser.script.run", args, grant)
		if done.Error == nil || done.Error.Code != "invalid_argument" {
			t.Fatalf("injected args accepted: %+v", done)
		}
	}
	denied := grant
	denied.Permissions = []string{"browser.view"}
	if done := call("browser.script.run", `{"script_id":"report","version":3}`, denied); done.Error == nil || done.Error.Code != "permission_denied" {
		t.Fatal("view escalated to run")
	}
	comp.startError = browserautomation.ErrUnsupportedNode
	if rejected := call("browser.script.run", `{"script_id":"report","version":3,"page_id":"pg_test"}`, grant); rejected.Error == nil || rejected.Error.Code != "unsupported_node" {
		t.Fatalf("preflight refusal lost: %+v", rejected)
	}
	comp.startError = nil
	done := call("browser.script.run", `{"script_id":"report","version":3,"page_id":"pg_test","params":{"month":"11"}}`, grant)
	if done.State != api.JobSucceeded || !strings.Contains(string(done.Result), `"state":"paused"`) || len(done.ArtifactIDs) != 1 {
		t.Fatalf("paused job result: %+v", done)
	}
	comp.mu.Lock()
	if !jsonEqual(comp.script, script) || comp.starts != 1 || comp.shots != 1 {
		t.Fatalf("mutable snapshot or missing private screenshot: %s", comp.script)
	}
	comp.mu.Unlock()
	other := grant
	other.ID = "other-grant"
	if done := call("browser.run.resume", `{"run_id":"run_fixture"}`, other); done.Error == nil || done.Error.Code != "permission_denied" {
		t.Fatal("paused run moved grants")
	}
	other = grant
	other.Agent = "other-agent"
	if done := call("browser.run.status", `{"run_id":"run_fixture"}`, other); done.Error == nil || done.Error.Code != "permission_denied" {
		t.Fatal("another agent read run")
	}
	done = call("browser.run.resume", `{"run_id":"run_fixture","choice":"ok"}`, grant)
	if done.State != api.JobSucceeded || !strings.Contains(string(done.Result), `"state":"succeeded"`) {
		t.Fatalf("resume: %+v", done)
	}
	// Cancelling a confirmation has no control ACK event in protocol 1.
	done = call("browser.script.run", `{"script_id":"report","version":3,"page_id":"pg_test"}`, grant)
	if done.State != api.JobSucceeded {
		t.Fatalf("new paused run: %+v", done)
	}
	done = call("browser.run.resume", `{"run_id":"run_fixture","choice":"cancel"}`, grant)
	if done.State != api.JobCancelled || !strings.Contains(string(done.Result), `"state":"cancelled"`) {
		t.Fatalf("cancel choice: %+v", done)
	}
	// A policy edit locally cancels paused native work before old grants can
	// resume. The callback is installed when the browser host book is initialized.
	done = call("browser.script.run", `{"script_id":"report","version":3,"page_id":"pg_test"}`, grant)
	if done.State != api.JobSucceeded || !strings.Contains(string(done.Result), `"state":"paused"`) {
		t.Fatalf("second paused run: %+v", done)
	}
	b.blockBrowserAgents(h.ID, "work")
	comp.mu.Lock()
	if comp.cancels != 1 || comp.closed < 2 {
		t.Fatal("opt-out did not cancel and release native session")
	}
	comp.mu.Unlock()
	if done := call("browser.run.resume", `{"run_id":"run_fixture"}`, grant); done.Error == nil || done.Error.Code != "permission_denied" {
		t.Fatal("blocked profile resumed")
	}
}
func TestBrowserPausedRunExpiresLocally(t *testing.T) {
	b := &Backend{}
	comp := &actionBrowserFixture{}
	b.browser.component = comp
	r := &browserAgentRun{id: "run_fixture", scope: "agent-fixture", expires: time.Now().Add(25 * time.Millisecond), update: browserNativeUpdate{State: "paused"}}
	r.timer = time.AfterFunc(time.Until(r.expires), func() { b.cancelBrowserRun(r) })
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		closed := r.closed
		r.mu.Unlock()
		if closed {
			comp.mu.Lock()
			n := comp.cancels
			comp.mu.Unlock()
			if n > 0 {
				return
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("paused browser survived grant expiry")
}

func jsonEqual(a, b []byte) bool {
	var x, y any
	json.Unmarshal(a, &x)
	json.Unmarshal(b, &y)
	xa, _ := json.Marshal(x)
	ya, _ := json.Marshal(y)
	return string(xa) == string(ya)
}

func TestBrowserRunningCancellationStopsNativeSession(t *testing.T) {
	b := &Backend{}
	comp := &actionBrowserFixture{wait: true}
	b.browser.component = comp
	r := &browserAgentRun{id: "run_fixture", scope: "agent-fixture", expires: time.Now().Add(time.Minute), update: browserNativeUpdate{State: "running"}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan api.ActionJobComplete, 1)
	go func() { done <- b.waitBrowserRun(ctx, nil, nil, "job", r, 0) }()
	cancel()
	select {
	case result := <-done:
		if result.State != api.JobCancelled || result.Error.Code != "outcome_unknown" {
			t.Fatalf("lost side effect warning: %+v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("native cancellation stuck")
	}
	comp.mu.Lock()
	defer comp.mu.Unlock()
	if comp.cancels != 1 || comp.closed != 1 {
		t.Fatal("HTTP cancellation left browser operating")
	}
}
