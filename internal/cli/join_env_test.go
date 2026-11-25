package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/myco-ai/myco/internal/config"
)

// myco join keeps the variables the session reaches its model with for the
// runner: in the agent's private env file, merged with what is there.
func TestJoinInheritsProviderEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	codex := filepath.Join(home, "codex-home")
	os.MkdirAll(codex, 0o755)
	os.WriteFile(filepath.Join(codex, "config.toml"), []byte("model_provider = \"magpie\"\n[model_providers.magpie]\nbase_url = \"http://127.0.0.1:3425/v1\"\nenv_key = \"MAGPIE_API_KEY\"\n"), 0o600)
	defer func(f func() []string) { environ = f }(environ)
	environ = func() []string {
		return []string{"CODEX_HOME=" + codex, "MAGPIE_API_KEY=k1", "OPENAI_API_KEY=unused", "PATH=/bin"}
	}
	defaultPath := filepath.Join(home, ".myco", "config.yaml")
	profile := config.ProfilePath(defaultPath, "mac-codex-1234")
	os.MkdirAll(filepath.Dir(profile), 0o700)
	if err := config.Save(profile, &config.Config{Hub: "https://hub.example", Agent: "mac-codex-1234", Token: "t"}); err != nil {
		t.Fatal(err)
	}
	// Something captured before stays.
	envFile := filepath.Join(config.Dir(defaultPath), "env", "mac-codex-1234.env")
	os.MkdirAll(filepath.Dir(envFile), 0o700)
	os.WriteFile(envFile, []byte("EXTRA=keep\nMAGPIE_API_KEY=old\n"), 0o600)

	names, file, err := inheritEnv(profile, defaultPath, "codex")
	if err != nil || file != envFile || !reflect.DeepEqual(names, []string{"CODEX_HOME", "MAGPIE_API_KEY"}) {
		t.Fatalf("inheritEnv: %v %s %v", names, file, err)
	}
	vars, err := config.ReadEnvFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(vars, " "); got != "CODEX_HOME="+codex+" EXTRA=keep MAGPIE_API_KEY=k1" {
		t.Fatalf("env file: %s", got)
	}
	if st, _ := os.Stat(file); st.Mode().Perm() != 0o600 && os.PathSeparator == '/' {
		t.Fatalf("mode %v", st.Mode())
	}
	cfg, _ := config.Load(profile)
	if cfg.Runner.EnvFile != envFile {
		t.Fatalf("runner.env_file = %q", cfg.Runner.EnvFile)
	}
	// The runner passes the key on and leaves the provider to config.toml.
	ch := resolveProvider("codex", "", envFile)
	if ch.settings != nil && len(ch.settings.ConfigArgs) > 0 {
		t.Fatalf("a key alone must not change the provider: %v", ch.settings.ConfigArgs)
	}

	// Nothing to inherit (an inbox agent): nothing written.
	if names, _, err := inheritEnv(profile, defaultPath, "inbox"); err != nil || names != nil {
		t.Fatalf("inbox: %v %v", names, err)
	}
}
