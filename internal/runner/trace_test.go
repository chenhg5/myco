package runner

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/harness"
)

type traceHub struct {
	*fakeHub
	mu       sync.Mutex
	events   []api.TraceEvent
	atReply  int // events uploaded when the reply came
	repliedT bool
}

func (h *traceHub) PostTrace(_ context.Context, _ string, events []api.TraceEvent) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, events...)
	return nil
}

func (h *traceHub) Reply(ctx context.Context, id, body string, dry bool) (*api.Message, error) {
	h.mu.Lock()
	h.atReply, h.repliedT = len(h.events), true
	h.mu.Unlock()
	return h.fakeHub.Reply(ctx, id, body, dry)
}

func runTraced(t *testing.T, mode string, msg api.Message) *traceHub {
	t.Helper()
	hub := &traceHub{fakeHub: newFakeHub(msg)}
	h := fakeHarness{fn: func(r harness.Request) (harness.Result, error) {
		if r.OnEvent != nil {
			r.OnEvent(harness.Event{Kind: "thinking", Text: "plan"})
			r.OnEvent(harness.Event{Kind: "tool", Tool: "Bash", ID: "1", Input: "make test"})
			r.OnEvent(harness.Event{Kind: "tool_result", ID: "1", Output: strings.Repeat("x", 9000)})
			r.OnEvent(harness.Event{Kind: "text", Text: "all green"})
		}
		return harness.Result{Output: "all green", SessionID: "s"}, nil
	}}
	r := New(hub, h, NewHubSessions(&fakeBinder{}, ""), nil, Options{Once: true, Concurrency: 1, Trace: mode}, quiet())
	r.Run(context.Background())
	return hub
}

func TestRunTraceUploaded(t *testing.T) {
	hub := runTraced(t, "", api.Message{ID: "t1", Type: "task", From: "alice", ThreadID: "th"})
	if !hub.repliedT || hub.atReply != 4 || len(hub.events) != 4 {
		t.Fatalf("all 4 steps must be on the hub before the reply: at reply %d, total %d", hub.atReply, len(hub.events))
	}
	for i, e := range hub.events {
		if e.Seq != i+1 {
			t.Fatalf("seq %d at %d", e.Seq, i)
		}
	}
	var res harness.Event
	json.Unmarshal(hub.events[2].Data, &res)
	if len(res.Output) > traceMaxOutput+40 || !strings.HasSuffix(res.Output, "(cut short)") {
		t.Fatalf("output not clipped: %d bytes", len(res.Output))
	}
}

func TestRunTraceSummaryAndOff(t *testing.T) {
	hub := runTraced(t, TraceSummary, api.Message{ID: "t1", Type: "task", From: "alice", ThreadID: "th"})
	for _, e := range hub.events {
		var ev harness.Event
		json.Unmarshal(e.Data, &ev)
		if ev.Text != "" || ev.Input != "" || ev.Output != "" {
			t.Fatalf("summary kept details: %+v", ev)
		}
	}
	if len(hub.events) != 4 || !strings.Contains(string(hub.events[1].Data), `"tool":"Bash"`) {
		t.Fatalf("summary should still say which tools ran: %s", hub.events[1].Data)
	}
	if hub := runTraced(t, TraceOff, api.Message{ID: "t1", Type: "task", From: "alice", ThreadID: "th"}); len(hub.events) != 0 {
		t.Fatalf("off uploaded %d steps", len(hub.events))
	}
	if hub := runTraced(t, "", api.Message{ID: "t1", Type: "task", From: "alice", ThreadID: "th", E2E: true}); len(hub.events) != 0 {
		t.Fatalf("an encrypted task was traced")
	}
}

func TestTracerJoinsOutputLines(t *testing.T) {
	hub := &traceHub{fakeHub: newFakeHub()}
	tr := newTracer(context.Background(), hub, "t1", TraceFull, quiet())
	for i := 0; i < 3; i++ {
		tr.add(harness.Event{Kind: "output", Stream: "stdout", Text: "line"})
	}
	tr.add(harness.Event{Kind: "output", Stream: "stderr", Text: "oops"})
	tr.add(harness.Event{Kind: "output", Stream: "stdout", Text: strings.Repeat("y", traceOutChunk)})
	tr.close()
	var got []harness.Event
	for _, e := range hub.events {
		var ev harness.Event
		json.Unmarshal(e.Data, &ev)
		got = append(got, ev)
	}
	if len(got) != 3 || got[0].Text != "line\nline\nline\n" || got[1].Stream != "stderr" || got[2].Stream != "stdout" {
		t.Fatalf("lines not joined per stream: %+v", got)
	}

	// Past the live limit, output stops (the result still has it).
	hub = &traceHub{fakeHub: newFakeHub()}
	tr = newTracer(context.Background(), hub, "t1", TraceFull, quiet())
	big := strings.Repeat("z", 1000)
	for i := 0; i < traceMaxLive/1000+50; i++ {
		tr.add(harness.Event{Kind: "output", Stream: "stdout", Text: big})
	}
	tr.close()
	n := 0
	for _, e := range hub.events {
		n += len(e.Data)
	}
	if n > traceMaxLive+traceMaxLive/10 || !strings.Contains(string(hub.events[len(hub.events)-1].Data), "more output") {
		t.Fatalf("live output not capped: %d bytes", n)
	}
}

func TestTracerJoinsDeltas(t *testing.T) {
	hub := &traceHub{fakeHub: newFakeHub()}
	tr := newTracer(context.Background(), hub, "t1", TraceFull, quiet())
	tr.add(harness.Event{Kind: "delta", Stream: "thinking", ID: "m/0", Text: "a"})
	tr.add(harness.Event{Kind: "delta", Stream: "thinking", ID: "m/0", Text: "b"})
	tr.add(harness.Event{Kind: "delta", Stream: "text", ID: "m/1", Text: "c"})
	tr.add(harness.Event{Kind: "text", Text: "c!"})
	tr.close()
	var got []harness.Event
	for _, e := range hub.events {
		var ev harness.Event
		json.Unmarshal(e.Data, &ev)
		got = append(got, ev)
	}
	if len(got) != 3 || got[0].Text != "ab" || got[0].Stream != "thinking" || got[1].ID != "m/1" || got[2].Kind != "text" {
		t.Fatalf("deltas not joined per block: %+v", got)
	}

	hub = &traceHub{fakeHub: newFakeHub()}
	tr = newTracer(context.Background(), hub, "t1", TraceSummary, quiet())
	tr.add(harness.Event{Kind: "delta", Stream: "text", ID: "m/1", Text: "secret"})
	tr.close()
	if len(hub.events) != 0 {
		t.Fatalf("summary mode uploaded deltas: %+v", hub.events)
	}
}
