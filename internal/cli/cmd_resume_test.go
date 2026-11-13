package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestResumeAutoFromURL(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())
	hub := newHub(t)
	old := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(t.TempDir(), "old.yaml")}}
	if code, _, e := old.run("agent", "register", "--name", "old-agent"); code != 0 {
		t.Fatalf("register: %s", e)
	}
	code, out, e := old.run("agent", "resume-link", "--format", "json")
	if code != 0 {
		t.Fatalf("link: %s", e)
	}
	var link struct{ Token, URL, Command string }
	if err := json.Unmarshal([]byte(out), &link); err != nil || link.Token == "" {
		t.Fatalf("link: %s %v", out, err)
	}
	if code, _, e := old.run("agent", "resume-link", "--dry-run"); code != ExitDryRun {
		t.Fatalf("dry-run: %d %s", code, e)
	}
	fresh := &harnessT{t, map[string]string{"MYCO_RESUME": link.URL, "MYCO_CONFIG": filepath.Join(t.TempDir(), "fresh.yaml")}}
	code, out, e = fresh.run("agent", "get", "--format", "json")
	if code != 0 || !strings.Contains(out, `"name": "old-agent"`) {
		t.Fatalf("auto resume: %d %s %s", code, out, e)
	}
	// A second command uses the saved login token; it need not exchange the
	// reusable recovery credential again.
	code, out, e = fresh.run("agent", "get", "--format", "json")
	if code != 0 || !strings.Contains(out, `"name": "old-agent"`) {
		t.Fatalf("saved identity: %d %s %s", code, out, e)
	}
	if code, _, e := old.run("agent", "get"); code != ExitPermission {
		t.Fatalf("old login must be invalid: %d %s", code, e)
	}
	if code, _, e := fresh.run("agent", "resume-link", "--revoke"); code != 0 {
		t.Fatalf("revoke: %s", e)
	}
	third := &harnessT{t, map[string]string{"MYCO_RESUME": link.URL, "MYCO_CONFIG": filepath.Join(t.TempDir(), "third.yaml")}}
	if code, _, _ := third.run("agent", "get"); code != ExitPermission {
		t.Fatalf("revoked link: exit %d", code)
	}
}

func TestResumeTokenParserRedactsInvalidInput(t *testing.T) {
	_, _, err := resumeTokenAndHub("https://hub.example/r/not-a-token?secret=abc")
	if err == nil || strings.Contains(err.Error(), "secret=abc") {
		t.Fatalf("credential leaked in error: %v", err)
	}
	tok, hub, err := resumeTokenAndHub("https://hub.example/r/myco_resume_abc")
	if err != nil || tok != "myco_resume_abc" || hub != "https://hub.example" {
		t.Fatalf("parse: %q %q %v", tok, hub, err)
	}
}
