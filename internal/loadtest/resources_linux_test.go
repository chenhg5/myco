package loadtest

import (
	"math"
	"testing"
)

func TestSystemCPUPercentKeepsStealAndIOWaitSeparate(t *testing.T) {
	before := []uint64{100, 0, 0, 100, 0, 0, 0, 0}
	after := []uint64{120, 0, 0, 150, 10, 0, 0, 5}
	steal, wait := systemCPUPercent(after, before)
	if steal == nil || wait == nil || steal == wait || math.Abs(*steal-500.0/85) > 1e-9 || math.Abs(*wait-1000.0/85) > 1e-9 {
		t.Fatalf("wrong CPU deltas: steal=%v wait=%v", steal, wait)
	}
	if a, b := systemCPUPercent(before, after); a != nil || b != nil {
		t.Fatal("counter reset must be unknown")
	}
	if a, b := systemCPUPercent(before, before); a != nil || b != nil {
		t.Fatal("no elapsed CPU ticks must be unknown")
	}
	if a, b := systemCPUPercent(nil, before); a != nil || b != nil {
		t.Fatal("missing counters must be unknown")
	}
}
