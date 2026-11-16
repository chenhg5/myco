package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A conversation exported: Markdown to stdout, JSON Lines to a file (with a
// short result), a dry run that writes nothing, and a bad format refused.
func TestThreadExport(t *testing.T) {
	hub := newHub(t)
	dir := t.TempDir()
	env := func(cfg string) map[string]string {
		return map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(dir, cfg)}
	}
	alice, bob := &harnessT{t, env("a.yaml")}, &harnessT{t, env("b.yaml")}
	for _, a := range []struct {
		h    *harnessT
		name string
	}{{alice, "alice"}, {bob, "bob"}} {
		if code, _, e := a.h.run("agent", "register", "--name", a.name); code != 0 {
			t.Fatalf("register %s: %d %s", a.name, code, e)
		}
	}
	code, out, e := alice.run("message", "send", "--to", "bob", "--body", "跑一遍测试", "--format", "json")
	if code != 0 {
		t.Fatalf("send: %d %s", code, e)
	}
	var sent struct{ Message struct{ ID string } }
	if err := json.Unmarshal([]byte(out), &sent); err != nil || sent.Message.ID == "" {
		t.Fatalf("send output: %s", out)
	}
	id := sent.Message.ID
	bob.run("inbox", "pull")
	bob.run("message", "ack", "--id", id)
	if code, _, e := bob.run("message", "reply", "--id", id, "--body", "全过了"); code != 0 {
		t.Fatalf("reply: %d %s", code, e)
	}

	code, md, e := alice.run("thread", "export", "--id", id)
	if code != 0 || !strings.HasPrefix(md, "# 对话导出\n") || !strings.Contains(md, "共 2 条消息") ||
		!strings.Contains(md, "**alice** → bob") || !strings.Contains(md, "· 任务\n\n跑一遍测试") || !strings.Contains(md, "· 回复\n\n全过了") {
		t.Fatalf("markdown: %d %s\n%s", code, e, md)
	}
	// Steps asked for where the hub keeps none: still exported.
	if code, _, e := alice.run("thread", "export", "--id", id, "--steps"); code != 0 {
		t.Fatalf("with --steps: %d %s", code, e)
	}

	file := filepath.Join(dir, "chat.jsonl")
	code, out, e = alice.run("thread", "export", "--id", id, "--as", "jsonl", "--output", file, "--format", "json")
	if code != 0 {
		t.Fatalf("jsonl: %d %s", code, e)
	}
	var res exportResult
	if json.Unmarshal([]byte(out), &res) != nil || res.Path != file || res.Messages != 2 || res.Format != "jsonl" {
		t.Fatalf("result: %s", out)
	}
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	var first map[string]any
	if len(lines) != 2 || json.Unmarshal([]byte(lines[0]), &first) != nil || first["id"] != id || first["body"] != "跑一遍测试" || first["type"] != "task" {
		t.Fatalf("jsonl file:\n%s", b)
	}

	// --dry-run says where and how much, and writes nothing.
	dry := filepath.Join(dir, "dry.md")
	code, out, _ = alice.run("thread", "export", "--id", id, "--output", dry, "--dry-run", "--format", "json")
	if code != ExitDryRun || !strings.Contains(out, `"messages": 2`) {
		t.Fatalf("dry run: %d %s", code, out)
	}
	if _, err := os.Stat(dry); !os.IsNotExist(err) {
		t.Fatal("a dry run must not write the file")
	}
	if code, _, _ := alice.run("thread", "export", "--id", id, "--as", "html"); code != ExitUsage {
		t.Fatalf("bad format: %d", code)
	}
}
