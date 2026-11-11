package codex

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/harness"
)

func TestCodexEvents(t *testing.T) {
	lines := []string{
		`{"type":"thread.started","thread_id":"th"}`,
		`{"type":"turn.started"}`,
		`{"type":"item.completed","item":{"id":"r1","type":"reasoning","text":"look at the failing test"}}`,
		`{"type":"item.started","item":{"id":"c1","type":"command_execution","command":"bash -lc 'make test'","aggregated_output":"","exit_code":null,"status":"in_progress"}}`,
		`{"type":"item.completed","item":{"id":"c1","type":"command_execution","command":"bash -lc 'make test'","aggregated_output":"FAIL","exit_code":2,"status":"failed"}}`,
		`{"type":"item.completed","item":{"id":"f1","type":"file_change","changes":[{"path":"x.go","kind":"update"}],"status":"completed"}}`,
		`{"type":"item.completed","item":{"id":"m1","type":"agent_message","text":"Fixed it."}}`,
		`{"type":"turn.completed","usage":{"input_tokens":1200,"cached_input_tokens":800,"output_tokens":90}}`,
	}
	s := newEventStream(time.Now(), tokenTotal{}, true)
	var got []harness.Event
	for _, l := range lines {
		got = append(got, s.line([]byte(l))...)
	}
	want := []harness.Event{
		{Kind: "thinking", Text: "look at the failing test"},
		{Kind: "tool", Tool: "Bash", ID: "c1", Input: "make test"},
		{Kind: "tool_result", ID: "c1", Output: "FAIL", Failed: true},
		{Kind: "tool", Tool: "Edit", ID: "f1", Input: "x.go"},
		{Kind: "tool_result", ID: "f1", Output: "update x.go"},
		{Kind: "text", Text: "Fixed it."},
	}
	if len(got) != len(want)+1 {
		t.Fatalf("got %d events: %+v", len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
	if u := got[len(got)-1]; u.Kind != "usage" || u.InputTokens != 1200 || u.OutputTokens != 90 {
		t.Errorf("usage: %+v", u)
	}
}

// Codex reports the session's running token total: a run shows what it
// added, and nothing when the total before it is not known.
func TestUsageIsTheRunsShare(t *testing.T) {
	line := []byte(`{"type":"turn.completed","usage":{"input_tokens":317844980,"cached_input_tokens":309750144,"output_tokens":512936}}`)
	s := newEventStream(time.Now(), tokenTotal{Input: 317448000, Output: 512500}, true)
	ev := s.line(line)
	if len(ev) != 1 || ev[0].InputTokens != 396980 || ev[0].OutputTokens != 436 {
		t.Fatalf("%+v", ev)
	}
	s = newEventStream(time.Now(), tokenTotal{}, false)
	if ev := s.line(line); len(ev) != 1 || ev[0].InputTokens != 0 || ev[0].OutputTokens != 0 || ev[0].Kind != "usage" {
		t.Fatalf("unknown total: %+v", ev)
	}
}

func TestSessionTokensFromTheRolloutFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	dir := filepath.Join(home, "sessions", "2026", "11", "10")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	id := "01a1c94f-8baa-7b70-bedc-81a6e07f001c"
	body := `{"type":"session_meta","payload":{"id":"` + id + `"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":100,"output_tokens":10}}}}
{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":250,"output_tokens":30},"last_token_usage":{"input_tokens":150}}}}
`
	if err := os.WriteFile(filepath.Join(dir, "rollout-2026-11-10T22-33-15-"+id+".jsonl"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, ok := sessionTokens(id); !ok || got.Input != 250 || got.Output != 30 {
		t.Fatalf("%+v %v", got, ok)
	}
	if _, ok := sessionTokens("missing"); ok {
		t.Fatal("a session without a file is not known")
	}
}
