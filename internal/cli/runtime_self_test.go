package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInboxWaitReportsTheSessionsModel(t *testing.T) {
	hub := newHub(t)
	dir := t.TempDir()
	h := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(dir, "agent.yaml")}}
	if code, _, e := h.run("agent", "register", "--name", "self"); code != 0 {
		t.Fatalf("register: %s", e)
	}
	// A Claude Code session: its transcript (the latest real reply's model)
	// and its effort.
	claude := filepath.Join(dir, "claude")
	proj := filepath.Join(claude, "projects", "-work")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	lines := `{"type":"user","message":{"content":"hi"}}
{"type":"assistant","message":{"model":"claude-old-1","content":[]}}
{"type":"assistant","message":{"model":"claude-new-2","content":[]}}
{"type":"assistant","message":{"model":"<synthetic>","content":[]}}
`
	if err := os.WriteFile(filepath.Join(proj, "sess-1.jsonl"), []byte(lines), 0o600); err != nil {
		t.Fatal(err)
	}
	h.env["CLAUDE_CODE_SESSION_ID"] = "sess-1"
	h.env["CLAUDE_CONFIG_DIR"] = claude
	h.env["CLAUDE_EFFORT"] = "high"
	runtime := func() map[string]any {
		code, out, e := h.run("agent", "get", "--format", "json")
		if code != 0 {
			t.Fatalf("get: %s", e)
		}
		var a struct {
			Runtime map[string]any `json:"runtime"`
		}
		_ = json.Unmarshal([]byte(out), &a)
		return a.Runtime
	}
	h.run("inbox", "wait", "--timeout", "1ms")
	if rt := runtime(); rt["model"] != "claude-new-2" || rt["effort"] != "high" || rt["model_source"] != "self" || rt["cli"] != "claude" {
		t.Fatalf("reported: %v", rt)
	}
	// An agent its runner started does not report from here.
	h.env["MYCO_MESSAGE_ID"] = "msg_x"
	h.env["CLAUDE_EFFORT"] = "low"
	h.run("inbox", "wait", "--timeout", "1ms")
	if rt := runtime(); rt["effort"] != "high" {
		t.Fatalf("a runner's agent: %v", rt)
	}
	// Nothing to read: nothing reported, nothing lost.
	delete(h.env, "MYCO_MESSAGE_ID")
	h.env["CLAUDE_CODE_SESSION_ID"] = "elsewhere"
	h.run("inbox", "wait", "--timeout", "1ms")
	if rt := runtime(); rt["model"] != "claude-new-2" {
		t.Fatalf("kept: %v", rt)
	}
}

func TestLastCodexTurn(t *testing.T) {
	p := filepath.Join(t.TempDir(), "rollout-x.jsonl")
	_ = os.WriteFile(p, []byte(`{"type":"turn_context","payload":{"model":"gpt-a","effort":"low"}}
{"type":"response_item","payload":{}}
{"type":"turn_context","payload":{"model":"gpt-b","effort":"high"}}
{"type":"event_msg","payload":{}}
`), 0o600)
	if m, e := lastCodexTurn(p); m != "gpt-b" || e != "high" {
		t.Fatalf("%s %s", m, e)
	}
}
