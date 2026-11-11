package codex

import (
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
	s := newEventStream(time.Now())
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
