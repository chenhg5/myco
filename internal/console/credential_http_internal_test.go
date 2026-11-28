package console

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/oscap"
)

// Exercise the actual desktop HTTP bridge, vault and TLS request path together.
// Only the external service and owner's input are fixtures.
func TestCredentialHTTPBridgeLifecycle(t *testing.T) {
	const secret = "fixture-local-credential"
	var requests atomic.Int64
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != "GET" || r.Header.Get("Authorization") != "Bearer "+secret || r.URL.Path != "/data/private" {
			t.Error("incorrect authenticated request")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Set-Cookie", "never-forward-this")
		_, _ = w.Write([]byte(`{"fixture":"ok"}`))
	}))
	defer provider.Close()
	// Trust only this test service's CA. Production TLS verification remains enabled.
	previousTransport := http.DefaultTransport
	http.DefaultTransport = provider.Client().Transport.(*http.Transport).Clone()
	t.Cleanup(func() { http.DefaultTransport = previousTransport })
	previousDiscover := credentialDiscoverSources
	credentialDiscoverSources = func(context.Context, *oscap.Env) []api.CredentialSource { return nil }
	t.Cleanup(func() { credentialDiscoverSources = previousDiscover })
	home := t.TempDir()
	secrets := mapSecrets{"workspace": "viewer-fixture"}
	page := osPage{Hub: "workspace", ID: "page"}
	origin, _ := url.Parse(provider.URL)
	makeBackend := func() *Backend {
		b := &Backend{Local: &Local{Home: home}, Secrets: secrets}
		g := b.grants()
		g.decl[osKey(page.Hub, page.ID)] = osDecl{known: true, app: true, appID: "fixture-app", declares: true, os: []string{"credentials", "network"}, net: []string{origin.Host}, at: time.Now()}
		p := g.perm(page.Hub, page.ID)
		p.once["credentials"], p.once["network"] = true, true
		return b
	}
	b := makeBackend()
	call := func(b *Backend, path string, payload any, owner bool) (int, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(payload)
		r := httptest.NewRequest("POST", path, strings.NewReader(string(raw)))
		r.Header.Set("Content-Type", "application/json")
		if owner {
			r.Header.Set("X-Myco-Credential-Owner", "1")
		}
		w := httptest.NewRecorder()
		b.Handler().ServeHTTP(w, r)
		var out map[string]any
		if json.Unmarshal(w.Body.Bytes(), &out) != nil {
			t.Fatal("non-JSON response")
		}
		return w.Code, out
	}
	descriptor := api.CredentialDescriptor{Name: "Fixture", Origin: provider.URL, Authentication: "bearer", Paths: []api.CredentialPath{{Path: "/data/", Prefix: true}}}
	status, prepared := call(b, "/app/credentials/prepare", credentialInput{Page: page, Descriptor: descriptor}, true)
	if status != 200 {
		t.Fatal("prepare", status, prepared)
	}
	status, connection := call(b, "/app/credentials/connect", map[string]any{"confirmation": prepared["confirmation"], "token": secret, "allow_private": true}, true)
	if status != 200 {
		t.Fatal("connect", status, connection)
	}
	id := connection["id"].(string)
	payload := map[string]any{"page": page, "name": "net.fetch", "args": map[string]any{"credential": id, "path": "/data/private", "query": map[string]string{"search": "private-search"}}}
	status, result := call(b, "/app/os/call", payload, false)
	if status != 200 {
		t.Fatal("read", status, result)
	}
	value := result["value"].(map[string]any)
	if value["body"] != `{"fixture":"ok"}` {
		t.Fatal(value)
	}
	serialized, _ := json.Marshal(result)
	if strings.Contains(string(serialized), secret) || strings.Contains(string(serialized), "never-forward-this") {
		t.Fatal("credential/cookie exposed")
	}
	fresh := makeBackend()
	if status, _ := call(fresh, "/app/os/call", payload, false); status != 200 {
		t.Fatal("cold start lost saved connection", status)
	}
	secrets["workspace"] = "other-viewer"
	if status, _ := call(fresh, "/app/os/call", payload, false); status != 404 {
		t.Fatal("handle crossed workspace sign-in", status)
	}
	secrets["workspace"] = "viewer-fixture"
	delete(fresh.grants().perm(page.Hub, page.ID).once, "network")
	if status, _ := call(fresh, "/app/os/call", payload, false); status != 403 {
		t.Fatal("network grant bypassed", status)
	}
	status, _ = call(fresh, "/app/os/call", map[string]any{"page": page, "name": "credentials.connections", "args": map[string]string{"action": "remove", "id": id}}, false)
	if status != 200 {
		t.Fatal("remove", status)
	}
	if status, _ := call(b, "/app/os/call", payload, false); status != 404 {
		t.Fatal("revoked handle remained usable", status)
	}
	if requests.Load() != 2 {
		t.Fatal("rejected calls reached service", requests.Load())
	}
	audit, err := os.ReadFile(filepath.Join(home, ".myco", "logs", "app-credentials.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{secret, "private-search", "/data/private", "never-forward-this"} {
		if strings.Contains(string(audit), private) {
			t.Fatal("private request data in audit")
		}
	}
	if !strings.Contains(string(audit), "/data/*") {
		t.Fatal("audit omitted grant template")
	}
}
