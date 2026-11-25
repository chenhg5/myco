package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myco-ai/myco/internal/config"
)

// A computer lets agents open a terminal only by its own choice, made on it.
func TestMachineShellAccess(t *testing.T) {
	// machine.yaml sits next to the default config (~/.myco): a home of its own.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cfg := filepath.Join(home, ".myco", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(cfg), 0o700); err != nil {
		t.Fatal(err)
	}
	if config.DefaultPath() != cfg {
		t.Fatalf("default path %s is not in the test's home", config.DefaultPath())
	}
	h := &harnessT{t, map[string]string{}}
	if code, _, e := h.run("machine", "shell-access", "--allow"); code != ExitUsage || !strings.Contains(e, "not_configured") {
		t.Fatalf("never joined: %d %s", code, e)
	}
	if err := os.WriteFile(config.MachinePath(cfg), []byte("hub: http://x\nmachine: m1\nenroll_token: t\nfuture_key: kept\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{}, {"--allow", "--deny"}} {
		if code, _, e := h.run(append([]string{"machine", "shell-access"}, args...)...); code != ExitUsage || !strings.Contains(e, "invalid_usage") {
			t.Fatalf("%v: %d %s", args, code, e)
		}
	}
	if code, _, _ := h.run("machine", "shell-access", "--allow", "--dry-run"); code != ExitDryRun || config.ShellAllowed(cfg) {
		t.Fatalf("dry run: %d", code)
	}
	var out struct {
		MaxAccess string `json:"max_access"`
		Changed   bool   `json:"changed"`
	}
	code, o, e := h.run("machine", "shell-access", "--allow")
	if code != 0 || json.Unmarshal([]byte(o), &out) != nil || out.MaxAccess != "full" || !out.Changed || !config.ShellAllowed(cfg) {
		t.Fatalf("allow: %d %s %s", code, o, e)
	}
	if _, o, _ = h.run("machine", "shell-access", "--allow"); json.Unmarshal([]byte(o), &out) != nil || out.Changed {
		t.Fatalf("allow again: %s", o)
	}
	if code, o, _ = h.run("machine", "shell-access", "--deny"); code != 0 || json.Unmarshal([]byte(o), &out) != nil || out.MaxAccess != "workspace" || !out.Changed || config.ShellAllowed(cfg) {
		t.Fatalf("deny: %d %s", code, o)
	}
	if b, _ := os.ReadFile(config.MachinePath(cfg)); !strings.Contains(string(b), "future_key: kept") {
		t.Fatalf("other keys lost: %s", b)
	}
	// The command is in the schema.
	if _, o, _ = h.run("schema", "--all"); !strings.Contains(o, "shell-access") || !strings.Contains(o, "allow-shell") {
		t.Fatal("schema lacks shell-access / --allow-shell")
	}
}
