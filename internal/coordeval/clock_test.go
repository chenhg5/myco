package coordeval

import (
	"context"
	"testing"
	"time"
)

func clockTestContext(t *testing.T) (context.Context, *scenarioClock) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	c := newScenarioClock()
	return context.WithValue(ctx, scenarioClockKey{}, c), c
}

// Host work, including a request waiting on slow storage, is runnable:
// another participant's timer must not consume that participant's budget.
func TestScenarioClockWaitsForActiveParticipant(t *testing.T) {
	ctx, c := clockTestContext(t)
	_, release := scenarioActorContext(ctx)
	start := c.Now()
	woke := make(chan bool, 1)
	goScenario(ctx, func(ctx context.Context) { woke <- sleepCtx(ctx, time.Second) })
	if !c.Now().Equal(start) {
		t.Fatal("clock advanced during active work")
	}
	select {
	case <-woke:
		t.Fatal("timer fired while a participant was active")
	default:
	}
	release()
	select {
	case ok := <-woke:
		if !ok {
			t.Fatal("timer was cancelled")
		}
	case <-ctx.Done():
		t.Fatal("timer did not fire after yielding")
	}
	if !c.Now().Equal(start.Add(time.Second)) {
		t.Fatalf("scenario time: %s", c.Now().Sub(start))
	}
}

// A parent's join is runnable as soon as its last child finishes, even
// if the OS has not scheduled the parent yet. No timer may leap over it.
func TestScenarioClockJoinResumesBeforeLaterDeadline(t *testing.T) {
	ctx, c := clockTestContext(t)
	ctx, done := scenarioActorContext(ctx)
	defer done()
	start := c.Now()
	join := newScenarioJoin(ctx, 1)
	goScenario(ctx, func(ctx context.Context) { sleepCtx(ctx, time.Second); join.done() })
	goScenario(ctx, func(ctx context.Context) { sleepCtx(ctx, 100*time.Second) })
	join.wait()
	if !c.Now().Equal(start.Add(time.Second)) {
		t.Fatalf("clock ran past the resumed parent: %s", c.Now().Sub(start))
	}
}

func TestScenarioClockCancellationDoesNotRequireAdvancingTime(t *testing.T) {
	ctx, c := clockTestContext(t)
	watchdog := ctx
	_, release := scenarioActorContext(ctx)
	defer release()
	ctx, cancel := context.WithCancel(ctx)
	start := c.Now()
	woke := make(chan bool, 1)
	goScenario(ctx, func(ctx context.Context) { woke <- sleepCtx(ctx, time.Hour) })
	cancel()
	select {
	case ok := <-woke:
		if ok {
			t.Fatal("cancelled sleep succeeded")
		}
	case <-watchdog.Done():
		t.Fatal("cancel did not release sleeper")
	}
	if !c.Now().Equal(start) {
		t.Fatal("cancellation advanced scenario time")
	}
}
