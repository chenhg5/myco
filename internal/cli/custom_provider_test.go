package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myco-ai/myco/internal/config"
)

func TestRunnerCustomProviderCLI(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	def := config.DefaultPath()
	profile := config.ProfilePath(def, "custom")
	if err := config.Save(profile, &config.Config{Hub: "http://unused", Agent: "custom", Harness: "codex"}); err != nil {
		t.Fatal(err)
	}
	run := func(input string, args ...string) (int, string) {
		var out, errs bytes.Buffer
		code := Run(context.Background(), args, strings.NewReader(input), &out, &errs, func(string) string { return "" }, false)
		return code, out.String() + errs.String()
	}
	args := []string{"runner", "provider", "set", "--agent", "custom", "--provider", "custom", "--base-url", "http://127.0.0.1:3425/v1", "--model", "my-model", "--wire-api", "chat", "--key-file", "-"}
	secret := "test-private-value"
	dry := append(append([]string{}, args...), "--dry-run")
	if code, out := run(secret, dry...); code != ExitDryRun || strings.Contains(out, secret) {
		t.Fatalf("dry run failed: %d %s", code, out)
	}
	cfg, _ := config.Load(profile)
	if cfg.Runner.EnvFile != "" {
		t.Fatal("dry run wrote profile")
	}
	if code, out := run(secret, args...); code != 0 || strings.Contains(out, secret) {
		t.Fatalf("set failed: %d %s", code, out)
	}
	cfg, _ = config.Load(profile)
	ch := resolveProvider("codex", cfg.Runner.Provider, cfg.Runner.EnvFile)
	if ch.host != "127.0.0.1:3425" || ch.source != "env_file" || !strings.Contains(strings.Join(ch.settings.ConfigArgs, " "), `wire_api="chat"`) {
		t.Fatal("custom provider not selected")
	}
	raw, _ := os.ReadFile(profile)
	if strings.Contains(string(raw), secret) {
		t.Fatal("key in profile")
	}
	if code, out := run("", "runner", "provider", "set", "--agent", "custom", "--provider", "machine"); code != 0 || strings.Contains(out, secret) {
		t.Fatal("clear failed")
	}
	vals, _ := config.ReadEnvFile(cfg.Runner.EnvFile)
	if len(vals) != 0 {
		t.Fatal("clear kept provider variables")
	}
	// Invalid URLs must never be echoed even by a dry-run response.
	args[8] = "http://user:private-url-secret@host"
	if code, out := run(secret, args...); code == 0 || strings.Contains(out, "private-url-secret") || strings.Contains(out, secret) {
		t.Fatal("invalid URL accepted or echoed")
	}
}

func TestResolveCustomCodexDoesNotExposeKey(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CC_SWITCH_DIR", home)
	path := filepath.Join(home, "agent.env")
	os.WriteFile(path, []byte("OPENAI_BASE_URL=http://localhost:3425/v1\nOPENAI_API_KEY=only-env-secret\nOPENAI_MODEL=m\nOPENAI_WIRE_API=responses\n"), 0600)
	ch := resolveProvider("codex", "", path)
	if strings.Contains(strings.Join(ch.settings.ConfigArgs, " "), "only-env-secret") {
		t.Fatal("secret in argv")
	}
}
