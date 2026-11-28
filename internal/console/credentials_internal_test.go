package console

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/oscap"
)

func TestCredentialHostConfirmationAndIdentityBoundary(t *testing.T) {
	old := credentialDiscoverSources
	t.Cleanup(func() { credentialDiscoverSources = old })
	credentialDiscoverSources = func(context.Context, *oscap.Env) []api.CredentialSource {
		return []api.CredentialSource{{ID: "discovered", CLI: "gh", Host: "github.com", Login: "demo", Origin: "https://api.github.com"}}
	}
	sec := mapSecrets{"workspace": "viewer-credential"}
	b := &Backend{Local: &Local{Home: t.TempDir()}, Secrets: sec}
	h := b.Handler()
	page := osPage{Hub: "workspace", ID: "page"}
	g := b.grants()
	g.decl[osKey(page.Hub, page.ID)] = osDecl{known: true, app: true, appID: "app", at: time.Now()}
	g.perm(page.Hub, page.ID).once["credentials"] = true
	request := func(path string, value any, owner bool) (int, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(value)
		req := httptest.NewRequest("POST", path, strings.NewReader(string(raw)))
		req.Header.Set("Content-Type", "application/json")
		if owner {
			req.Header.Set("X-Myco-Credential-Owner", "1")
		}
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		var out map[string]any
		_ = json.Unmarshal(res.Body.Bytes(), &out)
		return res.Code, out
	}
	input := credentialInput{Page: page, Descriptor: api.CredentialDescriptor{Name: "Fixture", Origin: "https://127.0.0.1", Authentication: "bearer", Paths: []api.CredentialPath{{Path: "/user"}}}}
	if status, _ := request("/app/credentials/prepare", input, false); status != 403 {
		t.Fatal("missing owner marker accepted", status)
	}
	status, out := request("/app/credentials/prepare", input, true)
	if status != 200 || out["private_network"] != true {
		t.Fatal(status, out)
	}
	if len(out["sources"].([]any)) != 0 {
		t.Fatal("gh token offered to a different origin")
	}
	confirmation := out["confirmation"]
	if status, _ := request("/app/credentials/connect", api.CredentialConnect{Confirmation: confirmation.(string), Token: "fixture-secret"}, true); status != 403 {
		t.Fatal("private consent bypassed", status)
	}
	if status, _ := request("/app/credentials/connect", api.CredentialConnect{Confirmation: confirmation.(string), Token: "fixture-secret", AllowPrivate: true}, true); status != 403 {
		t.Fatal("confirmation reused", status)
	}
	_, out = request("/app/credentials/prepare", input, true)
	sec["workspace"] = "other-identity"
	if status, _ := request("/app/credentials/connect", map[string]any{"confirmation": out["confirmation"], "token": "fixture-secret", "allow_private": true}, true); status != 403 {
		t.Fatal("crossed login identity", status)
	}
	_, out = request("/app/credentials/prepare", input, true)
	if status, _ := request("/app/credentials/connect", map[string]any{"confirmation": out["confirmation"], "source": "discovered", "allow_private": true}, true); status != 403 {
		t.Fatal("injected source accepted", status)
	}
	_, out = request("/app/credentials/prepare", input, true)
	if status, _ := request("/app/credentials/connect", map[string]any{"confirmation": out["confirmation"], "token": "fixture-secret", "allow_private": true}, true); status != 200 {
		t.Fatal("confirmed manual connection failed", status)
	}
	if status, _ := request("/app/os/call", map[string]any{"page": page, "name": "credentials.connections", "args": map[string]string{"action": "connect", "token": "fixture-secret"}}, false); status != 403 {
		t.Fatal("iframe supplied token", status)
	}
	if status, _ := request("/app/os/call", map[string]any{"page": page, "name": "forge.accounts", "args": map[string]string{"action": "list"}}, false); status != http.StatusNotImplemented {
		t.Fatal("old bridge remained available", status)
	}
}
