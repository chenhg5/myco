package console

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/browserautomation"
	"github.com/myco-ai/myco/internal/browserpolicy"
	"github.com/myco-ai/myco/internal/client"
)

type replayFixtureComponent struct {
	policyFixtureComponent
	features []string
	run      func(context.Context, api.BrowserControlArgs) (json.RawMessage, error)
}

func (f *replayFixtureComponent) Call(ctx context.Context, scope, method, path string, in, out any) error {
	var raw json.RawMessage
	if path == "/status" {
		raw, _ = json.Marshal(api.BrowserStatus{Protocol: 1, Features: f.features})
	} else if path == "/control/run" {
		var err error
		raw, err = f.run(ctx, in.(api.BrowserControlArgs))
		if err != nil {
			return err
		}
	} else {
		return errors.New("unexpected native call")
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}
func (f *replayFixtureComponent) Artifact(_ context.Context, scope, id string) ([]byte, string, error) {
	if scope != "agent-demo" || id != "a_fixture" {
		return nil, "", errors.New("wrong private image scope")
	}
	return []byte("private-image-fixture"), "image/png", nil
}

func TestBrowserReplayDiscoveryPreservesOlderControl(t *testing.T) {
	for _, test := range []struct {
		name      string
		features  []string
		rejectRun bool
		want      int
	}{
		{"legacy", []string{"agent_control"}, false, 6},
		{"current", []string{"agent_control", "agent_replay"}, false, 7},
		{"older-hub", []string{"agent_control", "agent_replay"}, true, 6},
	} {
		t.Run(test.name, func(t *testing.T) {
			posts := 0
			hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				posts++
				var req api.ActionHostRegister
				json.NewDecoder(r.Body).Decode(&req)
				count := 0
				hasRun := false
				for _, action := range req.Actions {
					if strings.HasPrefix(action.Name, "browser.page.") {
						count++
					}
					if action.Name == "browser.page.run" {
						hasRun = true
					}
				}
				if test.rejectRun && hasRun {
					w.WriteHeader(400)
					json.NewEncoder(w).Encode(api.Error{Code: "invalid_argument", Message: "unsupported browser.page.run"})
					return
				}
				if count != test.want || !req.BrowserHostScope {
					t.Errorf("wrong advertised actions/scope: %d %+v", count, req)
				}
				json.NewEncoder(w).Encode(api.ActionHost{ID: "host"})
			}))
			defer hub.Close()
			b := &Backend{}
			b.browser.component = &replayFixtureComponent{features: test.features}
			c, _ := client.New(hub.URL, "fixture")
			host, err := b.registerBrowserHost(context.Background(), c, "machine", browserHostPolicy{Policy: browserpolicy.Policy{Profile: "work"}})
			if err != nil || host.ID != "host" {
				t.Fatal(host, err)
			}
			wantPosts := 1
			if test.rejectRun {
				wantPosts = 2
			}
			if posts != wantPosts {
				t.Fatal(posts)
			}
		})
	}
}

