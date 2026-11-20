package domain

import (
	"testing"
	"time"
)

func TestMetricsCountOverlapsOnce(t *testing.T) {
	t0 := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	at := func(m int) time.Time { return t0.Add(time.Duration(m) * time.Minute) }
	run := &WorkflowRun{CreatedAt: t0, EndedAt: at(30)}
	steps := []*RunStep{
		{Kind: StepAgent, Status: StepDone, StartedAt: at(0), EndedAt: at(10), Retries: 1},
		{Kind: StepAgent, Status: StepDone, StartedAt: at(5), EndedAt: at(12)}, // in parallel
		{Kind: StepHuman, Status: StepDone, StartedAt: at(11), EndedAt: at(20)},
		{Kind: StepComputer, Status: StepFailed, StartedAt: at(25), EndedAt: at(27), Tasks: []string{"a", "b"}},
	}
	m := MetricsOf(run, steps, at(99))
	min := func(n int64) int64 { return n * 60_000 }
	if m.DurationMS != min(30) || m.ActiveMS != min(14) || m.WaitingMS != min(8) {
		t.Fatalf("times: %+v", m)
	}
	if m.Steps != 4 || m.Done != 3 || m.Failed != 1 || m.Retries != 1 || m.Attempts != 2+1+1+2 {
		t.Fatalf("counts: %+v", m)
	}
	// An open run is timed to now.
	run.EndedAt = time.Time{}
	if m := MetricsOf(run, nil, at(45)); m.DurationMS != min(45) {
		t.Fatalf("open run: %+v", m)
	}
}

func TestDistAndBetter(t *testing.T) {
	d := DistOf([]float64{5, 1, 3, 2, 4})
	if d.N != 5 || d.Min != 1 || d.Max != 5 || d.Avg != 3 || d.P50 != 3 || d.P95 != 5 {
		t.Fatalf("dist: %+v", d)
	}
	if (DistOf(nil) != Dist{}) {
		t.Fatal("empty dist")
	}
	if Better(BetterHigher, 4, 2) != "a" || Better(BetterLower, 4, 2) != "b" || Better("", 4, 2) != "" || Better(BetterHigher, 1, 1) != "" {
		t.Fatal("better")
	}
	for _, e := range []RunEvaluation{{Name: "1x"}, {Name: "ok", Direction: "up"}, {Name: "ok", Unit: "12345678901234567"}} {
		if e.Validate() == nil {
			t.Fatalf("%+v passed", e)
		}
	}
	if (&RunEvaluation{Name: "tests.passed", Direction: BetterHigher}).Validate() != nil {
		t.Fatal("a good score failed")
	}
}
