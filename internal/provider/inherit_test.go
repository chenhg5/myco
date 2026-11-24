package provider

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func names(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestInheritCodexCustomProvider(t *testing.T) {
	home := t.TempDir()
	codex := filepath.Join(home, "codex-home")
	os.MkdirAll(codex, 0o755)
	// A gateway (magpie) and a cc-switch provider, each naming its key variable.
	os.WriteFile(filepath.Join(codex, "config.toml"), []byte(`model_provider = "magpie"
model = "gpt-5"
[model_providers.magpie]
base_url = "http://127.0.0.1:3425/v1"
env_key = "MAGPIE_API_KEY"
[model_providers.deepseek]
base_url = "https://api.deepseek.com"
env_key = "DEEPSEEK_API_KEY"
`), 0o600)
	env := []string{"CODEX_HOME=" + codex, "MAGPIE_API_KEY=mk", "DEEPSEEK_API_KEY=dk",
		"OPENAI_API_KEY=ok", "OPENAI_BASE_URL=https://elsewhere", "HTTPS_PROXY=http://127.0.0.1:7890",
		"PATH=/usr/bin", "ANTHROPIC_API_KEY=ak", "UNRELATED=x"}
	got := Inherit("codex", home, env)
	// OPENAI_* is not used by a custom provider: left out, so it does not
	// turn the runner to the built-in one.
	want := []string{"CODEX_HOME", "DEEPSEEK_API_KEY", "HTTPS_PROXY", "MAGPIE_API_KEY"}
	if !reflect.DeepEqual(names(got), want) || got["MAGPIE_API_KEY"] != "mk" {
		t.Fatalf("got %v, want %v", names(got), want)
	}
}

func TestInheritCodexBuiltinProvider(t *testing.T) {
	home := t.TempDir() // no config.toml: the built-in OpenAI provider
	got := Inherit("codex", home, []string{"OPENAI_API_KEY=ok", "OPENAI_BASE_URL=https://gw.example/v1", "ANTHROPIC_API_KEY=ak"})
	if want := []string{"OPENAI_API_KEY", "OPENAI_BASE_URL"}; !reflect.DeepEqual(names(got), want) {
		t.Fatalf("got %v, want %v", names(got), want)
	}
	if got := Inherit("codex", home, []string{"PATH=/bin"}); got != nil {
		t.Fatalf("nothing to inherit: %v", got)
	}
}

func TestInheritClaudeSkipsSettingsFile(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".claude"), 0o755)
	// cc-switch writes the provider into settings.json: what equals it there
	// is left to the file, so switching providers later still applies.
	os.WriteFile(filepath.Join(home, ".claude", "settings.json"), []byte(`{"env":{"ANTHROPIC_BASE_URL":"https://a.example","ANTHROPIC_AUTH_TOKEN":"from-file"}}`), 0o600)
	env := []string{"ANTHROPIC_BASE_URL=https://a.example", "ANTHROPIC_AUTH_TOKEN=from-file",
		"ANTHROPIC_MODEL=m1", "ANTHROPIC_SMALL_FAST_MODEL=m2", "CLAUDE_CODE_USE_BEDROCK=1", "AWS_REGION=us-east-1",
		"CLAUDECODE=1", "CLAUDE_CODE_ENTRYPOINT=cli", "OPENAI_API_KEY=ok"}
	got := Inherit("claude", home, env)
	want := []string{"ANTHROPIC_MODEL", "ANTHROPIC_SMALL_FAST_MODEL", "AWS_REGION", "CLAUDE_CODE_USE_BEDROCK"}
	if !reflect.DeepEqual(names(got), want) {
		t.Fatalf("got %v, want %v", names(got), want)
	}
	if got := Inherit("inbox", home, env); got != nil {
		t.Fatalf("inbox: %v", got)
	}
}