func TestBrowserReplayHostLifecycleAndPrivateArtifacts(t *testing.T) {
	for _, mode := range []string{"success", "unsupported", "transport", "owner-stop"} {
		t.Run(mode, func(t *testing.T) {
			expiry := time.Now().Add(time.Minute)
			uploads := 0
			hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/artifacts") {
					uploads++
					json.NewEncoder(w).Encode(api.ActionArtifact{ID: "art_fixture", MediaType: "image/png", ExpiresAt: expiry})
					return
				}
				if r.URL.Query().Get("wait") == "30" {
					<-r.Context().Done()
					return
				}
				json.NewEncoder(w).Encode(api.ActionHostJobState{GrantState: api.GrantActive, GrantExpiresAt: &expiry})
			}))
			defer hub.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			b := &Backend{}
			comp := &replayFixtureComponent{features: []string{"agent_control", "agent_replay"}}
			b.browser.component = comp
			c, _ := client.New(hub.URL, "fixture")
			h := Hub{ID: "demo", URL: hub.URL}
			policy := browserHostPolicy{Scope: browserpolicy.Scope{Workspace: hub.URL, Owner: "owner"}, Policy: browserpolicy.Policy{Profile: "work", Binding: "binding"}}
			resource := &api.ActionResource{Provider: "browser", ID: "work", Binding: "binding"}
			host := &api.ActionHost{ID: "host", Generation: "generation", Provider: "browser", Resource: resource}
			grant := api.ActionGrant{ID: "grant", Agent: "demo-agent", ExpiresAt: &expiry, State: api.GrantActive, Target: api.ActionTarget{Kind: "machine", ID: "machine", Resource: resource}, Permissions: []string{api.BrowserControlPermission}}
			s := b.reserveBrowserControl(ctx, h, policy, grant, "session")
			if s == nil {
				t.Fatal("no session")
			}
			defer s.cancel()
			s.id = "session"
			s.scope = "agent-demo"
			s.lastActive = time.Now().Add(-6 * time.Minute)
			called := false
			comp.run = func(call context.Context, args api.BrowserControlArgs) (json.RawMessage, error) {
				called = true
				if args.String("request_id") != "job" || args.String("session") != "session" {
					t.Error("request identity changed")
				}
				deadline, ok := call.Deadline()
				if !ok || deadline.After(expiry) {
					t.Error("replay outlived its grant")
				}
				s.mu.Lock()
				active, last := s.replaying, s.lastActive
				s.mu.Unlock()
				if !active || time.Since(last) > time.Second {
					t.Error("replay did not refresh/suppress idle")
				}
				if mode == "transport" {
					return nil, errors.New("connection reset")
				}
				if mode == "owner-stop" {
					b.closeBrowserControl(s, api.BrowserControlOwnerStopped, true)
					<-call.Done()
					return nil, call.Err()
				}
				return json.RawMessage(`{"ok":true,"state":"succeeded","steps":[{"node":"image","state":"ok","ms":1}],"output":{"image":{"screenshot":{"id":"a_fixture"},"image":{"width":800,"height":600}},"title":"Public report"},"url":"https://example.com/","tabs":[]}`), nil
			}
			if mode == "unsupported" {
				comp.features = []string{"agent_control"}
			}
			input := json.RawMessage(`{"session":"session","graph":{"nodes":[{"id":"image","type":"screenshot","into":"image"}],"edges":[]}}`)
			done := b.runBrowserControlJob(ctx, c, host, api.ActionHostJob{Job: api.ActionJob{ID: "job", Action: "browser.page.run", Version: 1}, Args: input, Grant: grant}, h, policy)
			if mode == "success" {
				if done.State != api.JobSucceeded || len(done.ArtifactIDs) != 1 || uploads != 1 || strings.Contains(string(done.Result), "a_fixture") || !strings.Contains(string(done.Result), `"artifact_id":"art_fixture"`) {
					t.Fatalf("private replay output: %+v", done)
				}
			} else {
				want := "outcome_unknown"
				if mode == "unsupported" {
					want = "unsupported"
				}
				if mode == "owner-stop" {
					want = "session_closed"
				}
				if done.Error == nil || done.Error.Code != want || uploads != 0 {
					t.Fatalf("%s: %+v", mode, done)
				}
			}
			if called == (mode == "unsupported") {
				t.Fatal("component feature gate failed")
			}
			s.mu.Lock()
			active, last := s.replaying, s.lastActive
			s.mu.Unlock()
			if active || time.Since(last) > time.Second {
				t.Fatal("replay left idle suppressed")
			}
		})
	}
}

func TestBrowserReplayNativePreflightErrorNode(t *testing.T) {
	done := browserControlFailure(&browserautomation.ComponentError{Code: "invalid_argument", ErrorNode: "blocked_node"}, true)
	if done.Error == nil || done.Error.ErrorNode != "blocked_node" || done.Error.Code != "invalid_argument" {
		t.Fatal(done)
	}
}
