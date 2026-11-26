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

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/browserpolicy"
	"github.com/myco-ai/myco/internal/client"
)

type profilesFixtureComponent struct {
	policyFixtureComponent
	mu               sync.Mutex
	profiles         []api.BrowserProfile
	creates, deletes int
	legacy           bool
}

func (f *profilesFixtureComponent) Call(_ context.Context, _ string, method, path string, in, out any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	var value any
	switch {
	case method == "GET" && path == "/status":
		features := []string{"profile_delete"}
		if f.legacy {
			features = nil
		}
		value = api.BrowserStatus{Protocol: 1, Features: features, Profiles: f.profiles}
	case method == "POST" && path == "/profiles":
		// Model an older component which would create on every POST.
		f.creates++
		p := api.BrowserProfile{ID: fmt.Sprintf("p_demo%d", f.creates), Name: "Dedicated agent", Kind: "agent"}
		f.profiles = append(f.profiles, p)
		value = p
	case method == "DELETE" && strings.HasPrefix(path, "/profiles/"):
		f.deletes++
		value = true
	default:
		return fmt.Errorf("unexpected %s %s", method, path)
	}
	if out == nil {
		return nil
	}
	raw, _ := json.Marshal(value)
	return json.Unmarshal(raw, out)
}

func TestBrowserDedicatedProfileResolutionReusesOlderComponentIdentity(t *testing.T) {
	b := &Backend{}
	native := &profilesFixtureComponent{}
	var wg sync.WaitGroup
	for range 24 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, _, err := b.resolveBrowserAgentProfile(context.Background(), native)
			if err != nil || p.ID != "p_demo1" {
				t.Error(p, err)
			}
		}()
	}
	wg.Wait()
	if native.creates != 1 {
		t.Fatal("retry created another native profile", native.creates)
	}
	native.profiles = nil
	if _, _, err := b.resolveBrowserAgentProfile(context.Background(), native, false); err == nil || native.creates != 1 {
		t.Fatal("owner deletion ignored", err)
	}
	native.profiles = []api.BrowserProfile{{ID: "p_demo1", Kind: "agent"}}
	native.profiles = append(native.profiles, api.BrowserProfile{ID: "p_legacy", Kind: "agent"})
	p, ids, err := b.resolveBrowserAgentProfile(context.Background(), native)
	if err != nil || p.ID != "p_demo1" || len(ids) != 2 || native.creates != 1 {
		t.Fatal(p, ids, err)
	}
}

func TestBrowserProfileDeletionRequiresExactOwnerConfirmationAndIdle(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/v1/console/me"):
			json.NewEncoder(w).Encode(api.Whoami{Member: &api.Member{ID: "owner"}, Device: &api.ConsoleToken{}})
		case strings.Contains(r.URL.Path, "/v1/pages/"):
			json.NewEncoder(w).Encode(api.Page{ID: filepath.Base(r.URL.Path), Name: "Browser"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer hub.Close()
	store := NewStore(filepath.Join(t.TempDir(), "hubs.json"))
	h, err := store.Add("Fixture", hub.URL)
	if err != nil {
		t.Fatal(err)
	}
	b := &Backend{Store: store, Secrets: &policyFixtureSecrets{token: "fixture-token"}}
	native := &profilesFixtureComponent{profiles: []api.BrowserProfile{{ID: "p_demo", Name: "Demo"}}}
	b.browser.component = native
	grants := b.grants()
	grants.mu.Lock()
	grants.perm(h.ID, "pg_test").once["browser"] = true
	grants.mu.Unlock()
	c, _ := client.New(h.URL, "fixture-token")
	policy, err := b.browserPolicies()
	if err != nil {
		t.Fatal(err)
	}
	scope := browserpolicy.Scope{Workspace: h.URL, Owner: "owner"}
	if _, err = policy.SetEnabled(scope, "p_demo", true); err != nil {
		t.Fatal(err)
	}
	call := func(id, nonce string) error {
		raw, _ := json.Marshal(api.BrowserProfileRemoveInput{ID: id})
		_, err := b.browserCall(context.Background(), c, h, browserCall{Page: "pg_test", Instance: "frame", Action: "profiles.remove", Args: raw, Confirmation: nonce})
		return err
	}
	mux := http.NewServeMux()
	b.browserRoutes(mux)
	confirm := func(trusted bool) string {
		raw, _ := json.Marshal(browserCall{Page: "pg_test", Instance: "frame", Action: "profiles.remove", Args: json.RawMessage(`{"id":"p_demo"}`)})
		r := httptest.NewRequest("POST", "http://app/app/hubs/"+h.ID+"/browser/confirm", strings.NewReader(string(raw)))
		if trusted {
			r.Header.Set("X-Myco-Browser-Owner", "1")
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if !trusted {
			if w.Code != 403 {
				t.Fatal(w.Code)
			}
			return ""
		}
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var out struct{ Confirmation string }
		json.Unmarshal(w.Body.Bytes(), &out)
		return out.Confirmation
	}
	confirm(false)
	if call("p_demo", "") == nil {
		t.Fatal("unconfirmed deletion")
	}
	nonce := confirm(true)
	if call("p_other", nonce) == nil || call("p_demo", nonce) == nil {
		t.Fatal("substituted or reused nonce")
	}
	native.legacy = true
	if call("p_demo", confirm(true)) == nil {
		t.Fatal("old component deletion")
	}
	native.legacy = false
	book := b.browserRuns()
	book.mu.Lock()
	book.controls = map[string]*browserControlSession{"live": {profile: "p_demo"}}
	book.mu.Unlock()
	if call("p_demo", confirm(true)) == nil {
		t.Fatal("deleted controlled profile")
	}
	book.mu.Lock()
	delete(book.controls, "live")
	book.mu.Unlock()
	if native.deletes != 0 {
		t.Fatal("rejected deletion reached component")
	}
	if err = call("p_demo", confirm(true)); err != nil {
		t.Fatal(err)
	}
	if native.deletes != 1 || !b.browserAgentsBlocked(h.ID, "p_demo") {
		t.Fatal("deleted profile still advertised")
	}
	rows, err := policy.Profiles(scope)
	if err != nil || len(rows) != 0 {
		t.Fatal("deleted local policy retained", rows, err)
	}
}
