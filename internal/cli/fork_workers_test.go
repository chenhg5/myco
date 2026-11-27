package cli

import (
	"context"
	"testing"
	"time"
)

func TestForkRetirementWaitsForOnlyTheMatchingWorker(t *testing.T) {
	s := newForkWorkers()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	otherCtx, otherCancel := context.WithCancel(context.Background())
	defer otherCancel()
	otherDone := make(chan struct{})
	defer close(otherDone)
	if !s.add("/profiles/copy", "token-a", cancel, done) || !s.add("/profiles/source", "token-source", otherCancel, otherDone) {
		t.Fatal("worker not registered")
	}
	finished := make(chan error, 1)
	go func() {
		bounded, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		finished <- s.stopPath(bounded, "/profiles/copy")
	}()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("copy not cancelled")
	}
	select {
	case <-otherCtx.Done():
		t.Fatal("source worker was cancelled")
	default:
	}
	select {
	case <-finished:
		t.Fatal("retirement returned before worker cleanup")
	default:
	}
	close(done)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	lateDone := make(chan struct{})
	defer close(lateDone)
	if s.add("/profiles/copy", "token-a", func() {}, lateDone) {
		t.Fatal("retired identity restarted")
	}
	if !s.add("/profiles/copy", "new-identity", func() {}, lateDone) {
		t.Fatal("blocked an unrelated replacement identity")
	}
}
