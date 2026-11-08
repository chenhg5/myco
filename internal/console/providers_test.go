package console_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/config"
	"github.com/chenhg5/myco/internal/console"
	_ "modernc.org/sqlite"
)

func TestLocalAgentProvider(t *testing.T) {
	r := newRig(t)
	home := t.TempDir()
	t.Setenv("CC_SWITCH_DIR", "")
	b := &console.Backend{Store: console.NewStore(r.store), Secrets: r.secrets, Local: &console.Local{Home: home}, Shell: "mac", Version: "v1"}
	r.app.Config.Handler = b.Handler()

	if code, m, _ := r.do("GET", "/app/local/providers", nil); code != 200 || m["cc_switch"] != false {
		t.Fatalf("no cc-switch: %d %v", code, m)
	}
	ccs := filepath.Join(home, ".cc-switch")
	os.MkdirAll(ccs, 0o700)
	db, err := sql.Open("sqlite", filepath.Join(ccs, "cc-switch.db"))
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE TABLE providers (id TEXT, app_type TEXT, name TEXT, settings_config TEXT, created_at INTEGER, sort_index INTEGER, is_current BOOLEAN)`,
		`INSERT INTO providers VALUES ('glm','claude','GLM','{"env":{"ANTHROPIC_BASE_URL":"https://open.bigmodel.cn/api/anthropic","ANTHROPIC_AUTH_TOKEN":"secret"}}',0,0,1)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	code, m, raw := r.do("GET", "/app/local/providers", nil)
	if code != 200 || m["cc_switch"] != true || !strings.Contains(string(raw), "open.bigmodel.cn") || strings.Contains(string(raw), "secret") {
		t.Fatalf("providers: %d %s", code, raw)
	}

	// An agent that does not run here cannot be set.
	if code, m, _ := r.do("GET", "/app/local/agents/docs/provider", nil); code != 200 || m["local"] != false {
		t.Fatalf("not local: %d %v", code, m)
	}
	def := filepath.Join(home, ".myco", "config.yaml")
	prof := config.ProfilePath(def, "docs")
	if err := config.Save(prof, &config.Config{Hub: "http://hub", Agent: "docs", Harness: "claude"}); err != nil {
		t.Fatal(err)
	}
	if code, m, _ := r.do("PUT", "/app/local/agents/docs/provider", map[string]string{"provider": "cc-switch:glm"}); code != 200 || m["provider"] != "cc-switch:glm" {
		t.Fatalf("pin: %d %v", code, m)
	}
	if c, _ := config.Load(prof); c.Runner.Provider != "cc-switch:glm" {
		t.Fatalf("profile: %+v", c.Runner)
	}
	if code, _, _ := r.do("PUT", "/app/local/agents/docs/provider", map[string]string{"provider": "cc-switch:gone"}); code != 400 {
		t.Fatalf("unknown provider: %d", code)
	}
	// By hand: written to the env file (600), the pin removed.
	code, m, _ = r.do("PUT", "/app/local/agents/docs/provider", map[string]any{"manual": map[string]string{"base_url": "https://api.example.com/anthropic", "api_key": "sk-1", "model": "m1"}})
	if code != 200 || m["provider"] != "env_file" || m["env_host"] != "api.example.com" {
		t.Fatalf("manual: %d %v", code, m)
	}
	c, _ := config.Load(prof)
	vals, _ := config.ReadEnvFile(c.Runner.EnvFile)
	if c.Runner.Provider != "" || len(vals) != 3 {
		t.Fatalf("env file: %+v %v", c.Runner, vals)
	}
	if st, _ := os.Stat(c.Runner.EnvFile); st.Mode().Perm() != 0o600 && os.PathSeparator == '/' {
		t.Fatalf("mode %v", st.Mode().Perm())
	}
	// Following the machine again drops the hand-entered variables.
	if code, m, _ := r.do("PUT", "/app/local/agents/docs/provider", map[string]string{"provider": ""}); code != 200 || m["provider"] != "" {
		t.Fatalf("follow machine: %d %v", code, m)
	}
	if vals, _ := config.ReadEnvFile(c.Runner.EnvFile); len(vals) != 0 {
		t.Fatalf("left over: %v", vals)
	}
}
