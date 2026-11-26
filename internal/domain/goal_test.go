package domain

import (
	"testing"
	"time"
)

func TestGoalRoundBackoffAndTerminal(t *testing.T) {
	now := time.Now()
	g := Goal{State: "active", EverySec: 1800, BaseEverySec: 1800}
	r := GoalRound{Summary: "none"}
	for i := 1; i <= 4; i++ {
		g.FinishRound(now, &r)
		if g.NoProgress != i {
			t.Fatal(g)
		}
		if i == 2 && g.EverySec != 3600 {
			t.Fatal(g)
		}
		if i == 4 && g.State != "paused" {
			t.Fatal(g)
		}
	}
	g.State = "active"
	r.Progressed = true
	g.FinishRound(now, &r)
	if g.NoProgress != 0 || g.EverySec != 1800 {
		t.Fatal(g)
	}
	g.End(now, "achieved", "demo", "done")
	if g.Open() || g.NextWakeAt != nil || g.EndedAt == nil {
		t.Fatal(g)
	}
}
