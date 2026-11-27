package harness

import (
	"reflect"
	"testing"
)

func TestProcessTimingDistinguishesSessionAndModel(t *testing.T) {
	for _, tc := range []struct {
		cli   string
		lines []string
	}{
		{"codex", []string{`not json`, `{"type":"thread.started"}`, `{"type":"turn.started"}`, `{"type":"item.completed","item":{"type":"reasoning","text":"private"}}`, `{"type":"item.completed","item":{"type":"agent_message","text":"secret"}}`}},
		{"claude", []string{`not json`, `{"type":"system","subtype":"init"}`, `{"type":"system","subtype":"notice"}`, `{"type":"stream_event","event":{"type":"message_start"}}`, `{"type":"assistant","message":{"content":[{"type":"text","text":"secret"}]}}`}},
	} {
		t.Run(tc.cli, func(t *testing.T) {
			var stages []string
			forwarded, started := 0, false
			p := Process{OnStarted: func() { started = true }, OnStdoutLine: func([]byte) { forwarded++ }}
			ObserveProcess(&p, Request{OnTiming: func(s string) { stages = append(stages, s) }}, tc.cli)
			p.OnStarted()
			for i, line := range tc.lines {
				p.OnStdoutLine([]byte(line))
				if i == 2 && len(stages) != 3 {
					t.Fatal("session metadata was classified as model work")
				}
			}
			want := []string{"process_started", "first_cli_event", "session_announced", "first_model_event"}
			if !reflect.DeepEqual(stages, want) || !started || forwarded != len(tc.lines) {
				t.Fatalf("milestones=%v started=%v forwarded=%d", stages, started, forwarded)
			}
		})
	}
}
