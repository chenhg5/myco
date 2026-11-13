package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/api"
)

func TestInboxSteerHint(t *testing.T) {
	hint := inboxTaskHint([]api.Message{{Type: "steer", ReplyTo: "msg_original"}})
	if !strings.Contains(hint, "reply_to") || !strings.Contains(hint, "not a new task") {
		t.Fatalf("steer hint missing: %q", hint)
	}
}

func TestReplyChecksUnreadSteersAndAnyway(t *testing.T) {
	hub := newHub(t)
	dir := t.TempDir()
	newAgent := func(name string) *harnessT {
		h := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(dir, name+".yaml")}}
		if code, _, err := h.run("agent", "register", "--name", name); code != 0 {
			t.Fatalf("register %s: %s", name, err)
		}
		return h
	}
	alice, bob := newAgent("alice"), newAgent("bob")
	code, out, errText := alice.run("message", "send", "--to", "bob", "--body", "work")
	if code != 0 {
		t.Fatalf("send: %s", errText)
	}
	var sent struct{ Message struct{ ID string } }
	if err := json.Unmarshal([]byte(out), &sent); err != nil {
		t.Fatal(err)
	}
	id := sent.Message.ID
	attachment := filepath.Join(dir, "correction.txt")
	if err := os.WriteFile(attachment, []byte("revised detail"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, out, e := alice.run("message", "steer", "--id", id, "--body", "include tests", "--attach", attachment); code != 0 || !strings.Contains(out, "correction.txt") {
		t.Fatalf("steer attachment: %d %s %s", code, out, e)
	}
	if code, _, e := bob.run("message", "reply", "--id", id, "--body", "done"); code != ExitConflict || !strings.Contains(e, "include tests") || !strings.Contains(e, "--anyway") {
		t.Fatalf("reply should stop for unread steer: %d %s", code, e)
	}
	if code, _, e := bob.run("inbox", "pull"); code != 0 {
		t.Fatalf("pull: %s", e)
	}
	if code, out, e := bob.run("message", "ack", "--id", id); code != 0 || !strings.Contains(out, "include tests") {
		t.Fatalf("ack should consume steer: %d %s %s", code, out, e)
	}
	if code, _, e := alice.run("message", "steer", "--id", id, "--body", "also update docs"); code != 0 {
		t.Fatalf("second steer: %s", e)
	}
	if code, out, e := bob.run("message", "progress", "--id", id, "--body", "testing", "--format", "text"); code != 0 || !strings.Contains(out, "also update docs") || !strings.Contains(out, "补充了") {
		t.Fatalf("progress should show and consume steer: %d %s %s", code, out, e)
	}
	if code, _, e := bob.run("message", "reply", "--id", id, "--body", "done"); code != 0 {
		t.Fatalf("reply after ack: %s", e)
	}
	code, out, _ = alice.run("message", "send", "--to", "bob", "--body", "another")
	_ = json.Unmarshal([]byte(out), &sent)
	id = sent.Message.ID
	alice.run("message", "steer", "--id", id, "--body", "ignore this")
	if code, _, e := bob.run("inbox", "pull"); code != 0 {
		t.Fatalf("pull another: %s", e)
	}
	if code, _, e := bob.run("message", "reply", "--id", id, "--body", "done", "--anyway"); code != 0 {
		t.Fatalf("--anyway: %s", e)
	}
}

func TestReplySkipsSteerCheckOnOldHub404(t *testing.T) {
	var replied bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/v1/messages/msg_old/steers" {
			http.NotFound(w, r)
			return
		}
		if r.Method == "POST" && r.URL.Path == "/v1/messages/msg_old/reply" {
			replied = true
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(201)
			_, _ = w.Write([]byte(`{"id":"reply_old","from":"bob","to":"alice","type":"reply","status":"done"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	h := &harnessT{t, map[string]string{"MYCO_HUB": srv.URL, "MYCO_TOKEN": "myco_x", "MYCO_CONFIG": filepath.Join(t.TempDir(), "c.yaml")}}
	if code, _, e := h.run("message", "reply", "--id", "msg_old", "--body", "done"); code != 0 || !replied {
		t.Fatalf("old Hub reply: %d %s", code, e)
	}
}
