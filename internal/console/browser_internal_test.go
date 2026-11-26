package console

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/browserautomation"
	"github.com/myco-ai/myco/internal/client"
)

func TestBrowserConfirmationIsOneShotAndFullyBound(t *testing.T) {
	b := &Backend{}
	for _, test := range []struct {
		scope, action, profile string
		ok                     bool
	}{{"hub/page/one", "install", "", true}, {"other/page/one", "install", "", false}, {"hub/page/one", "profiles.allowAgents", "p_a", false}} {
		token := b.confirmBrowser("hub/page/one", "install", "")
		if got := b.consumeBrowserConfirmation(token, test.scope, test.action, test.profile); got != test.ok {
			t.Fatal("confirmation binding")
		}
		if b.consumeBrowserConfirmation(token, "hub/page/one", "install", "") {
			t.Fatal("confirmation reused")
		}
	}
	token := b.confirmBrowser("hub/page/one", "profiles.allowAgents", "p_a")
	if b.consumeBrowserConfirmation(token, "hub/page/one", "profiles.allowAgents", "p_b") {
		t.Fatal("another profile approved")
	}
	token = b.confirmBrowser("hub/page/one", "install", "")
	b.browser.confirmations[token] = browserConfirmation{Scope: "hub/page/one", Action: "install", Expires: time.Now().Add(-time.Second)}
	if b.consumeBrowserConfirmation(token, "hub/page/one", "install", "") {
		t.Fatal("expired confirmation")
	}
	if b.consumeBrowserConfirmation("true", "hub/page/one", "install", "") {
		t.Fatal("page boolean used as approval")
	}
}
func TestBrowserStatusAbsentHasNoSideEffects(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/pages/pg_test" {
			t.Errorf("unexpected Hub request %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(api.Page{ID: "pg_test", Name: "Fixture"})
	}))
	defer hub.Close()
	home := t.TempDir()
	b := &Backend{Local: &Local{Home: home}}
	c, _ := client.New(hub.URL, "fixture")
	out, err := b.browserCall(context.Background(), c, Hub{ID: "hub"}, browserCall{Page: "pg_test", Instance: "frame-a", Action: "status"})
	if err != nil {
		t.Fatal(err)
	}
	status, ok := out.(api.BrowserStatus)
	if !ok || status.Installed || status.Version != "" || status.Profiles == nil || status.Browsers == nil || status.Protocol != 1 {
		t.Fatalf("absent contract: %+v", out)
	}
	if _, err = os.Stat(filepath.Join(home, ".myco")); !os.IsNotExist(err) {
		t.Fatal("status installed or wrote private data")
	}
}
func TestBrowserScopesSeparateInstancesAndWorkspaces(t *testing.T) {
	a := browserScope("hub-a", "pg_a", "frame-a")
	for _, b := range []string{browserScope("hub-a", "pg_a", "frame-b"), browserScope("hub-b", "pg_a", "frame-a"), browserScope("hub-a", "pg_b", "frame-a")} {
		if a == b {
			t.Fatal("scope collision")
		}
	}
	if validBrowserCall(browserCall{Page: "pg_a", Instance: "../other"}) {
		t.Fatal("scope separator in instance accepted")
	}
}
func TestBrowserFeatureAdvertisesInstallerOnSupportedDesktop(t *testing.T) {
	b := &Backend{}
	if strings.Contains(b.browserPageScript(), `.concat(["browser"])`) {
		t.Fatal("web-only console advertised browser")
	}
	home := t.TempDir()
	b.Local = &Local{Home: home}
	b.browser.component = &browserautomation.Component{Home: home}
	script := b.browserPageScript()
	if b.browserComponent().Status().Supported && !strings.Contains(script, `.concat(["browser"])`) {
		t.Fatal("supported desktop cannot discover installer")
	}

}

// The browser is its own group, and may be allowed always (taken back in
// Settings → App permissions).
func TestBrowserGroupIsSeparate(t *testing.T) {
	if osGroupOf("browser.call") != "browser" || osOnceOnly["browser"] {
		t.Fatal("browser capability consent")
	}
}

type browserFixtureSecrets struct{}

func (browserFixtureSecrets) Get(string) (string, error) { return "fixture-console-token", nil }
func (browserFixtureSecrets) Set(string, string) error   { return nil }
func (browserFixtureSecrets) Delete(string) error        { return nil }

func TestBrowserConfirmationRouteRequiresHostHeaderAndLocalDevice(t *testing.T) {
	var owner atomic.Bool
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/pages/pg_test":
			json.NewEncoder(w).Encode(api.Page{ID: "pg_test", Name: "Fixture"})
		case "/v1/console/me":
			who := api.Whoami{}
			if owner.Load() {
				who.Device = &api.ConsoleToken{}
			}
			json.NewEncoder(w).Encode(who)
		default:
			json.NewEncoder(w).Encode(map[string]any{})
		}
	}))
	defer hub.Close()
	home := t.TempDir()
	store := NewStore(filepath.Join(home, "hubs.json"))
	saved, err := store.Add("Fixture", hub.URL)
	if err != nil {
		t.Fatal(err)
	}
	b := &Backend{Local: &Local{Home: home}, Store: store, Secrets: browserFixtureSecrets{}}
	mux := http.NewServeMux()
	b.browserRoutes(mux)
	for _, tc := range []struct {
		device, header bool
		want           int
	}{{true, false, 403}, {false, true, 403}, {true, true, 200}} {
		owner.Store(tc.device)
		req := httptest.NewRequest("POST", "http://app/app/hubs/"+saved.ID+"/browser/confirm", strings.NewReader(`{"page":"pg_test","instance":"frame","action":"install"}`))
		if tc.header {
			req.Header.Set("X-Myco-Browser-Owner", "1")
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Fatalf("device=%v header=%v: %d %s", tc.device, tc.header, w.Code, w.Body.String())
		}
		if w.Code == 200 {
			var result struct {
				Confirmation string `json:"confirmation"`
			}
			json.Unmarshal(w.Body.Bytes(), &result)
			if !b.consumeBrowserConfirmation(result.Confirmation, browserScope(saved.ID, "pg_test", "frame"), "install", "") {
				t.Fatal("host nonce not bound to page")
			}
		}
	}
	// A page's raw request cannot use a truthy string as owner approval.
	req := httptest.NewRequest("POST", "http://app/app/hubs/"+saved.ID+"/browser", strings.NewReader(`{"page":"pg_test","instance":"frame","action":"install","confirmation":"true"}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("forged install confirmation: %d %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(home, ".myco", "components")); !os.IsNotExist(err) {
		t.Fatal("unapproved install wrote component files")
	}
}
