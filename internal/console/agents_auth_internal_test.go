package console

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func isolatedAuthLocal(t *testing.T) *Local {
	t.Helper()
	for _, key := range []string{
		"CODEBUDDY_API_KEY",
		"CODEBUDDY_AUTH_TOKEN",
		"CODEBUDDY_CONFIG_DIR",
		"CODEBUDDY_AUTH_FILE_PATH",
		"QODER_CONFIG_DIR",
		"QODER_PERSONAL_ACCESS_TOKEN",
		"CODEBUDDY_API_KEY_DISABLED",
		"CODEBUDDY_API_KEY_HELPER_DISABLED",
		"KIMI_CODE_HOME",
		"COPILOT_HOME",
		"COPILOT_GITHUB_TOKEN",
		"GH_TOKEN",
		"GITHUB_TOKEN",
		"COPILOT_PROVIDER_BASE_URL",
		"COPILOT_PROVIDER_API_KEY",
		"AUGMENT_CACHE_DIR",
		"AUGMENT_CODING_AGENT_DIR",
		"AUGMENT_SESSION_AUTH",
		"AUGMENT_API_TOKEN",
		"AUGMENT_API_URL",
		"CLINE_DIR",
		"CLINE_DATA_DIR",
		"CLINE_PROVIDER_SETTINGS_PATH",
		"GROK_HOME",
		"GROK_AUTH_PATH",
		"GROK_DISABLE_API_KEY_AUTH",
		"XAI_API_KEY",
		"GEMINI_API_KEY",
		"GOOSE_PATH_ROOT",
		"GOOSE_PROVIDER",
		"GOOSE_ADDITIONAL_CONFIG_FILES",
		"XDG_CONFIG_HOME",
		"XDG_DATA_HOME",
		"KILO_DB",
		"APPDATA",
		"GROK_FORCE_LOGIN_TEAM_ID",
		"AGY_PATH",
	} {
		t.Setenv(key, "")
	}
	return &Local{Home: t.TempDir()}
}

