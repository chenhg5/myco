package console

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/config"
)

// Agents started here sign in the way the person picks: the shell's API
// variables are taken over (never shown), and "account" puts Claude's own
// settings back, keeping a Codex endpoint and other variables.
func TestClaudeAuthChoice(t *testing.T) {
	t.Setenv("CC_SWITCH_DIR", t.TempDir())
	home := t.TempDir()
	l := &Local{Home: home, LookPath: func(string) (string, error) { return "", os.ErrNotExist }, Dirs: []string{}}
	node := config.ProfilePath(l.defaultConfig(), "mac-machine")
	envFile := filepath.Join(home, ".myco", "env", "mac-machine.env")
	_ = os.MkdirAll(filepath.Dir(envFile), 0o700)
	_ = os.WriteFile(envFile, []byte("OPENAI_BASE_URL=https://codex.example\nFAKE=1\n"), 0o600)
	if err := config.Save(node, &config.Config{Agent: "mac-machine", Harness: "machine", Hub: "http://hub", Runner: config.Runner{Harness: "machine", EnvFile: envFile}}); err != nil {
		t.Fatal(err)
	}
	old := loginShellEnv
	loginShellEnv = func(context.Context) map[string]string {
		return map[string]string{"ANTHROPIC_BASE_URL": "https://api.magpie.example/v1", "ANTHROPIC_AUTH_TOKEN": "sk-secret-123"}
	}
	t.Cleanup(func() { loginShellEnv = old })
	b := &Backend{Local: l}
	srv := httptest.NewServer(b.Handler())
	defer srv.Close()
	call := func(method, body string) (int, ClaudeAuth, string) {
		req, _ := http.NewRequest(method, srv.URL+"/app/local/claude-auth", strings.NewReader(body))
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(res.Body)
		var a ClaudeAuth
		_ = json.Unmarshal(buf.Bytes(), &a)
		return res.StatusCode, a, buf.String()
	}
	code, a, raw := call("GET", "")
	if code != 200 || a.ShellHost != "api.magpie.example" || len(a.Shell) != 2 || a.Using != "" || strings.Contains(raw, "sk-secret") {
		t.Fatalf("get: %d %s", code, raw)
	}
	code, a, raw = call("POST", `{"use":"shell"}`)
	if code != 200 || a.Using != "shell" || a.UsingName != "api.magpie.example" || strings.Contains(raw, "sk-secret") {
		t.Fatalf("shell: %d %s", code, raw)
	}
	got, _ := os.ReadFile(envFile)
	for _, want := range []string{"ANTHROPIC_AUTH_TOKEN=sk-secret-123", "ANTHROPIC_BASE_URL=https://api.magpie.example/v1", "OPENAI_BASE_URL=https://codex.example", "FAKE=1"} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("env file lacks %s:\n%s", want, got)
		}
	}
	code, a, _ = call("POST", `{"use":"account"}`)
	got, _ = os.ReadFile(envFile)
	if code != 200 || a.Using != "" || strings.Contains(string(got), "ANTHROPIC_") || !strings.Contains(string(got), "OPENAI_BASE_URL") || !strings.Contains(string(got), "FAKE=1") {
		t.Fatalf("account: %d %+v\n%s", code, a, got)
	}
	if code, _, _ := call("POST", `{"use":"rm -rf"}`); code != 400 {
		t.Fatalf("bad use: %d", code)
	}
}
