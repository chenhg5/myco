package claude

import (
	"testing"

	"github.com/chenhg5/myco/internal/harness"
)

func TestLineEvents(t *testing.T) {
	lines := []string{
		`{"type":"system","subtype":"init","session_id":"s"}`,
		`{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"check the tests first"},{"type":"text","text":"Running the tests."},{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"make test","description":"run"}}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":[{"type":"text","text":"FAIL x_test.go"}],"is_error":true}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t2","name":"Read","input":{"file_path":"/repo/x.go"}}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t2","content":"package x"}]}}`,
		`{"type":"result","subtype":"success","result":"done","duration_ms":4200,"total_cost_usd":0.03,"usage":{"input_tokens":10,"cache_read_input_tokens":90,"output_tokens":40}}`,
		`not json`,
	}
	var got []harness.Event
	for _, l := range lines {
		got = append(got, lineEvents([]byte(l))...)
	}
	want := []harness.Event{
		{Kind: "thinking", Text: "check the tests first"},
		{Kind: "text", Text: "Running the tests."},
		{Kind: "tool", Tool: "Bash", ID: "t1", Input: "make test"},
		{Kind: "tool_result", ID: "t1", Output: "FAIL x_test.go", Failed: true},
		{Kind: "tool", Tool: "Read", ID: "t2", Input: "/repo/x.go"},
		{Kind: "tool_result", ID: "t2", Output: "package x"},
		{Kind: "usage", InputTokens: 100, OutputTokens: 40, CostUSD: 0.03, DurationMS: 4200},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d events: %+v", len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}
