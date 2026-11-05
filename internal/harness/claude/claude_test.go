package claude

import (
	"strings"
	"testing"
)

func TestParseStream(t *testing.T) {
	out := strings.Join([]string{
		`{"type":"system","subtype":"init","session_id":"s-1"}`,
		`{"type":"assistant","session_id":"s-1","message":{"content":[{"type":"text","text":"partial answer"}]}}`,
		`{"type":"result","subtype":"success","is_error":false,"result":"PONG","session_id":"s-1"}`,
	}, "\n")
	res, final, ok := parseStream(out)
	if !ok || final.IsError || res.Output != "PONG" || res.SessionID != "s-1" {
		t.Fatalf("%+v %+v %v", res, final, ok)
	}
	// Killed before the result event: fall back to the last assistant text.
	res, _, ok = parseStream(strings.Join(strings.Split(out, "\n")[:2], "\n"))
	if ok || res.Output != "partial answer" || res.SessionID != "s-1" {
		t.Fatalf("fallback: %+v %v", res, ok)
	}
}

func TestInheritedSessionEnv(t *testing.T) {
	if got := inheritedSessionEnv([]string{"CLAUDE_CONFIG_DIR=/x", "CLAUDE_CODE_SESSION_ID=a"}); got != nil {
		t.Fatalf("not nested: nothing to strip, got %v", got)
	}
	got := inheritedSessionEnv([]string{"CLAUDECODE=1", "CLAUDE_CODE_SESSION_ID=a", "CLAUDE_CODE_REMOTE=true",
		"CLAUDE_CONFIG_DIR=/x", "CLAUDE_CODE_OAUTH_TOKEN=t", "PATH=/bin"})
	want := map[string]bool{"CLAUDECODE": true, "CLAUDE_CODE_SESSION_ID": true, "CLAUDE_CODE_REMOTE": true}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for _, k := range got {
		if !want[k] {
			t.Fatalf("unexpected %s in %v", k, got)
		}
	}
}
