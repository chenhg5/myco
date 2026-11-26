package claude

import (
	"testing"
	"time"
)

func TestRateLimitQuota(t *testing.T) {
	at := time.Date(2026, 11, 26, 8, 37, 0, 0, time.UTC)
	// A rate_limit_event's payload: both windows.
	q := RateLimitQuota([]byte(`{"status":"allowed","rateLimitType":"five_hour","resetsAt":1795687200,
		"unifiedWindows":{"seven_day":{"resetsAt":1796090400,"status":"allowed","utilization":0.4},
		"five_hour":{"resetsAt":1795687200,"status":"allowed","utilization":0.15}}}`), at)
	if q == nil || len(q.Windows) != 2 || q.Windows[0].Name != "five_hour" || q.Windows[0].Used != 0.15 ||
		q.Windows[1].Name != "seven_day" || !q.Windows[1].ResetsAt.Equal(time.Unix(1796090400, 0)) || !q.At.Equal(at) {
		t.Fatalf("windows: %+v", q)
	}
	// What a transcript keeps when a limit was reached: that window is full.
	q = RateLimitQuota([]byte(`{"status":"rejected","resetsAt":1795485600,"rateLimitType":"seven_day","overageStatus":"rejected"}`), time.Time{})
	if q == nil || len(q.Windows) != 1 || q.Windows[0].Name != "seven_day" || q.Windows[0].Used != 1 || q.Windows[0].Status != "rejected" {
		t.Fatalf("refused: %+v", q)
	}
	// Nothing usable.
	for _, in := range []string{`{}`, `{"status":"allowed","rateLimitType":"five_hour"}`, `not json`} {
		if q := RateLimitQuota([]byte(in), at); q != nil {
			t.Fatalf("%s: %+v", in, q)
		}
	}
}
