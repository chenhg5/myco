package codex

import (
	"strings"
	"testing"
)

func TestParseEventsPrefersTurnFailure(t *testing.T) {
	// Real output from a Codex run with an unsupported model.
	stdout := strings.Join([]string{
		`{"type":"thread.started","thread_id":"01a1"}`,
		`{"type":"item.completed","item":{"id":"item_0","type":"error","message":"Model metadata for ` + "`gpt-6.1-sol`" + ` not found."}}`,
		`{"type":"turn.started"}`,
		`{"type":"error","message":"{\"type\":\"error\",\"status\":400,\"error\":{\"type\":\"invalid_request_error\",\"message\":\"The 'gpt-6.1-sol' model is not supported when using Codex with a ChatGPT account.\",\"code\":\"model_not_found\"}}"}`,
		`{"type":"turn.failed","error":{"message":"{\"type\":\"error\",\"status\":400,\"error\":{\"type\":\"invalid_request_error\",\"message\":\"The 'gpt-6.1-sol' model is not supported when using Codex with a ChatGPT account.\",\"code\":\"model_not_found\"}}"}}`,
	}, "\n")
	res, failure := parseEvents(stdout)
	if res.SessionID != "01a1" {
		t.Fatalf("session: %+v", res)
	}
	want := "The 'gpt-6.1-sol' model is not supported when using Codex with a ChatGPT account. (model_not_found)"
	if failure != want {
		t.Fatalf("failure:\n got %q\nwant %q", failure, want)
	}
}

func TestParseEventsOutput(t *testing.T) {
	res, failure := parseEvents(`{"type":"item.completed","item":{"type":"agent_message","text":"hello"}}`)
	if res.Output != "hello" || failure != "" {
		t.Fatalf("%+v %q", res, failure)
	}
}
