package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestMessageStopCLI(t *testing.T) {
	hub := newHub(t)
	admin := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(t.TempDir(), "admin.yaml")}}
	token := func(name string) string {
		code, out, e := admin.run("agent", "register", "--name", name, "--no-save", "--format", "json")
		if code != 0 {
			t.Fatalf("register %s: %s", name, e)
		}
		var v struct{ Token string }
		if err := json.Unmarshal([]byte(out), &v); err != nil {
			t.Fatal(err)
		}
		return v.Token
	}
	alice := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_TOKEN": token("alice")}}
	bob := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_TOKEN": token("bob")}}
	code, out, e := alice.run("message", "send", "--to", "bob", "--body", "task", "--format", "json")
	if code != 0 {
		t.Fatalf("send: %s", e)
	}
	var sent struct{ Message struct{ ID string } }
	if err := json.Unmarshal([]byte(out), &sent); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := bob.run("message", "stop", "--id", sent.Message.ID); code != ExitPermission {
		t.Fatalf("other sender stop exit %d", code)
	}
	if code, _, _ := alice.run("message", "stop", "--id", sent.Message.ID, "--dry-run"); code != ExitDryRun {
		t.Fatalf("stop dry run exit %d", code)
	}
	if code, out, e := alice.run("message", "stop", "--id", sent.Message.ID); code != 0 || !strings.Contains(out, `"error": "stopped by the sender"`) {
		t.Fatalf("stop: %d %s %s", code, out, e)
	}
}
