package perf

import (
	"context"
	"testing"
	"time"
)

func TestReportPercentilesAndWindow(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	r := New()
	r.now = func() time.Time { return now }
	for i := 0; i < 95; i++ {
		r.Add("fast", time.Millisecond, 1)
	}
	for i := 0; i < 5; i++ {
		r.Add("fast", 200*time.Millisecond, 3)
	}
	r.Add("slow", 2*time.Second, 40)
	got := r.Report(15*time.Minute, 0)
	if len(got) != 2 || got[0].Name != "slow" || got[1].Name != "fast" {
		t.Fatalf("order: %+v", got)
	}
	f := got[1]
	if f.Count != 100 || f.Max != 200*time.Millisecond || f.MaxRows != 3 || f.AvgRows != 1 {
		t.Fatalf("fast: %+v", f)
	}
	// (bucket bounds: within 25% above the true value)
	if f.P50 < time.Millisecond || f.P50 > 1250*time.Microsecond || f.P95 > 1250*time.Microsecond {
		t.Fatalf("fast percentiles: p50 %v p95 %v", f.P50, f.P95)
	}
	if s := got[0]; s.P50 != 2*time.Second || s.P95 != 2*time.Second {
		t.Fatalf("one sample is its own percentile, capped at the max: %+v", s)
	}
	if got := r.Report(time.Minute, 1); len(got) != 1 || got[0].Name != "slow" {
		t.Fatalf("limit: %+v", got)
	}

	now = now.Add(20 * time.Minute)
	if got := r.Report(15*time.Minute, 0); len(got) != 0 {
		t.Fatalf("outside the window: %+v", got)
	}
	r.Add("later", time.Millisecond, 0)
	now = now.Add(Keep)
	r.Add("much later", time.Millisecond, 0)
	if len(r.minutes) != 1 {
		t.Fatalf("old minutes are dropped: %d kept", len(r.minutes))
	}
	var nilRec *Recorder
	nilRec.Add("x", time.Second, 0)
	if nilRec.Report(time.Minute, 0) != nil {
		t.Fatal("a nil recorder reports nothing")
	}
}

func TestRoute(t *testing.T) {
	ctx := WithRoute(context.Background())
	SetRoute(context.WithValue(ctx, struct{}{}, 1), "GET /v1/agents")
	if Route(ctx) != "GET /v1/agents" {
		t.Fatalf("route: %q", Route(ctx))
	}
	SetRoute(context.Background(), "x") // (no place: no-op)
	if Route(context.Background()) != "" {
		t.Fatal("no route without WithRoute")
	}
}
