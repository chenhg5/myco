package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/config"
	"github.com/chenhg5/myco/internal/testutil"
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
	dryPath := filepath.Join(t.TempDir(), "dry.yaml")
	dryCLI := &harnessT{t, map[string]string{"MYCO_CONFIG": dryPath}}
	if code, _, e := dryCLI.run("agent", "resume", "--token", link.URL, "--dry-run"); code != ExitDryRun {
		t.Fatalf("resume dry-run: %d %s", code, e)
	}
	if _, err := os.Stat(dryPath); !os.IsNotExist(err) {
		t.Fatalf("resume dry-run wrote config: %v", err)
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
	tokenOnly := &harnessT{t, map[string]string{"MYCO_RESUME": link.Token, "MYCO_HUB": hub, "MYCO_CONFIG": filepath.Join(t.TempDir(), "token-only.yaml")}}
	if code, out, e := tokenOnly.run("agent", "get"); code != 0 || !strings.Contains(out, "old-agent") {
		t.Fatalf("token-only auto resume: %d %s %s", code, out, e)
	}
	if code, _, e := tokenOnly.run("agent", "resume-link", "--revoke"); code != 0 {
		t.Fatalf("revoke: %s", e)
	}
	third := &harnessT{t, map[string]string{"MYCO_RESUME": link.URL, "MYCO_CONFIG": filepath.Join(t.TempDir(), "third.yaml")}}
	if code, _, _ := third.run("agent", "get"); code != ExitPermission {
		t.Fatalf("revoked link: exit %d", code)
	}
}

func TestResumeAutoWithoutConfig(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	hub := newHub(t)
	old := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(t.TempDir(), "old.yaml")}}
	if code, _, e := old.run("agent", "register", "--name", "sandbox-agent"); code != 0 {
		t.Fatalf("register: %s", e)
	}
	code, out, e := old.run("agent", "resume-link")
	if code != 0 {
		t.Fatalf("resume link: %s", e)
	}
	var link struct{ URL string }
	if err := json.Unmarshal([]byte(out), &link); err != nil {
		t.Fatal(err)
	}
	fresh := &harnessT{t, map[string]string{"MYCO_RESUME": link.URL}}
	code, out, e = fresh.run("agent", "get")
	if code != 0 || !strings.Contains(out, "sandbox-agent") {
		t.Fatalf("resume from empty home: %d %s %s", code, out, e)
	}
	cfg, err := config.Load(filepath.Join(home, ".myco", "config.yaml"))
	if err != nil || cfg.Agent != "sandbox-agent" || cfg.Token == "" {
		t.Fatalf("saved identity: %+v %v", cfg.Redacted(), err)
	}
}

func TestEnrollOnOldHubSkipsResumeLink(t *testing.T) {
	newHubURL := newHub(t)
	admin, _ := client.New(newHubURL, "admin-token-123456")
	enr, err := admin.CreateEnrollment(context.Background(), api.CreateEnrollmentRequest{NamePrefix: "old-"}, false)
	if err != nil {
		t.Fatal(err)
	}
	backend, _ := url.Parse(newHubURL)
	proxy := httputil.NewSingleHostReverseProxy(backend)
	old := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/me/resume" {
			http.NotFound(w, r)
			return
		}
		proxy.ServeHTTP(w, r)
	}))
	defer old.Close()
	cli := &harnessT{t, map[string]string{"MYCO_HUB": old.URL, "MYCO_ENROLL_TOKEN": enr.Token, "MYCO_CONFIG": filepath.Join(t.TempDir(), "old.yaml")}}
	code, out, stderr := cli.run("agent", "enroll", "--harness", "inbox", "--session-id", "old-session", "--format", "json")
	if code != 0 || strings.Contains(out, "resume_command") {
		t.Fatalf("old hub enrollment: %d %s %s", code, out, stderr)
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

func TestResumeCommandSchema(t *testing.T) {
	cli := &harnessT{t, map[string]string{"MYCO_CONFIG": filepath.Join(t.TempDir(), "config.yaml")}}
	if code, out, e := cli.run("schema", "agent", "resume"); code != 0 || !strings.Contains(out, `"token"`) || !strings.Contains(out, `"dry-run"`) {
		t.Fatalf("resume schema: %d %s %s", code, out, e)
	}
	if code, out, e := cli.run("schema", "agent", "resume-link"); code != 0 || !strings.Contains(out, `"revoke"`) {
		t.Fatalf("resume-link schema: %d %s %s", code, out, e)
	}
}
