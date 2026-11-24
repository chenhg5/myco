package provider

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myco-ai/myco/internal/config"
)

func TestCustomProviderPrivateFileAndArguments(t *testing.T) {
	file := filepath.Join(t.TempDir(), "agent.env")
	if err := os.WriteFile(file, []byte("UNRELATED=yes\nANTHROPIC_API_KEY=old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, wire := range []string{"chat", "responses"} {
		m := Manual{BaseURL: "http://127.0.0.1:3425/v1", APIKey: "private-key-$literal\\tail", Model: "custom-model", WireAPI: wire}
		if err := m.Validate("codex"); err != nil {
			t.Fatal(err)
		}
		if err := WriteEnv(file, "agent", m.Variables("codex")); err != nil {
			t.Fatal(err)
		}
		vals, err := config.ReadEnvFile(file)
		if err != nil {
			t.Fatal(err)
		}
		env := map[string]string{}
		for _, kv := range vals {
			k, v, _ := strings.Cut(kv, "=")
			env[k] = v
		}
		if env["OPENAI_API_KEY"] != m.APIKey || env["UNRELATED"] != "yes" || env["ANTHROPIC_API_KEY"] != "" {
			t.Fatal("env file did not preserve/isolate provider settings")
		}
		st, _ := os.Stat(file)
		if os.PathSeparator == '/' && st.Mode().Perm() != 0600 {
			t.Fatal("env file is not private")
		}
		args, vars := CustomCodex(m.BaseURL, m.APIKey, m.Model, m.WireAPI)
		joined := strings.Join(args, " ")
		if strings.Contains(joined, m.APIKey) || vars[CodexKeyEnv] != m.APIKey || !strings.Contains(joined, `wire_api="`+wire+`"`) || !strings.Contains(joined, `env_key="MYCO_CODEX_API_KEY"`) {
			t.Fatal("invalid or secret-bearing custom provider overrides")
		}
	}
	if err := WriteEnv(file, "agent", nil); err != nil {
		t.Fatal(err)
	}
	vals, _ := config.ReadEnvFile(file)
	if len(vals) != 1 || vals[0] != "UNRELATED=yes" {
		t.Fatalf("clear: %v", vals)
	}
}

func TestManualValidationDoesNotEchoSecrets(t *testing.T) {
	for _, base := range []string{"https://user:secret@example.test", "https://example.test/?key=secret", "file:///secret", "https://example.test/#secret", "http://"} {
		m := Manual{BaseURL: base, APIKey: "private-key"}
		err := m.Validate("claude")
		if err == nil || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "private-key") {
			t.Fatal("URL accepted or echoed")
		}
	}
	for _, wire := range []string{"garbage", "responses\nkey"} {
		m := Manual{BaseURL: "http://localhost:3425", APIKey: "k", WireAPI: wire}
		if m.Validate("codex") == nil {
			t.Fatal("invalid wire accepted")
		}
	}
	m := Manual{BaseURL: "http://localhost:3425", APIKey: "k"}
	if err := m.Validate("codex"); err != nil || m.WireAPI != "responses" {
		t.Fatal("missing wire default")
	}
	// A Provider's JSON intentionally cannot serialize its environment.
	raw, _ := json.Marshal(Provider{Env: map[string]string{"KEY": "private-key"}, CodexArgs: []string{"private-key"}})
	if strings.Contains(string(raw), "private-key") {
		t.Fatal("provider JSON leaks secrets")
	}
}

func TestMachineEndpoint(t *testing.T) {
	home := t.TempDir()
	claude := filepath.Join(home, "claude")
	codex := filepath.Join(home, "codex")
	os.MkdirAll(claude, 0700)
	os.MkdirAll(codex, 0700)
	env := map[string]string{"CLAUDE_CONFIG_DIR": claude, "CODEX_HOME": codex, "ANTHROPIC_BASE_URL": "https://shell.example/api?secret=x", "OPENAI_BASE_URL": "https://shell-openai.example/v1"}
	get := func(k string) string { return env[k] }
	host, _ := MachineEndpoint(home, "claude", get)
	if host != "shell.example" {
		t.Fatal(host)
	}
	os.WriteFile(filepath.Join(claude, "settings.json"), []byte(`{"env":{"ANTHROPIC_BASE_URL":"http://127.0.0.1:3425/anthropic?secret=k","ANTHROPIC_AUTH_TOKEN":"secret","ANTHROPIC_MODEL":"claude-m"}}`), 0600)
	host, model := MachineEndpoint(home, "claude", get)
	if host != "127.0.0.1:3425" || model != "claude-m" {
		t.Fatal(host, model)
	}
	if DisplayName(host, Provider{Name: "Old", Host: "old.example"}, true) != "magpie 网关" {
		t.Fatal("magpie label")
	}
	os.WriteFile(filepath.Join(codex, "config.toml"), []byte("model = 'codex-m'\nmodel_provider = 'gateway'\n[model_providers.gateway]\nbase_url = 'https://user:secret@gateway.example:443/v1?key=hidden'\nenv_key = 'PRIVATE_KEY'\n[model_providers.other]\nbase_url = 'https://wrong.example'\n"), 0600)
	host, model = MachineEndpoint(home, "codex", get)
	if host != "gateway.example:443" || model != "codex-m" {
		t.Fatal(host, model)
	}
	if DisplayName(host, Provider{Name: "Current", Host: host}, true) != "Current" || DisplayName(host, Provider{Name: "Wrong", Host: "other.example"}, true) != host {
		t.Fatal("cc-switch endpoint matching")
	}
}
