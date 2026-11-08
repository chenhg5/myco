package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCLIConfigModel(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "claude"), 0o700)
	os.MkdirAll(filepath.Join(dir, "codex"), 0o700)
	os.WriteFile(filepath.Join(dir, "claude", "settings.json"), []byte(`{"model":"claude-opus-5-5","theme":"dark"}`), 0o600)
	os.WriteFile(filepath.Join(dir, "codex", "config.toml"), []byte("model = \"gpt-6-sol\"\napproval = \"never\"\n\n[profiles.fast]\nmodel = \"gpt-6-mini\"\n"), 0o600)
	env := map[string]string{"CLAUDE_CONFIG_DIR": filepath.Join(dir, "claude"), "CODEX_HOME": filepath.Join(dir, "codex")}
	get := func(k string) string { return env[k] }
	if m := cliConfigModel("claude", get); m != "claude-opus-5-5" {
		t.Errorf("claude: %q", m)
	}
	if m := cliConfigModel("codex", get); m != "gpt-6-sol" {
		t.Errorf("codex: %q (a profile's model must not win)", m)
	}
	os.WriteFile(filepath.Join(dir, "codex", "config.toml"), []byte("[profiles.fast]\nmodel = \"gpt-6-mini\"\n"), 0o600)
	if m := cliConfigModel("codex", get); m != "" {
		t.Errorf("codex without a top-level model: %q", m)
	}
	if m := cliConfigModel("exec", get); m != "" {
		t.Errorf("exec: %q", m)
	}
}
