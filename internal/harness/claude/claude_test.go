package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/harness"
)

func TestParseStream(t *testing.T) {
	out := strings.Join([]string{
		`{"type":"system","subtype":"init","session_id":"s-1"}`,
		`{"type":"assistant","session_id":"s-1","message":{"content":[{"type":"text","text":"partial answer"}]}}`,
		`{"type":"result","subtype":"success","is_error":false,"result":"PONG","session_id":"s-1"}`,
	}, "\n")
	res, final, ok := parseStream(out)
	if !ok || final.IsError || res.Output != "PONG" || res.SessionID != "s-1" {
		t.Fatalf("%+v %+v %v", res, final, ok)
	}
	// Killed before the result event: fall back to the last assistant text.
	res, _, ok = parseStream(strings.Join(strings.Split(out, "\n")[:2], "\n"))
	if ok || res.Output != "partial answer" || res.SessionID != "s-1" {
		t.Fatalf("fallback: %+v %v", res, ok)
	}
}

func TestInheritedSessionEnv(t *testing.T) {
	if got := inheritedSessionEnv([]string{"CLAUDE_CONFIG_DIR=/x", "CLAUDE_CODE_SESSION_ID=a"}); got != nil {
		t.Fatalf("not nested: nothing to strip, got %v", got)
	}
	got := inheritedSessionEnv([]string{"CLAUDECODE=1", "CLAUDE_CODE_SESSION_ID=a", "CLAUDE_CODE_REMOTE=true",
		"CLAUDE_CONFIG_DIR=/x", "CLAUDE_CODE_OAUTH_TOKEN=t", "PATH=/bin"})
	want := map[string]bool{"CLAUDECODE": true, "CLAUDE_CODE_SESSION_ID": true, "CLAUDE_CODE_REMOTE": true}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for _, k := range got {
		if !want[k] {
			t.Fatalf("unexpected %s in %v", k, got)
		}
	}
}

// The provider file overrides settings.json key by key: keys the user's
// settings set but the provider does not are blanked, others kept out.
func TestProviderSettingsFile(t *testing.T) {
	dir := t.TempDir()
	user := `{"model":"opus","env":{"ANTHROPIC_BASE_URL":"https://old","ANTHROPIC_MODEL":"old-model","CLAUDE_CODE_USE_BEDROCK":"1","DISABLE_TELEMETRY":"1"}}`
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(user), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := providerSettingsFile(&harness.ProviderSettings{Env: map[string]string{"ANTHROPIC_BASE_URL": "https://new", "ANTHROPIC_AUTH_TOKEN": "k"}}, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f)
	if st, _ := os.Stat(f); runtime.GOOS != "windows" && st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", st.Mode().Perm())
	}
	b, _ := os.ReadFile(f)
	var got struct {
		Env   map[string]string `json:"env"`
		Model string            `json:"model"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"ANTHROPIC_BASE_URL": "https://new", "ANTHROPIC_AUTH_TOKEN": "k", "ANTHROPIC_MODEL": "", "CLAUDE_CODE_USE_BEDROCK": ""}
	if len(got.Env) != len(want) || got.Model != "" {
		t.Fatalf("got %s", b)
	}
	for k, v := range want {
		if got.Env[k] != v {
			t.Fatalf("%s = %q in %s", k, got.Env[k], b)
		}
	}
}
