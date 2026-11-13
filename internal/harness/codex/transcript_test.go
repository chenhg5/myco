package codex

import (
	"os"
	"path/filepath"
	"testing"
)

// Lines shaped as a Codex session file records them (Codex app, Nov 2026).
func TestTranscriptEvents(t *testing.T) {
	cases := []struct {
		line, kind, tool, text string
	}{
		{`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"I will read the handler first."}],"phase":"commentary"}}`, "text", "", "I will read the handler first."},
		{`{"type":"response_item","payload":{"type":"reasoning","summary":[{"type":"summary_text","text":"Checking the lease"}]}}`, "thinking", "", "Checking the lease"},
		{`{"type":"response_item","payload":{"type":"custom_tool_call","call_id":"c1","name":"exec","input":"const r=await Promise.allSettled([tools.exec_command({cmd:\"go test ./...\",\"max_output_tokens\":3500}),tools.exec_command({cmd:\"git status\"})]); for(const x of r)text(x);"}}`, "tool", "Bash", "go test ./...\ngit status"},
		{`{"type":"response_item","payload":{"type":"custom_tool_call","call_id":"c2","name":"exec","input":"text(await tools.web__run({open:[]}))"}}`, "tool", "exec", "text(await tools.web__run({open:[]}))"},
		{"{\"type\":\"response_item\",\"payload\":{\"type\":\"custom_tool_call\",\"call_id\":\"c4\",\"name\":\"exec\",\"input\":\"const r=await tools.exec_command({cmd:`gcloud sql instances list --format='table(name)'`});\"}}", "tool", "Bash", "gcloud sql instances list --format='table(name)'"},
		{`{"type":"response_item","payload":{"type":"custom_tool_call","call_id":"c5","name":"exec","input":"text(await tools.write_stdin({session_id:97094,chars:\"\",yield_time_ms:5000}))"}}`, "tool", "Bash", "(waiting for the output of a running command)"},
		{`{"type":"response_item","payload":{"type":"function_call","call_id":"c6","name":"update_plan","arguments":"{\"plan\":[{\"step\":\"Read the handler\",\"status\":\"completed\"},{\"step\":\"Fix the check\",\"status\":\"in_progress\"},{\"step\":\"Run the tests\",\"status\":\"pending\"}]}"}}`, "tool", "TodoWrite", "[x] Read the handler\n[~] Fix the check\n[ ] Run the tests"},
		{`{"type":"response_item","payload":{"type":"function_call","call_id":"c3","name":"spawn_agent","namespace":"collaboration","arguments":"{\"task_name\":\"brain\"}"}}`, "tool", "collaboration.spawn_agent", `{"task_name":"brain"}`},
		{`{"type":"response_item","payload":{"type":"custom_tool_call_output","call_id":"c1","output":[{"type":"input_text","text":"Script completed\n"},{"type":"input_text","text":"ok"}]}}`, "tool_result", "", "Script completed\nok"},
		{`{"type":"response_item","payload":{"type":"function_call_output","call_id":"c3","output":"{\"task_name\":\"/root/brain\"}"}}`, "tool_result", "", `{"task_name":"/root/brain"}`},
	}
	for _, c := range cases {
		evs := TranscriptEvents([]byte(c.line))
		if len(evs) != 1 {
			t.Fatalf("%s: %+v", c.line, evs)
		}
		ev := evs[0]
		got := ev.Text
		if ev.Kind == "tool" {
			got = ev.Input
		} else if ev.Kind == "tool_result" {
			got = ev.Output
		}
		if ev.Kind != c.kind || ev.Tool != c.tool || got != c.text {
			t.Fatalf("%s:\n got %+v", c.line, ev)
		}
	}
	for _, skip := range []string{
		`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"the task"}]}}`,
		`{"type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"instructions"}]}}`,
		`{"type":"response_item","payload":{"type":"reasoning","summary":[]}}`,
		`{"type":"event_msg","payload":{"type":"token_count"}}`,
		`not json`,
	} {
		if evs := TranscriptEvents([]byte(skip)); len(evs) != 0 {
			t.Fatalf("%s should be no step: %+v", skip, evs)
		}
	}
}

func TestTranscriptPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	dir := filepath.Join(home, "sessions", "2026", "11", "14")
	_ = os.MkdirAll(dir, 0o755)
	f := filepath.Join(dir, "rollout-2026-11-14T08-00-00-01a1-thread.jsonl")
	_ = os.WriteFile(f, nil, 0o600)
	if got := TranscriptPath("01a1-thread"); got != f {
		t.Fatalf("path: %q", got)
	}
	if TranscriptPath("") != "" || TranscriptPath("other") != "" {
		t.Fatal("no file, no path")
	}
}
