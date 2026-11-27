package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/harness"
)

func TestTimedRunKeepsFailedAndSuccessfulInvocationsSeparate(t *testing.T) {
	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, nil))
	calls := 0
	r := &Runner{harness: fakeHarness{fn: func(req harness.Request) (harness.Result, error) {
		calls++
		req.OnEvent(harness.Event{Kind: "text", Text: "private-response"})
		req.OnEvent(harness.Event{Kind: "text", Text: "private-response"})
		if calls == 1 {
			return harness.Result{}, errors.New("private-error")
		}
		return harness.Result{}, nil
	}}}
	for i := 1; i <= 2; i++ {
		_, _ = r.timedRun(context.Background(), harness.Request{}, log, time.Now(), i)
	}
	first := map[int]int{}
	ended := map[int]bool{}
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		var event struct {
			Stage      string `json:"stage"`
			Invocation int    `json:"invocation"`
			Succeeded  bool   `json:"succeeded"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event.Stage == "first_text" {
			first[event.Invocation]++
		}
		if event.Stage == "harness_returned" {
			ended[event.Invocation] = event.Succeeded
		}
	}
	if first[1] != 1 || first[2] != 1 || len(ended) != 2 || ended[1] || !ended[2] || strings.Contains(logs.String(), "private-") {
		t.Fatal("attempt correlation, success state or content-free logging failed")
	}
}

func TestTraceCoalescingKeepsFirstSourceObservation(t *testing.T) {
	hub := &traceHub{fakeHub: newFakeHub()}
	tr := newTracer(context.Background(), hub, "timed", TraceFull, quiet())
	tr.add(harness.Event{Kind: "delta", Stream: "text", ID: "block", Text: "a", ObservedAtMS: 1234})
	tr.add(harness.Event{Kind: "delta", Stream: "text", ID: "block", Text: "b", ObservedAtMS: 5678})
	tr.close()
	if len(hub.events) != 1 {
		t.Fatalf("events=%d", len(hub.events))
	}
	var ev harness.Event
	if err := json.Unmarshal(hub.events[0].Data, &ev); err != nil {
		t.Fatal(err)
	}
	if ev.ObservedAtMS != 1234 || ev.Text != "ab" {
		t.Fatalf("source observation changed: %+v", ev)
	}
}

func TestRunLatencyLogsNoContentAndOnlyOneFirstText(t *testing.T) {
	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, nil))
	hub := newFakeHub(api.Message{ID: "timed", Type: "task", From: "alice", ThreadID: "thread", Body: "private-prompt"})
	h := fakeHarness{fn: func(req harness.Request) (harness.Result, error) {
		req.OnTiming("process_started")
		req.OnEvent(harness.Event{Kind: "delta", Stream: "thinking", Text: "private-thought"})
		req.OnEvent(harness.Event{Kind: "delta", Stream: "text", Text: "private-response"})
		req.OnEvent(harness.Event{Kind: "text", Text: "private-response"})
		return harness.Result{Output: "private-response", SessionID: "fixture-session"}, nil
	}}
	r := New(hub, h, NewHubSessions(&fakeBinder{}, ""), nil, Options{Once: true, Concurrency: 1, Trace: TraceOff}, log)
	if err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	text := logs.String()
	if strings.Contains(text, "private-") || strings.Count(text, `"stage":"first_text"`) != 1 {
		t.Fatal("timing logs included content or duplicate first-text milestones")
	}
	for _, phase := range []string{"runner_handle", "ack_done", "context_ready", "request_ready", "process_started", "harness_returned"} {
		if !strings.Contains(text, `"stage":"`+phase+`"`) {
			t.Fatalf("missing phase %s", phase)
		}
	}
}
