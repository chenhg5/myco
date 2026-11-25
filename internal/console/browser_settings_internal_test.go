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
	"github.com/myco-ai/myco/internal/client"
)

type settingsFixtureComponent struct {
	policyFixtureComponent
	writes int
}

func (f *settingsFixtureComponent) Call(ctx context.Context, scope, method, path string, in, out any) error {
	if method != "PATCH" {
		return f.policyFixtureComponent.Call(ctx, scope, method, path, in, out)
	}
	if path != "/profiles/profile_work" {
		return fmt.Errorf("wrong profile")
	}
	raw, _ := json.Marshal(in)
	var request struct {
		Settings api.BrowserProfileSettingsInput `json:"settings"`
	}
	if err := json.Unmarshal(raw, &request); err != nil {
		return err
	}
	f.writes++
	profile := api.BrowserProfile{ID: "profile_work", Name: "Work", Settings: &api.BrowserProfileSettings{UserAgent: request.Settings.UserAgent, Lang: request.Settings.Lang, Args: request.Settings.Args}}
	if p := request.Settings.Proxy; p != nil {
		profile.Settings.Proxy = &api.BrowserProxySettings{Server: p.Server, Bypass: p.Bypass, Username: p.Username, HasPassword: p.Password != nil && *p.Password != ""}
	}
	raw, _ = json.Marshal(profile)
	return json.Unmarshal(raw, out)
}
func TestBrowserSettingsRequireExactOwnerConfirmation(t *testing.T) {
	var agent atomic.Bool
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/v1/console/me"):
			who := api.Whoami{Member: &api.Member{ID: "owner", Name: "Owner"}, Device: &api.ConsoleToken{}}
			if agent.Load() {
				who.Member = nil
				who.Device = nil
			}
			json.NewEncoder(w).Encode(who)
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
	native := &settingsFixtureComponent{}
	b.browser.component = native
	grants := b.grants()
	grants.mu.Lock()
	grants.perm(h.ID, "pg_test").once["browser"] = true
	grants.mu.Unlock()
	c, _ := client.New(h.URL, "fixture-token")
	ctx := context.Background()
	mux := http.NewServeMux()
	b.browserRoutes(mux)
	raw := json.RawMessage(`{"id":"profile_work","settings":{"proxy":{"server":"http://proxy.example:8080","username":"fixture-user","password":"write-only-fixture","bypass":["<local>"]},"user_agent":"Mozilla/5.0","lang":"zh-CN","args":["--disable-gpu"]}}`)
	confirm := func(owner bool) string {
		t.Helper()
		body, _ := json.Marshal(browserCall{Page: "pg_test", Instance: "frame", Action: "profiles.configure", Args: raw})
		r := httptest.NewRequest("POST", "http://app/app/hubs/"+h.ID+"/browser/confirm", strings.NewReader(string(body)))
		if owner {
			r.Header.Set("X-Myco-Browser-Owner", "1")
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if !owner {
			if w.Code != 403 {
				t.Fatal("untrusted confirmation accepted")
			}
			return ""
		}
		if w.Code != 200 {
			t.Fatalf("confirm: %d %s", w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), "write-only-fixture") {
			t.Fatal("confirmation leaked password")
		}
		var result struct{ Confirmation string }
		json.Unmarshal(w.Body.Bytes(), &result)
		return result.Confirmation
	}
	call := func(input json.RawMessage, nonce string) (any, error) {
		return b.browserCall(ctx, c, h, browserCall{Page: "pg_test", Instance: "frame", Action: "profiles.configure", Args: input, Confirmation: nonce})
	}
	confirm(false)
	if _, err = call(raw, ""); err == nil || native.writes != 0 {
		t.Fatal("unconfirmed settings reached component")
	}
	nonce := confirm(true)
	changed := json.RawMessage(strings.Replace(string(raw), "Mozilla/5.0", "Changed UA", 1))
	if _, err = call(changed, nonce); err == nil || native.writes != 0 {
		t.Fatal("confirmation accepted substituted settings")
	}
	if _, err = call(raw, nonce); err == nil {
		t.Fatal("failed use did not consume nonce")
	}
	nonce = confirm(true)
	value, err := call(raw, nonce)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(value)
	if native.writes != 1 || strings.Contains(string(encoded), "write-only-fixture") || !strings.Contains(string(encoded), `"has_password":true`) {
		t.Fatalf("settings response/forwarding: %s", encoded)
	}
	if _, err = call(raw, nonce); err == nil || native.writes != 1 {
		t.Fatal("confirmation reused")
	}
	nonce = confirm(true)
	agent.Store(true)
	if _, err = call(raw, nonce); err == nil || native.writes != 1 {
		t.Fatal("agent configured profile")
	}
}
