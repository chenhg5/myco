package runner

import (
	"context"
	"sync"
	"testing"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/harness"
)

type usageHub struct {
	*fakeHub
	mu      sync.Mutex
	reports []api.UsageReport
}

func (h *usageHub) PostUsage(_ context.Context, req api.UsageRequest) (*api.UsageRecorded, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.reports = append(h.reports, req.Reports...)
	return &api.UsageRecorded{Recorded: len(req.Reports)}, nil
}
func TestUsageIndependentOfTraceAndEncryption(t *testing.T) {
	for _, e2e := range []bool{false, true} {
		t.Run(map[bool]string{false: "trace_off", true: "encrypted"}[e2e], func(t *testing.T) {
			t.Setenv("MYCO_NO_USAGE", "")
			hub := &usageHub{fakeHub: newFakeHub(api.Message{ID: "task", ThreadID: "thread", Type: "task", From: "alice", Project: "project", E2E: e2e})}
			h := fakeHarness{fn: func(req harness.Request) (harness.Result, error) {
				req.OnEvent(harness.Event{Kind: "usage", InputTokens: 100, CacheReadTokens: 60, CacheWriteTokens: 10, OutputTokens: 20, CostUSD: 0.1, Model: "model"})
				return harness.Result{Output: "secret output"}, nil
			}}
			r := New(hub, h, NewMemorySessions(), nil, Options{Once: true, Trace: TraceOff}, quiet())
			r.Run(context.Background())
			if len(hub.reports) != 1 {
				t.Fatalf("reports %+v", hub.reports)
			}
			p := hub.reports[0]
			if p.InputTokens != 30 || p.CacheReadTokens != 60 || p.CacheWriteTokens != 10 || p.OutputTokens != 20 || p.Calls != 1 || p.Project != "project" || p.Model != "model" || p.Key == "" {
				t.Fatalf("usage %+v", p)
			}
		})
	}
}
func TestUsageOptOut(t *testing.T) {
	t.Setenv("MYCO_NO_USAGE", "1")
	hub := &usageHub{fakeHub: newFakeHub(api.Message{ID: "task", ThreadID: "thread", Type: "task", From: "alice"})}
	h := fakeHarness{fn: func(req harness.Request) (harness.Result, error) {
		req.OnEvent(harness.Event{Kind: "usage", InputTokens: 10})
		return harness.Result{Output: "ok"}, nil
	}}
	New(hub, h, NewMemorySessions(), nil, Options{Once: true}, quiet()).Run(context.Background())
	if len(hub.reports) != 0 {
		t.Fatal("usage opt-out ignored")
	}
}
