package domain

import (
	"testing"
	"time"
)

func TestWatchDeadlineAndTerminalState(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	w := Watch{ID: "wat_test", StartedAt: now, Deadline: now.Add(30 * time.Minute), State: WatchWatching}
	if err := w.Extend(now.Add(time.Minute), 23*time.Hour); err != nil {
		t.Fatal(err)
	}
	for _, d := range []time.Duration{0, -time.Second, 24 * time.Hour} {
		if err := w.Extend(now.Add(time.Minute), d); err == nil {
			t.Fatalf("accepted duration %v", d)
		}
	}
	if err := w.End(now.Add(time.Hour), WatchMet, "hub", "done"); err != nil {
		t.Fatal(err)
	}
	if err := w.Extend(now.Add(time.Hour), time.Minute); err == nil {
		t.Fatal("reopened ended watch")
	}
	if err := w.End(now.Add(time.Hour), WatchExpired, "hub", "expired"); err == nil {
		t.Fatal("ended twice")
	}
}
