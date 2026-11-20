package agenttools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func put(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}
func TestScanRedactsSecretsAndScopes(t *testing.T) {
	home := t.TempDir()
	project := filepath.Join(home, "work")
	claude := filepath.Join(home, ".claude")
	codex := filepath.Join(home, ".codex")
	put(t, filepath.Join(home, ".claude.json"), `{"mcpServers":{"local":{"command":"secret-binary","args":["secret-arg"],"env":{"TOKEN":"secret-env"}}},"projects":{}}`)
	put(t, filepath.Join(project, ".mcp.json"), `{"mcpServers":{"remote":{"type":"sse","url":"https://user:secret-pass@example.com/secret-path?token=secret-query","headers":{"Authorization":"secret-header"}}}}`)
	put(t, filepath.Join(claude, "skills", "myco", "SKILL.md"), "---\nname: myco\ndescription: Helps use myco\n---\n<!-- installed by myco: test -->\n")
	put(t, filepath.Join(project, ".claude", "skills", "review", "SKILL.md"), "---\nname: review\ndescription: Review work\n---\n")
	put(t, filepath.Join(claude, "settings.json"), `{"enabledPlugins":{"one@market":true}}`)
	put(t, filepath.Join(claude, "plugins", "installed_plugins.json"), `{"plugins":{"one@market":[{"installPath":"`+filepath.ToSlash(filepath.Join(home, "plugin"))+`"}]}}`)
	put(t, filepath.Join(home, "plugin", ".mcp.json"), `{"mcpServers":{"plugin-server":{"command":"secret-plugin"}}}`)
	got := Scan("claude", Options{Home: home, ClaudeConfigDir: claude, Project: project})
	b, _ := json.Marshal(got)
	s := string(b)
	for _, secret := range []string{"secret-binary", "secret-arg", "secret-env", "secret-pass", "secret-path", "secret-query", "secret-header", "secret-plugin"} {
		if strings.Contains(s, secret) {
			t.Fatalf("secret %s leaked: %s", secret, s)
		}
	}
	if len(got.MCP) != 3 || got.MCP[1].Host != "example.com" || got.MCP[1].Scope != "project" {
		t.Fatalf("Claude MCP: %+v", got.MCP)
	}
	if len(got.Skills) != 2 || got.Skills[1].ManagedBy != "myco" || len(got.Plugins) != 1 || !got.Plugins[0].Enabled {
		t.Fatalf("Claude tools: %+v", got)
	}
	only := got.ProjectOnly(project)
	if len(only.MCP) != 1 || len(only.Skills) != 1 || len(only.Plugins) != 0 {
		t.Fatalf("project only: %+v", only)
	}
	put(t, filepath.Join(codex, "config.toml"), `[mcp_servers.docs]
url = "https://secret-user:secret-pass@docs.example/secret-path?token=secret-query"
http_headers = { Authorization = "secret-header" }
[plugins.writer]
enabled = false
`)
	put(t, filepath.Join(project, ".codex", "config.toml"), `[mcp_servers.local]
command = "secret-binary"
args = ["secret-arg"]
`)
	put(t, filepath.Join(codex, "skills", "writer", "SKILL.md"), "---\nname: writer\ndescription: Write docs\n---\n")
	got = Scan("codex", Options{Home: home, CodexHome: codex, Project: project})
	b, _ = json.Marshal(got)
	s = string(b)
	for _, secret := range []string{"secret-binary", "secret-arg", "secret-pass", "secret-path", "secret-query", "secret-header"} {
		if strings.Contains(s, secret) {
			t.Fatalf("Codex secret %s leaked: %s", secret, s)
		}
	}
	if len(got.MCP) != 2 || got.MCP[1].Host != "docs.example" || len(got.Plugins) != 1 || got.Plugins[0].Enabled {
		t.Fatalf("Codex tools: %+v", got)
	}
}
func TestScanSkipsBadConfigWithSafeWarning(t *testing.T) {
	home := t.TempDir()
	put(t, filepath.Join(home, ".claude.json"), `{"mcpServers":{"x":{"env":{"TOKEN":"secret-value"}}}`)
	got := Scan("claude", Options{Home: home})
	b, _ := json.Marshal(got)
	if len(got.Warnings) != 1 || !strings.Contains(got.Warnings[0], "invalid JSON") || strings.Contains(string(b), "secret-value") {
		t.Fatalf("unsafe warning: %+v", got)
	}
}
