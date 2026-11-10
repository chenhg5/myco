package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/testutil"
)

// The project commands from both sides: a person creates a project, adds a
// node and makes it the coordinator; the coordinator writes the brief and a
// document; a member lists its projects, shows one and pulls its context.
func TestProjectCommands(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)
	hub := newHub(t)
	admin := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(home, "admin.yaml")}}
	_, out, _ := admin.run("enrollment", "create", "--name-prefix", "dev-", "--format", "json")
	var created struct{ Token string }
	json.Unmarshal([]byte(out), &created)
	agent := func(name, sid string) *harnessT {
		h := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ENROLL_TOKEN": created.Token, "CLAUDE_CODE_SESSION_ID": sid,
			"MYCO_CONFIG": filepath.Join(home, name+".yaml")}}
		if code, _, e := h.run("agent", "enroll", "--name", name); code != 0 {
			t.Fatalf("enroll %s: %s", name, e)
		}
		return h
	}
	lead, worker := agent("dev-lead", "s1"), agent("dev-worker", "s2")

	if code, _, _ := admin.run("project", "create", "--name", "myco", "--goal", "ship projects", "--dry-run"); code != ExitDryRun {
		t.Fatalf("create --dry-run: exit %d", code)
	}
	if code, _, e := admin.run("project", "create", "--name", "myco", "--title", "myco 开发", "--goal", "ship projects"); code != 0 {
		t.Fatalf("create: %s", e)
	}
	if code, _, e := admin.run("project", "add", "--name", "myco", "--agent", "dev-lead", "--role", "coordinator"); code != 0 {
		t.Fatalf("add lead: %s", e)
	}
	if code, _, e := admin.run("project", "add", "--name", "myco", "--agent", "dev-worker", "--note", "后端"); code != 0 {
		t.Fatalf("add worker: %s", e)
	}
	// The coordinator writes the brief and a document with its own token.
	brief := filepath.Join(home, "BRIEF.md")
	os.WriteFile(brief, []byte("Run make check before pushing."), 0o600)
	if code, _, e := lead.run("project", "update", "--name", "myco", "--brief-file", brief, "--link", "repo=https://github.com/chenhg5/myco"); code != 0 {
		t.Fatalf("coordinator updates: %s", e)
	}
	doc := filepath.Join(home, "conv.md")
	os.WriteFile(doc, []byte("# Conventions\nKeep commits small."), 0o600)
	if code, _, e := lead.run("project", "doc", "put", "--name", "myco", "--doc", "conventions", "--file", doc); code != 0 {
		t.Fatalf("doc put: %s", e)
	}
	// A plain member cannot.
	if code, _, _ := worker.run("project", "doc", "put", "--name", "myco", "--doc", "x", "--file", doc); code != ExitPermission {
		t.Fatalf("member doc put: exit %d", code)
	}
	// The member sees its project, shows it, pulls it.
	_, out, _ = worker.run("project", "list", "--format", "json")
	if !strings.Contains(out, `"name": "myco"`) {
		t.Fatalf("list: %s", out)
	}
	_, out, _ = worker.run("project", "show", "--name", "myco", "--format", "text")
	for _, want := range []string{"ship projects", "Run make check", "`dev-lead` (coordinator)", "后端", "conventions"} {
		if !strings.Contains(out, want) {
			t.Fatalf("show lacks %q:\n%s", want, out)
		}
	}
	dir := filepath.Join(home, "ctx")
	if code, _, e := worker.run("project", "pull", "--name", "myco", "--dir", dir); code != 0 {
		t.Fatalf("pull: %s", e)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "myco", "PROJECT.md"))
	d, _ := os.ReadFile(filepath.Join(dir, "myco", "docs", "conventions.md"))
	if !strings.Contains(string(b), "## Brief") || !strings.Contains(string(b), "docs/conventions.md") || string(d) != "# Conventions\nKeep commits small." {
		t.Fatalf("pulled:\n%s\n---\n%s", b, d)
	}
	// Leaving.
	if code, _, e := worker.run("project", "remove", "--name", "myco"); code != 0 {
		t.Fatalf("leave: %s", e)
	}
}
