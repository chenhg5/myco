package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
)

func TestRuntimeAccountSources(t *testing.T) {
	home := t.TempDir()
	env := map[string]string{"CLAUDE_CODE_USER_EMAIL": "cloud@example.com"}
	get := func(k string) string { return env[k] }
	write := func(path, data string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(home, ".claude.json"), `{"oauthAccount":{"emailAddress":"local@example.com"}}`)
	for _, tc := range []struct{ cli, want string }{{"claude-cloud", "cloud@example.com"}, {"claude", "local@example.com"}, {"shell", ""}} {
		if got := runtimeAccount(tc.cli, home, get); got != tc.want {
			t.Fatalf("%s identity mismatch", tc.cli)
		}
	}
	delete(env, "CLAUDE_CODE_USER_EMAIL")
	if runtimeAccount("claude-cloud", home, get) != "local@example.com" {
		t.Fatal("cloud fallback missing")
	}
	write(filepath.Join(home, ".claude.json"), `{"oauthAccount":{"emailAddress":45}}`)
	if runtimeAccount("claude", home, get) != "" {
		t.Fatal("invalid field accepted")
	}
	write(filepath.Join(home, ".claude.json"), `{"oauthAccount":{"emailAddress":"line\nbreak@example.com"}}`)
	if runtimeAccount("claude", home, get) != "" {
		t.Fatal("multiline identity accepted")
	}
	token := "fixture." + base64.RawURLEncoding.EncodeToString([]byte(`{"email":"codex@example.com","exp":1}`)) + ".unsigned"
	write(filepath.Join(home, ".codex", "auth.json"), `{"tokens":{"id_token":"`+token+`"}}`)
	if runtimeAccount("codex", home, get) != "codex@example.com" {
		t.Fatal("identity hint not decoded")
	}
	env["CODEX_HOME"] = t.TempDir()
	if runtimeAccount("codex", home, get) != "" {
		t.Fatal("override fell back to unrelated account")
	}
	write(filepath.Join(env["CODEX_HOME"], "auth.json"), `{"tokens":{"id_token":"`+token+`"}}`)
	if runtimeAccount("codex", home, get) != "codex@example.com" {
		t.Fatal("Codex override ignored")
	}
	for _, raw := range []string{`{}`, `{"tokens":{"id_token":"bad"}}`, `{"tokens":{"id_token":"a.%%%.c"}}`, `{"OPENAI_API_KEY":"fixture"}`, strings.Repeat("x", (1<<20)+1)} {
		write(filepath.Join(env["CODEX_HOME"], "auth.json"), raw)
		if runtimeAccount("codex", home, get) != "" {
			t.Fatal("invalid credentials yielded an identity")
		}
	}
	if runtimeAccount("claude", "", func(string) string { return "" }) != "" {
		t.Fatal("missing home must not inspect real user")
	}
}

func TestSessionAccountReportsWithoutModelAndRefreshes(t *testing.T) {
	reports := make(chan api.Runtime, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me":
			json.NewEncoder(w).Encode(api.Agent{Binding: api.Binding{Harness: "claude-cloud"}, Runtime: &api.Runtime{Account: "previous@example.com", Model: "existing-model", ModelSource: "observed", Quota: &api.Quota{Source: "claude", Status: "ok", Windows: []api.QuotaWindow{{Name: "five_hour", Used: .9}}}}})
		case "/v1/me/runtime":
			var rt api.Runtime
			if err := json.NewDecoder(r.Body).Decode(&rt); err != nil {
				t.Error(err)
			}
			reports <- rt
			json.NewEncoder(w).Encode(map[string]any{})
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	c, err := client.New(server.URL, "fixture-token")
	if err != nil {
		t.Fatal(err)
	}
	vars := map[string]string{"CLAUDE_CODE_REMOTE_SESSION_ID": "fixture-cloud", "CLAUDE_CODE_USER_EMAIL": "first@example.com", "HOME": t.TempDir()}
	get := func(k string) string { return vars[k] }
	reportSessionRuntime(context.Background(), c, get)
	vars["CLAUDE_CODE_USER_EMAIL"] = "second@example.com"
	reportSessionRuntime(context.Background(), c, get)
	if len(reports) != 2 {
		t.Fatal("expected two identity reports")
	}
	first, second := <-reports, <-reports
	if first.Account != "first@example.com" || second.Account != "second@example.com" {
		t.Fatal("account-only reporting or refresh failed")
	}
	for _, rt := range []api.Runtime{first, second} {
		if rt.Quota != nil {
			t.Fatal("previous account quota carried into changed identity")
		}
		if rt.Model != "existing-model" || rt.ModelSource != "observed" {
			t.Fatal("account report changed model provenance")
		}
	}
}

func TestRuntimeStableAccountDigest(t *testing.T) {
	home := t.TempDir()
	get := func(string) string { return "" }
	if e := os.WriteFile(filepath.Join(home, ".claude.json"), []byte(`{"oauthAccount":{"accountUuid":"stable-a","emailAddress":"demo@example.com"}}`), 0600); e != nil {
		t.Fatal(e)
	}
	digest := runtimeAccountIDHash("claude", home, get, "demo@example.com")
	if !strings.HasPrefix(digest, "qa_") || len(digest) != 67 || strings.Contains(digest, "stable-a") {
		t.Fatal("invalid identity digest")
	}
	if runtimeAccountIDHash("claude-cloud", home, get, "other@example.com") != "" {
		t.Fatal("cloud email associated an unrelated local account")
	}
	if runtimeAccountIDHash("claude", home, get, "demo@example.com") != digest {
		t.Fatal("identity not stable")
	}
	override := t.TempDir()
	getOverride := func(k string) string {
		if k == "CLAUDE_CONFIG_DIR" {
			return override
		}
		return ""
	}
	if runtimeAccount("claude", home, getOverride) != "" || runtimeAccountIDHash("claude", home, getOverride, "demo@example.com") != "" {
		t.Fatal("config override fell back to default account")
	}
}
