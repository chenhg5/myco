package oscap

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestScreenMovieWaitsForFinalizedOutputAfterStop(t *testing.T) {
	var state atomic.Int32
	var stopped, closed atomic.Bool
	started := make(chan struct{})
	done := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		done <- waitScreenMovie(ctx, 60, func(event string, _ any) {
			if event == "started" {
				close(started)
			}
		}, screenMovieBackend{
			state: func() int { return int(state.Load()) }, failure: func() string { return "failed" }, stop: func() { stopped.Store(true) }, close: func() { closed.Store(true) },
		}, screenMovieTiming{poll: time.Millisecond, start: time.Second, finish: time.Second})
	}()
	select {
	case <-started:
		t.Fatal("reported recording before native start")
	case <-time.After(5 * time.Millisecond):
	}
	state.Store(1)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("no recording event")
	}
	cancel()
	deadline := time.Now().Add(time.Second)
	for !stopped.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !stopped.Load() {
		t.Fatal("stop not delivered")
	}
	select {
	case e := <-done:
		t.Fatalf("returned before file finalized: %v", e)
	case <-time.After(5 * time.Millisecond):
	}
	state.Store(2)
	select {
	case e := <-done:
		if e != nil || !closed.Load() {
			t.Fatalf("completion: %v, closed=%v", e, closed.Load())
		}
	case <-time.After(time.Second):
		t.Fatal("finalized movie not returned")
	}
}
func TestScreenMovieFailureAndStopTimeout(t *testing.T) {
	for _, state := range []int{-1, 0} {
		t.Run(map[int]string{-1: "failed", 0: "stuck"}[state], func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			stopped := false
			err := waitScreenMovie(ctx, 60, func(string, any) {}, screenMovieBackend{state: func() int { return state }, failure: func() string { return "disk full" }, stop: func() { stopped = true }, close: func() {}}, screenMovieTiming{poll: time.Millisecond, start: time.Second, finish: 10 * time.Millisecond})
			if err == nil || AsError(err).Code != CodeFailed || !stopped {
				t.Fatalf("%v, stopped=%v", err, stopped)
			}
		})
	}
}
