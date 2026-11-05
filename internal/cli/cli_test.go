package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/hubserver"
)

type harnessT struct {
	t   *testing.T
	env map[string]string
}

func (h *harnessT) run(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := Run(context.Background(), args, strings.NewReader(""), &out, &errb, func(k string) string { return h.env[k] }, false)
	return code, out.String(), errb.String()
}

func newHub(t *testing.T) string {
	t.Helper()
	hub, err := hubserver.Build(hubserver.Options{DBPath: ":memory:", AdminToken: "admin-token-123456",
		Policy: app.DefaultMessagePolicy(), OnlineWindow: time.Minute, Log: newLogger("error")})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(hub.Handler)
	t.Cleanup(func() { srv.Close(); hub.Close() })
	return srv.URL
}

func TestCLIEndToEnd(t *testing.T) {
	hub := newHub(t)
	dir := t.TempDir()
	alice := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(dir, "a.yaml")}}
	bob := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(dir, "b.yaml")}}

	if code, _, e := alice.run("agent", "register", "--name", "alice"); code != 0 {
		t.Fatalf("register alice: %d %s", code, e)
	}
	if code, _, e := bob.run("agent", "register", "--name", "bob", "--capability", "code"); code != 0 {
		t.Fatalf("register bob: %d %s", code, e)
	}
	if code, _, _ := bob.run("agent", "register", "--name", "bob"); code != ExitConflict {
		t.Fatalf("duplicate register: want %d, got %d", ExitConflict, code)
	}

	code, out, _ := alice.run("message", "send", "--to", "bob", "--body", "hi")
	if code != 0 {
		t.Fatalf("send: %d", code)
	}
	var sent struct{ Message struct{ ID string } }
	if err := json.Unmarshal([]byte(out), &sent); err != nil || sent.Message.ID == "" {
		t.Fatalf("send output not JSON: %q", out)
	}

	code, out, _ = bob.run("inbox", "pull")
	if code != 0 || !strings.Contains(out, sent.Message.ID) {
		t.Fatalf("pull: %d %s", code, out)
	}
	if code, _, e := bob.run("message", "reply", "--id", sent.Message.ID, "--body", "pong"); code != 0 {
		t.Fatalf("reply: %d %s", code, e)
	}
	code, out, _ = alice.run("message", "wait", "--id", sent.Message.ID, "--timeout", "1s")
	if code != 0 || !strings.Contains(out, "pong") {
		t.Fatalf("wait: %d %s", code, out)
	}

	// Agent-friendliness contract.
	if code, _, _ := alice.run("message", "send", "--to", "bob", "--body", "x", "--dry-run"); code != ExitDryRun {
		t.Errorf("dry-run exit: %d", code)
	}
	code, _, e := alice.run("message", "send", "--too", "bob")
	if code != ExitUsage || !strings.Contains(e, `did you mean \"to\"`) {
		t.Errorf("unknown flag: %d %s", code, e)
	}
	if code, _, e := alice.run("message", "get", "--id", "msg_nope"); code != ExitNotFound || !strings.Contains(e, `"error":"not_found"`) {
		t.Errorf("not found: %d %s", code, e)
	}
	if code, _, _ := alice.run("message", "send", "--to", "bob", "--body", "x", "--type", "reply"); code != ExitUsage {
		t.Errorf("enum validation: %d", code)
	}
	if code, out, _ := alice.run("schema", "message", "send"); code != 0 || !strings.Contains(out, `"idempotency-key"`) {
		t.Errorf("schema: %d %s", code, out)
	}
	if code, out, _ := alice.run("--help"); code != 0 || !strings.Contains(out, "Exit codes") {
		t.Errorf("help: %d %s", code, out)
	}
}

func TestCLIAutoEnroll(t *testing.T) {
	hub := newHub(t)
	dir := t.TempDir()
	admin := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(dir, "admin.yaml")}}
	code, out, e := admin.run("enrollment", "create", "--name-prefix", "cloud-", "--harness", "claude-cloud", "--format", "json")
	if code != 0 {
		t.Fatalf("create enrollment: %d %s", code, e)
	}
	var created struct{ Token string }
	json.Unmarshal([]byte(out), &created)

	session := &harnessT{t, map[string]string{
		"MYCO_HUB": hub, "MYCO_ENROLL_TOKEN": created.Token, "MYCO_CONFIG": filepath.Join(dir, "s.yaml"),
		"CLAUDE_CODE_REMOTE_SESSION_ID": "cse_01AbCdEfGh12345678", "CLAUDE_CODE_REMOTE_ENVIRONMENT_TYPE": "cloud_default",
	}}
	// Any agent command auto-enrolls on first use.
	code, out, e = session.run("agent", "get", "--format", "json")
	if code != 0 || !strings.Contains(e, "enrolled as cloud-") || !strings.Contains(out, `"session_id": "session_01AbCdEfGh12345678"`) {
		t.Fatalf("auto-enroll: %d out=%s err=%s", code, out, e)
	}
	// Container restart: config lost, same session -> same agent.
	os.Remove(filepath.Join(dir, "s.yaml"))
	code, out2, _ := session.run("agent", "enroll", "--format", "json")
	if code != 0 || !strings.Contains(out2, `"created": false`) {
		t.Fatalf("recover: %d %s", code, out2)
	}
	// Enrollment token cannot administer the hub.
	session.env["MYCO_ADMIN_TOKEN"] = created.Token
	if code, _, _ := session.run("agent", "register", "--name", "evil"); code != ExitPermission {
		t.Fatalf("enroll token used as admin: exit %d", code)
	}
}
