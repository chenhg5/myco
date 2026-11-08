package provider_test

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/chenhg5/myco/internal/provider"
)

// FakeDB writes a cc-switch database with the columns we read.
func FakeDB(t *testing.T, dir string) {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(dir, "cc-switch.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, q := range []string{
		`CREATE TABLE providers (id TEXT NOT NULL, app_type TEXT NOT NULL, name TEXT NOT NULL, settings_config TEXT NOT NULL,
			website_url TEXT, category TEXT, created_at INTEGER, sort_index INTEGER, notes TEXT, icon TEXT, icon_color TEXT,
			meta TEXT NOT NULL DEFAULT '{}', is_current BOOLEAN NOT NULL DEFAULT 0, in_failover_queue BOOLEAN NOT NULL DEFAULT 0,
			PRIMARY KEY (id, app_type))`,
		`INSERT INTO providers (id, app_type, name, settings_config, sort_index, is_current) VALUES
			('official', 'claude', 'Claude Official', '{"env":{}}', 0, 0),
			('ds', 'claude', 'DeepSeek', '{"env":{"ANTHROPIC_BASE_URL":"https://api.deepseek.com/anthropic","ANTHROPIC_AUTH_TOKEN":"sk-ds","ANTHROPIC_MODEL":"deepseek-chat","API_TIMEOUT_MS":600000}}', 1, 1),
			('ds', 'codex', 'DeepSeek', '{"auth":{},"config":""}', 0, 1)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
}

func TestListAndResolve(t *testing.T) {
	dir := t.TempDir()
	if _, err := provider.List(dir, "claude"); !errors.Is(err, provider.ErrNoCCSwitch) {
		t.Fatalf("no cc-switch: %v", err)
	}
	FakeDB(t, dir)
	ps, err := provider.List(dir, "claude")
	if err != nil || len(ps) != 2 || ps[0].Name != "Claude Official" || !ps[1].Current {
		t.Fatalf("list: %+v %v", ps, err)
	}
	p, err := provider.Resolve(dir, "cc-switch:ds", "claude")
	if err != nil || p.Host != "api.deepseek.com" || p.Env["ANTHROPIC_AUTH_TOKEN"] != "sk-ds" || p.Env["API_TIMEOUT_MS"] != "600000" {
		t.Fatalf("resolve: %+v %v", p, err)
	}
	if _, err := provider.Resolve(dir, "cc-switch:gone", "claude"); !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if cur, ok := provider.Current(dir, "claude"); !ok || cur.ID != "ds" {
		t.Fatalf("current: %+v", cur)
	}
}

func TestLegacyConfig(t *testing.T) {
	dir := t.TempDir()
	body := `{"version":2,"claude":{"providers":{"k":{"id":"k","name":"Kimi","settingsConfig":{"env":{"ANTHROPIC_BASE_URL":"https://api.moonshot.cn/anthropic"}}}},"current":"k"}}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	ps, err := provider.List(dir, "claude")
	if err != nil || len(ps) != 1 || ps[0].Ref != "cc-switch:k" || !ps[0].Current || ps[0].Host != "api.moonshot.cn" {
		t.Fatalf("legacy: %+v %v", ps, err)
	}
}
