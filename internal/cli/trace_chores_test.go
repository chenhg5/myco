package cli

import (
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/harness"
)

func TestIsChore(t *testing.T) {
	for in, want := range map[string]bool{
		"myco inbox wait --timeout 55m --format text":          true,
		"cd /home/user/myco && myco inbox wait --timeout 55m":  true,
		"myco inbox pull --format text 2>&1 | head -40":        true,
		"myco message ack --id msg_x >/dev/null 2>&1; echo ok": false, // echo is not a chore
		"myco message ack --id msg_x && myco inbox wait":       true,
		`bash -lc "myco inbox wait --timeout 55m"`:             true,
		"myco message reply --id msg_x --body done":            false,
		"myco message progress --id msg_x --body 'half way'":   false,
		"go test ./... && myco inbox wait":                     false,
		"":                                                     false,
		"cd /tmp":                                              false,
		"myco inbox":                                           false,
		"myco message done --id msg_y":                         true,
	} {
		if got := isChore(in); got != want {
			t.Errorf("isChore(%q) = %v, want %v", in, got, want)
		}
	}
}

func kindsOf(evs []harness.Event) string {
	var k []string
	for _, e := range evs {
		k = append(k, e.Kind)
	}
	return strings.Join(k, ",")
}

func gateAll(evs ...harness.Event) string {
	var g turnGate
	var out []harness.Event
	for _, e := range evs {
		out = append(out, g.add(e)...)
	}
	return kindsOf(out)
}

var (
	think   = harness.Event{Kind: "thinking", Text: "…"}
	say     = harness.Event{Kind: "text", Text: "still waiting"}
	end     = harness.Event{Kind: "turn_end"}
	waitCmd = harness.Event{Kind: "tool", Tool: "Bash", Input: "myco inbox wait --timeout 55m --format text"}
	waitRes = harness.Event{Kind: "tool_result", Output: "started"}
	work    = harness.Event{Kind: "tool", Tool: "Bash", Input: "go test ./..."}
	workRes = harness.Event{Kind: "tool_result", Output: "ok"}
)

func TestTurnGateFirstTurnPasses(t *testing.T) {
	// The turn that acknowledged the task is work, chores and all.
	if got := gateAll(think, waitCmd, waitRes, say, end); got != "thinking,tool,tool_result,text,turn_end" {
		t.Fatalf("first turn: %s", got)
	}
}

func TestTurnGateDropsAChoreTurn(t *testing.T) {
	// Its wait timed out: it starts it again and ends the turn. Nothing of it goes up.
	got := gateAll(work, workRes, say, end, think, waitCmd, waitRes, say, end)
	if got != "tool,tool_result,text,turn_end" {
		t.Fatalf("chore turn should be left out: %s", got)
	}
}

func TestTurnGateKeepsWorkAfterAChore(t *testing.T) {
	// Woken, it pulls and then works on the task again: all of it goes up,
	// from the start of the turn.
	got := gateAll(say, end, think, waitCmd, waitRes, work, workRes, say, end)
	if got != "text,turn_end,thinking,tool,tool_result,tool,tool_result,text,turn_end" {
		t.Fatalf("work after a chore: %s", got)
	}
}

func TestTurnGateKeepsATurnThatOnlySpeaks(t *testing.T) {
	got := gateAll(say, end, think, say, end)
	if got != "text,turn_end,thinking,text,turn_end" {
		t.Fatalf("a turn that only speaks goes up at its end: %s", got)
	}
}
