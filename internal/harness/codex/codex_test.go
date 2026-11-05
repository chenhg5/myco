package codex

import (
	"strings"
	"testing"
)

func TestParseEvents(t *testing.T) {
	stdout := strings.Join([]string{
		`{"type":"thread.started","thread_id":"01a1"}`,
		`{"type":"item.completed","item":{"type":"agent_message","text":"hello"}}`,
		`not json`,
		`{"type":"error","message":"stream disconnected"}`,
		`{"type":"turn.failed","error":{"message":"model gpt-x not available"}}`,
	}, "\n")
	res, failure := parseEvents(stdout)
	if res.SessionID != "01a1" || res.Output != "hello" {
		t.Fatalf("result: %+v", res)
	}
	if failure != "stream disconnected; model gpt-x not available" {
		t.Fatalf("failure: %q", failure)
	}
}
