package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// handover-link makes a command; `myco join` with MYCO_TAKEOVER takes the
// node over and prints the handover note; the old session exits 11.
func TestHandoverLinkAndTakeOver(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())
	hub := newHub(t)
	dir := t.TempDir()
	admin := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(dir, "admin.yaml")}}
	old := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(dir, "old.yaml")}}
	if code, _, e := old.run("agent", "register", "--name", "docs"); code != 0 {
		t.Fatalf("register: %s", e)
	}
	if code, _, e := admin.run("agent", "handover-link", "--name", "docs", "--dry-run"); code != ExitDryRun {
		t.Fatalf("dry run: %d %s", code, e)
	}
	code, out, e := admin.run("agent", "handover-link", "--name", "docs", "--note", "moving accounts", "--format", "json")
	if code != 0 {
		t.Fatalf("link: %d %s", code, e)
	}
	var link struct{ URL, Command string }
	if err := json.Unmarshal([]byte(out), &link); err != nil || !strings.Contains(link.URL, "/h/myco_takeover_") {
		t.Fatalf("link: %s %v", out, err)
	}
	// A bad link names no credential in its error.
	bad := &harnessT{t, map[string]string{"MYCO_TAKEOVER": "myco_resume_secret", "MYCO_CONFIG": filepath.Join(dir, "bad.yaml")}}
	if code, _, e := bad.run("join"); code != ExitUsage || strings.Contains(e, "secret") {
		t.Fatalf("bad link: %d %s", code, e)
	}
	nw := &harnessT{t, map[string]string{"MYCO_TAKEOVER": link.URL, "MYCO_CONFIG": filepath.Join(dir, "new.yaml")}}
	code, out, e = nw.run("join", "--harness", "inbox", "--format", "text")
	if code != 0 || !strings.Contains(out, "took over docs") || !strings.Contains(out, "you now are the agent docs") || !strings.Contains(out, "moving accounts") {
		t.Fatalf("take over: %d %s %s", code, out, e)
	}
	if code, out, e := nw.run("handover", "show", "--format", "text"); code != 0 || !strings.Contains(out, "Handover") {
		t.Fatalf("handover show: %d %s %s", code, out, e)
	}
	if code, out, e := nw.run("agent", "get", "--format", "json"); code != 0 || !strings.Contains(out, `"name": "docs"`) {
		t.Fatalf("new identity: %d %s %s", code, out, e)
	}
	code, _, e = old.run("inbox", "pull")
	if code != ExitReplaced || !strings.Contains(e, "replaced") || !strings.Contains(e, "taken over") {
		t.Fatalf("old session: %d %s", code, e)
	}
	code, out, e = admin.run("agent", "handover-list", "--name", "docs", "--format", "text")
	if code != 0 || !strings.Contains(out, "done") || !strings.Contains(out, "inbox") {
		t.Fatalf("list: %d %s %s", code, out, e)
	}
	// Without a machine or --back, handover says what to do.
	if code, _, e := admin.run("agent", "handover", "--name", "docs"); code != ExitUsage || !strings.Contains(e, "handover-link") {
		t.Fatalf("handover without machine: %d %s", code, e)
	}
}
