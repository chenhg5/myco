package claude

import (
	"encoding/json"
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

// TodoWrite's plan reaches the trace as a checklist, each item with its
// status, so the app can show it being ticked off.
func TestTodoWriteIsAChecklist(t *testing.T) {
	in := json.RawMessage(`{"todos":[
		{"content":"读代码","status":"completed","activeForm":"读代码中"},
		{"content":"改接口","status":"in_progress","activeForm":"改接口中"},
		{"content":"跑测试","status":"pending","activeForm":"跑测试中"}]}`)
	got := toolInput("TodoWrite", in)
	want := "[x] 读代码\n[~] 改接口\n[ ] 跑测试"
	if got != want {
		t.Fatalf("TodoWrite input = %q, want %q", got, want)
	}
}

func TestLiveParserDeltas(t *testing.T) {
	lines := []string{
		`{"type":"stream_event","event":{"type":"message_start","message":{"id":"m1"}}}`,
		`{"type":"stream_event","event":{"type":"content_block_start","index":0,"content_block":{"type":"thinking"}}}`,
		`{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"look "}}}`,
		`{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"x"}}}`,
		`{"type":"stream_event","event":{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"Hi"}}}`,
		`{"type":"assistant","message":{"id":"m1","content":[{"type":"text","text":"Hi there"}]}}`,
	}
	var p liveParser
	var got []harness.Event
	for _, l := range lines {
		got = append(got, p.events([]byte(l))...)
	}
	want := []harness.Event{
		{Kind: "delta", Stream: "thinking", ID: "m1/0", Text: "look "},
		{Kind: "delta", Stream: "text", ID: "m1/1", Text: "Hi"},
		{Kind: "text", Text: "Hi there"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

// In a session file, the assistant message that ends a turn is followed by
// a turn_end step; one that calls a tool is not.
func TestTranscriptTurnEnd(t *testing.T) {
	end := TranscriptEvents([]byte(`{"type":"assistant","message":{"id":"m1","stop_reason":"end_turn","content":[{"type":"text","text":"Waiting for the review."}]}}`))
	if len(end) != 2 || end[0].Kind != "text" || end[1].Kind != "turn_end" {
		t.Fatalf("end of turn: %+v", end)
	}
	mid := TranscriptEvents([]byte(`{"type":"assistant","message":{"id":"m2","stop_reason":"tool_use","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"}}]}}`))
	if len(mid) != 1 || mid[0].Kind != "tool" {
		t.Fatalf("a tool call is not the end: %+v", mid)
	}
	// stream-json (a runner) has its own end: the result line.
	if evs := lineEvents([]byte(`{"type":"assistant","message":{"stop_reason":"end_turn","content":[{"type":"text","text":"x"}]}}`)); len(evs) != 1 {
		t.Fatalf("lineEvents adds no turn_end: %+v", evs)
	}
}
