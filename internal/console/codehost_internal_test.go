package console

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestForgeHostConfirmationAndPageBoundary(t *testing.T) {
	sec := mapSecrets{"workspace": "viewer-credential"}
	b := &Backend{Local: &Local{Home: t.TempDir()}, Secrets: sec}
	h := b.Handler()
	page := osPage{Hub: "workspace", ID: "page", Name: "Code hosts"}
	p := b.grants().perm(page.Hub, page.ID)
	p.once["codehost"] = true
	request := func(path string, value any, owner bool) (int, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(value)
		req := httptest.NewRequest("POST", path, strings.NewReader(string(raw)))
		req.Header.Set("Content-Type", "application/json")
		if owner {
			req.Header.Set("X-Myco-Codehost-Owner", "1")
		}
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		var out map[string]any
		_ = json.Unmarshal(res.Body.Bytes(), &out)
		return res.Code, out
	}
	input := codeHostInput{Page: page, Provider: "gitlab", Host: "https://127.0.0.1", Source: "token"}
	if status, _ := request("/app/codehost/prepare", input, false); status != http.StatusForbidden {
		t.Fatal("missing host marker allowed", status)
	}
	status, out := request("/app/codehost/prepare", input, true)
	if status != 200 || out["private_network"] != true {
		t.Fatal("private endpoint not described", status, out)
	}
	confirmation := out["confirmation"]
	status, _ = request("/app/codehost/connect", map[string]any{"confirmation": confirmation, "token": "fixture-provider-token"}, true)
	if status != 403 {
		t.Fatal("private network consent bypassed", status)
	}
	status, _ = request("/app/codehost/connect", map[string]any{"confirmation": confirmation, "token": "fixture-provider-token", "allow_private": true}, true)
	if status != 403 {
		t.Fatal("confirmation reused", status)
	}
	_, out = request("/app/codehost/prepare", input, true)
	sec["workspace"] = "different-viewer-credential"
	status, _ = request("/app/codehost/connect", map[string]any{"confirmation": out["confirmation"], "token": "fixture-provider-token", "allow_private": true}, true)
	if status != 403 {
		t.Fatal("confirmation crossed viewer identity", status)
	}
	status, _ = request("/app/os/call", map[string]any{"page": page, "name": "forge.accounts", "args": map[string]any{"action": "connect", "provider": "github", "source": "token", "token": "fixture-provider-token"}}, false)
	if status != 403 {
		t.Fatal("page passed token through generic bridge", status)
	}
	status, out = request("/app/os/call", map[string]any{"page": page, "name": "forge.accounts", "args": map[string]any{"action": "list"}}, false)
	if status != 200 {
		t.Fatal(status, out)
	}
	if len(sec) != 1 {
		t.Fatal("unconfirmed credential was saved")
	}
	// First permission also names the exact requested provider and server.
	delete(p.once, "codehost")
	status, out = request("/app/codehost/prepare", input, true)
	if status != 403 || out["host"] != "gitlab · https://127.0.0.1" {
		t.Fatal("consent omitted destination", status, out)
	}
	// No raw token in the capability audit file.
	files, _ := filepath.Glob(filepath.Join(b.Local.Home, ".myco", "logs", "*"))
	for _, file := range files {
		raw, _ := os.ReadFile(file)
		if strings.Contains(string(raw), "fixture-provider-token") {
			t.Fatal("audit log contained a token")
		}
	}
}
