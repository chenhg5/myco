package cli

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestResolveProvider(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CC_SWITCH_DIR", dir)
	envFile := filepath.Join(dir, "a.env")
	if err := os.WriteFile(envFile, []byte("ANTHROPIC_BASE_URL=https://proxy.example/v1\nANTHROPIC_API_KEY=k\nOTHER=x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// No cc-switch: the env file's provider variables are used as settings.
	ch := resolveProvider("claude", "", envFile)
	if ch.source != "env_file" || ch.name != "proxy.example" || len(ch.settings.Env) != 2 {
		t.Fatalf("env file: %+v", ch)
	}
	if ch := resolveProvider("codex", "", envFile); ch.settings != nil {
		t.Fatalf("codex gets no claude settings: %+v", ch)
	}
	db, err := sql.Open("sqlite", filepath.Join(dir, "cc-switch.db"))
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE TABLE providers (id TEXT, app_type TEXT, name TEXT, settings_config TEXT, created_at INTEGER, sort_index INTEGER, is_current BOOLEAN)`,
		`INSERT INTO providers VALUES ('glm','claude','GLM','{"env":{"ANTHROPIC_BASE_URL":"https://open.bigmodel.cn/api/anthropic","ANTHROPIC_MODEL":"glm-5"}}',0,0,0),
			('ds','claude','DeepSeek','{"env":{}}',0,1,1)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	ch = resolveProvider("claude", "cc-switch:glm", envFile)
	if ch.source != "pinned" || ch.name != "GLM" || ch.model != "glm-5" || ch.settings == nil {
		t.Fatalf("pinned: %+v", ch)
	}
	ch = resolveProvider("claude", "cc-switch:gone", "")
	if ch.source != "missing" || ch.settings != nil || ch.warn == "" || ch.name != "DeepSeek" {
		t.Fatalf("missing: %+v", ch)
	}
	ch = resolveProvider("claude", "", "")
	if ch.source != "machine" || ch.name != "DeepSeek" || ch.settings != nil {
		t.Fatalf("machine: %+v", ch)
	}
}