func writeAuthFixture(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestCodebuddyCredentials(t *testing.T) {
	l := isolatedAuthLocal(t)
	state := func(want string) {
		t.Helper()
		if got := l.signedIn(context.Background(), "codebuddy", "unused"); got != want {
			t.Fatalf("state=%s want=%s", got, want)
		}
	}
	state("unknown")
	// MCP credentials and merely selecting an auth method are not a sign-in.
	writeAuthFixture(t, filepath.Join(l.Home, ".codebuddy", ".credentials.json"), `{"accessToken":"fixture-mcp"}`)
	writeAuthFixture(t, filepath.Join(l.Home, ".codebuddy", "settings.json"), `{"selectedAuthType":"oauth"}`)
	state("unknown")
	auth := filepath.Join(codebuddyDataDir(l.Home, runtime.GOOS), "Data", "Public", "auth", "Tencent-Cloud.coding-copilot.info")
	writeAuthFixture(t, auth, `{"auth":{"accessToken":""},"account":{"id":"demo"}}`)
	state("unknown")
	writeAuthFixture(t, auth, `{"auth":{"accessToken":"fixture-secret"},"account":{"id":"demo"}}`)
	state("ready")
	writeAuthFixture(t, auth+".logged-out", "logout")
	state("signed_out")
	if err := os.Remove(auth + ".logged-out"); err != nil {
		t.Fatal(err)
	}
	// Protected fields are detected without decrypting or invoking the keychain.
	writeAuthFixture(t, auth, `{"auth":{"accessToken":{"ciphertext":"fixture"}},"account":{"id":"demo"}}`)
	state("ready")
	writeAuthFixture(t, auth, `{"auth":`)
	state("unknown")
	custom := filepath.Join(t.TempDir(), "auth.info")
	t.Setenv("CODEBUDDY_AUTH_FILE_PATH", custom)
	writeAuthFixture(t, custom, `{"auth":{"refreshToken":"fixture"},"account":{"id":"demo"}}`)
	state("ready")
}

func TestCodebuddyAPIConfiguration(t *testing.T) {
	for _, body := range []string{`{"env":{"CODEBUDDY_API_KEY":"fixture"}}`, `{"env":{"CODEBUDDY_AUTH_TOKEN":"fixture"}}`, `{"apiKeyHelper":"example-helper"}`} {
		t.Run(body[:10], func(t *testing.T) {
			l := isolatedAuthLocal(t)
			dir := t.TempDir()
			t.Setenv("CODEBUDDY_CONFIG_DIR", dir)
			writeAuthFixture(t, filepath.Join(dir, "settings.json"), body)
			if l.codebuddyAuthState() != "ready" {
				t.Fatal("configured credentials not detected")
			}
		})
	}
	l := isolatedAuthLocal(t)
	// A GUI gets its terminal's environment through the existing shell-env reader.
	l.ShellEnv = func() []string { return []string{"CODEBUDDY_AUTH_TOKEN=fixture"} }
	if l.codebuddyAuthState() != "ready" {
		t.Fatal("shell credential not detected")
	}
}

func TestCodebuddyPlatformDataPaths(t *testing.T) {
	for _, tc := range []struct {
		os    string
		parts []string
	}{
		{"darwin", []string{"Library", "Application Support", "CodeBuddyExtension"}},
		{"windows", []string{"AppData", "Local", "CodeBuddyExtension"}},
		{"linux", []string{".local", "share", "CodeBuddyExtension"}},
	} {
		if got, want := codebuddyDataDir("demo-home", tc.os), filepath.Join(append([]string{"demo-home"}, tc.parts...)...); got != want {
			t.Fatalf("%s path=%s want=%s", tc.os, got, want)
		}
	}
}

func TestQoderAndQwenCredentials(t *testing.T) {
	l := isolatedAuthLocal(t)
	dir := t.TempDir()
	t.Setenv("QODER_CONFIG_DIR", dir)
	path := filepath.Join(dir, "qoder_oauth_creds.json")
	for _, tc := range []struct {
		body  string
		ready bool
	}{
		{`{}`, false}, {`{"version":1,"accounts":{}}`, false},
		{`{"version":1,"accounts":{"global":{"userName":"demo","accessToken":""}}}`, false},
		{`{"version":1,"accounts":{"global":{"accessToken":"fixture"}}}`, true},
		{`{"version":1,"accounts":{"cn":{"refreshToken":"fixture"}}}`, true},
		{`{"version":2,"accounts":{"global":{"accessToken":"fixture"}}}`, false},
		{`{"version":`, false},
	} {
		writeAuthFixture(t, path, tc.body)
		if got := l.qoderSignedIn(); got != tc.ready {
			t.Fatalf("credential detection=%v want=%v", got, tc.ready)
		}
	}
	t.Setenv("QODER_PERSONAL_ACCESS_TOKEN", "fixture")
	if !l.qoderSignedIn() {
		t.Fatal("PAT not detected")
	}
	if l.qwenSignedIn() {
		t.Fatal("Qoder credentials must not sign Qwen in")
	}
	writeAuthFixture(t, filepath.Join(l.Home, ".qwen", "oauth_creds.json"), `{"access_token":"fixture"}`)
	if !l.qwenSignedIn() {
		t.Fatal("Qwen OAuth not detected")
	}
}

func TestRecentAgentSuccess(t *testing.T) {
	l := isolatedAuthLocal(t)
	now := time.Now()
	writeAuthFixture(t, filepath.Join(l.Home, ".myco", "agents", "demo.yaml"), "hub: https://hub.example.com\nagent: demo\nrunner:\n  harness: qoder\n")
	path := filepath.Join(l.Home, ".myco", "state", "demo.json")
	for _, tc := range []struct {
		cli   string
		at    time.Time
		ready bool
	}{
		{"qoder", now.Add(-time.Hour), true}, {"qoder", now.Add(-25 * time.Hour), false},
		{"qoder", now.Add(time.Hour), false}, {"codebuddy", now, false},
	} {
		b, _ := json.Marshal(map[string]any{"success": map[string]any{"cli": tc.cli, "at": tc.at}})
		writeAuthFixture(t, path, string(b))
		if got := l.recentAgentSuccess("qoder", now); got != tc.ready {
			t.Fatalf("recent success=%v want=%v", got, tc.ready)
		}
	}
	writeAuthFixture(t, path, `{"runs":{"session":123},"tasks":{}}`)
	if l.recentAgentSuccess("qoder", now) {
		t.Fatal("ordinary activity is not a successful task")
	}
	b, _ := json.Marshal(map[string]any{"success": map[string]any{"cli": "qoder", "at": now}})
	writeAuthFixture(t, path, string(b))
	writeAuthFixture(t, filepath.Join(l.Home, ".myco", "agents", "demo.yaml"), "agent: demo\nharness: qoder\nreplaced: moved\n")
	if l.recentAgentSuccess("qoder", now) {
		t.Fatal("replaced profile must not prove readiness")
	}
}
