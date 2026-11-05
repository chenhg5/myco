package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
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
	h, closeFn, err := hubserver.Build(hubserver.Options{DBPath: ":memory:", AdminToken: "admin-token-123456",
		Policy: app.DefaultMessagePolicy(), OnlineWindow: time.Minute, Log: newLogger("error")})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(func() { srv.Close(); closeFn() })
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
