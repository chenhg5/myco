package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestInboxHintOnlyForTasks(t *testing.T) {
	hub := newHub(t)
	dir := t.TempDir()
	alice := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(dir, "alice.yaml")}}
	bob := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(dir, "bob.yaml")}}
	for _, h := range []struct {
		cli  *harnessT
		name string
	}{{alice, "alice"}, {bob, "bob"}} {
		if code, _, errOut := h.cli.run("agent", "register", "--name", h.name); code != 0 {
			t.Fatalf("register %s: %s", h.name, errOut)
		}
	}
	send := func(kind string) {
		t.Helper()
		if code, _, errOut := alice.run("message", "send", "--to", "bob", "--body", "hello", "--type", kind); code != 0 {
			t.Fatalf("send %s: %s", kind, errOut)
		}
	}
	checkJSON := func(command string, wantHint bool) {
		t.Helper()
		args := []string{"inbox", command, "--format", "json"}
		if command == "wait" {
			args = append(args, "--timeout", "1s")
		}
		code, out, errOut := bob.run(args...)
		if code != 0 {
			t.Fatalf("%s json: %s", command, errOut)
		}
		var result struct {
			Messages []json.RawMessage `json:"messages"`
			Hint     string            `json:"hint"`
		}
		if err := json.Unmarshal([]byte(out), &result); err != nil || len(result.Messages) == 0 {
			t.Fatalf("%s json: %s (%v)", command, out, err)
		}
		if wantHint != (result.Hint != "") || (wantHint && (!strings.Contains(result.Hint, "message ack") || !strings.Contains(result.Hint, "message progress") || !strings.Contains(result.Hint, "message reply"))) {
			t.Fatalf("%s json hint: %s", command, out)
		}
	}
	checkText := func(command string, wantHint bool) {
		t.Helper()
		args := []string{"inbox", command, "--format", "text"}
		if command == "wait" {
			args = append(args, "--timeout", "1s")
		}
		code, out, errOut := bob.run(args...)
		if code != 0 || strings.Contains(out, "message progress") != wantHint {
			t.Fatalf("%s text: code=%d out=%s err=%s", command, code, out, errOut)
		}
	}

	send("task")
	checkJSON("wait", true)
	checkText("wait", true)
	checkJSON("pull", true)
	send("task")
	checkText("pull", true)

	send("note")
	checkJSON("wait", false)
	checkText("wait", false)
	checkJSON("pull", false)
	send("note")
	checkText("pull", false)
	if code, out, errOut := bob.run("inbox", "pull", "--format", "json"); code != 0 || strings.Contains(out, `"hint"`) {
		t.Fatalf("empty pull hint: code=%d out=%s err=%s", code, out, errOut)
	}
}
