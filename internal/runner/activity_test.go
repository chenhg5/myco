package runner

import (
	"context"
	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/harness"
	"sync"
	"testing"
	"time"
)

func TestIdleActivityWaitsForWorkAndPollResponses(t *testing.T) {
	a := &Activity{}
	done, ok := a.Work(t.Context())
	if !ok {
		t.Fatal("work refused")
	}
	if a.PauseIfIdle() {
		t.Fatal("paused a running task")
	}
	done()
	poll, finish, ok := a.Poll(t.Context())
	if !ok {
		t.Fatal("poll refused")
	}
	if !a.PauseIfIdle() || a.Quiescent() {
		t.Fatal("in-flight claim was ignored")
	}
	<-poll.Done()
	// The Hub may already have claimed a task when cancellation arrives.
	finish(1)
	if a.Quiescent() {
		t.Fatal("late claim was ignored")
	}
	a.Pending("agent", 1)
	a.Done()
	if a.Quiescent() {
		t.Fatal("undelivered result was ignored")
	}
	a.Pending("agent", 0)
	if !a.Quiescent() {
		t.Fatal("completed work did not drain")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, ok := a.Poll(ctx); ok {
		t.Fatal("new claims started while paused")
	}
	a.Resume()
	done, ok = a.Work(t.Context())
	if !ok {
		t.Fatal("work did not resume")
	}
	done()
}

func TestIdleActivityTracksEveryPoll(t *testing.T) {
	a := &Activity{}
	_, one, _ := a.Poll(t.Context())
	_, two, _ := a.Poll(t.Context())
	a.PauseIfIdle()
	one(0)
	if a.Quiescent() {
		t.Fatal("one poll lease replaced another")
	}
	two(0)
	if !a.Quiescent() {
		t.Fatal("polls did not drain")
	}
}

func TestIdleActivityWaitsForHarnessAndDeliveredReply(t *testing.T) {
	a := &Activity{}
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	m := api.Message{ID: "task", Type: "task", From: "sender", ThreadID: "thread"}
	hub := newFakeHub(m)
	h := ctxHarness{fn: func(ctx context.Context, _ harness.Request) (harness.Result, error) {
		close(entered)
		select {
		case <-ctx.Done():
			return harness.Result{}, ctx.Err()
		case <-release:
			return harness.Result{Output: "finished"}, nil
		}
	}}
	r := New(hub, h, NewMemorySessions(), nil, Options{Self: "worker", Activity: a, Trace: TraceOff}, quiet())
	go func() { defer close(finished); r.Handle(t.Context(), m) }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("harness did not start")
	}
	if a.PauseIfIdle() {
		t.Fatal("restart permitted during harness task")
	}
	close(release)
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("task did not finish")
	}
	if !a.PauseIfIdle() || !a.Quiescent() {
		t.Fatal("delivered reply did not become idle")
	}
	hub.mu.Lock()
	defer hub.mu.Unlock()
	if hub.replies[m.ID] != "finished" || hub.status[m.ID] != "done" {
		t.Fatalf("task was interrupted: %+v", hub.status)
	}
}

func TestIdleActivityWaitsForAllClaimedProfiles(t *testing.T) {
	a := &Activity{}
	entered := make(chan struct{}, 2)
	start := func(name string) (*fakeHub, func(), <-chan struct{}) {
		m := api.Message{ID: name, Type: "task", From: "sender", ThreadID: name}
		hub := newFakeHub(m)
		release, finished := make(chan struct{}), make(chan struct{})
		var once sync.Once
		unblock := func() { once.Do(func() { close(release) }) }
		t.Cleanup(unblock)
		h := ctxHarness{fn: func(ctx context.Context, _ harness.Request) (harness.Result, error) {
			entered <- struct{}{}
			select {
			case <-ctx.Done():
				return harness.Result{}, ctx.Err()
			case <-release:
				return harness.Result{Output: "delivered"}, nil
			}
		}}
		r := New(hub, h, NewMemorySessions(), nil, Options{Self: name, Once: true, Activity: a, Trace: TraceOff}, quiet())
		go func() {
			defer close(finished)
			if err := r.Run(t.Context()); err != nil {
				t.Error(err)
			}
		}()
		return hub, unblock, finished
	}
	first, releaseFirst, firstDone := start("first")
	second, releaseSecond, secondDone := start("second")
	await := func(ch <-chan struct{}) {
		t.Helper()
		select {
		case <-ch:
		case <-time.After(5 * time.Second):
			t.Fatal("runner did not reach the checkpoint")
		}
	}
	await(entered)
	await(entered)
	if a.PauseIfIdle() {
		t.Fatal("claimed work was not counted")
	}
	releaseFirst()
	await(firstDone)
	if a.PauseIfIdle() {
		t.Fatal("another profile still has a task")
	}
	releaseSecond()
	await(secondDone)
	if !a.PauseIfIdle() || !a.Quiescent() {
		t.Fatal("finished profiles did not drain")
	}
	for _, hub := range []*fakeHub{first, second} {
		hub.mu.Lock()
		for _, reply := range hub.replies {
			if reply != "delivered" {
				t.Error("reply lost")
			}
		}
		hub.mu.Unlock()
	}
}
