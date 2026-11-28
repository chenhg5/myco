package runner

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/harness"
)

func admitted(t *testing.T, s *SlotTicket) {
	t.Helper()
	select {
	case <-s.Ready:
	case <-time.After(time.Second):
		t.Fatal("not admitted")
	}
}
func waiting(t *testing.T, s *SlotTicket) {
	t.Helper()
	select {
	case <-s.Ready:
		t.Fatal("admitted without capacity")
	default:
	}
}

func TestSlotsWorkspaceFairnessAndFIFO(t *testing.T) {
	p := NewSlotPool(2)
	a0 := p.Request("https://hub/w/a/", "a", "a0")
	a1 := p.Request("https://hub/w/a", "a", "a1")
	admitted(t, a0)
	admitted(t, a1)
	a2 := p.Request("https://hub/w/a", "a", "a2")
	a3 := p.Request("https://hub/w/a", "a", "a3")
	b0 := p.Request("https://hub/w/b", "b", "b0")
	b1 := p.Request("https://hub/w/b", "b", "b1")
	waiting(t, b0)
	// The newly waiting workspace gets the next release, ahead of a2.
	a0.Done()
	admitted(t, b0)
	waiting(t, a2)
	// Both now own capacity: round-robin, with FIFO inside each workspace.
	a1.Done()
	admitted(t, a2)
	waiting(t, a3)
	waiting(t, b1)
	b0.Done()
	admitted(t, b1)
	waiting(t, a3)
	a2.Done()
	admitted(t, a3)
	a3.Done()
	b1.Done()
	if s := p.Snapshot(""); s.InUse != 0 || s.Waiting != 0 {
		t.Fatalf("leaked slots: %+v", s)
	}
}

func TestSlotsSingleWorkspaceCancelAndIdentityIsolation(t *testing.T) {
	p := NewSlotPool(1)
	a := p.Request("https://hub/w/a", SlotOwner("https://hub/w/a", "same"), "a")
	admitted(t, a)
	b := p.Request("https://hub/w/a", SlotOwner("https://hub/w/a", "same"), "b")
	c := p.Request("https://hub/w/b", SlotOwner("https://hub/w/b", "same"), "c")
	d := p.Request("https://hub/w/a", SlotOwner("https://hub/w/a", "same"), "d")
	snap := p.Snapshot(SlotOwner("https://hub/w/a", "same"))
	if len(snap.Queue) != 2 || snap.Queue[0].MessageID != "b" || snap.Queue[0].Position != 2 {
		t.Fatalf("bad projected queue or leaked another workspace: %+v", snap)
	}
	c.Done()
	c.Done()
	a.Done()
	admitted(t, b)
	waiting(t, d)
	b.Done()
	admitted(t, d)
	d.Done()
	d.Done()
	if p.Snapshot("").InUse != 0 {
		t.Fatal("cancellation leaked capacity")
	}
}

func TestRunnerReportsWaitingAndClearsOnAdmission(t *testing.T) {
	p := NewSlotPool(1)
	holder := p.Request("https://hub/a", "other", "holder")
	h := newFakeHub(api.Message{ID: "queued", Type: "task", From: "alice", Body: "hi"})
	states := make(chan *api.RunnerSlots, 10)
	ran := make(chan struct{}, 1)
	r := New(h, fakeHarness{fn: func(harness.Request) (harness.Result, error) {
		ran <- struct{}{}
		return harness.Result{Output: "done"}, nil
	}}, NewMemorySessions(), nil, Options{Self: "me", HubURL: "https://hub/b", SlotPool: p, OnSlots: func(s *api.RunnerSlots) { states <- s }, Once: true, Trace: TraceOff, SlotTouch: time.Millisecond}, quiet())
	done := make(chan error, 1)
	go func() { done <- r.Run(context.Background()) }()
	select {
	case s := <-states:
		if len(s.Queue) != 1 || s.Queue[0].MessageID != "queued" || s.Queue[0].Position != 1 || s.InUse != 1 || s.Waiting != 1 {
			t.Fatalf("not reported waiting: %+v", s)
		}
	case <-time.After(time.Second):
		t.Fatal("no queue report")
	}
	holder.Done()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("runner blocked")
	}
	select {
	case <-ran:
	default:
		t.Fatal("never ran")
	}
	select {
	case s := <-states:
		if len(s.Queue) != 0 || s.Waiting != 0 {
			t.Fatal("queue not cleared")
		}
	default:
		t.Fatal("no admission report")
	}
	if p.Snapshot("").InUse != 0 {
		t.Fatal("runner leaked slot")
	}
}

func TestSlotCancelGrantRace(t *testing.T) {
	for i := 0; i < 100; i++ {
		p := NewSlotPool(1)
		a := p.Request("a", "a", "a")
		b := p.Request("b", "b", "b")
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); a.Done() }()
		go func() { defer wg.Done(); b.Done() }()
		wg.Wait()
		if s := p.Snapshot(""); s.InUse != 0 || s.Waiting != 0 {
			t.Fatalf("leaked after cancel/grant race: %+v", s)
		}
	}
}

func TestSlotsRotateWhenAllWorkspacesStillHaveCapacity(t *testing.T) {
	p := NewSlotPool(4)
	a0 := p.Request("a", "a", "a0")
	a1 := p.Request("a", "a", "a1")
	b0 := p.Request("b", "b", "b0")
	b1 := p.Request("b", "b", "b1")
	a2 := p.Request("a", "a", "a2")
	a3 := p.Request("a", "a", "a3")
	b2 := p.Request("b", "b", "b2")
	a0.Done()
	admitted(t, a2)
	waiting(t, b2)
	b0.Done()
	admitted(t, b2)
	waiting(t, a3)
	a1.Done()
	admitted(t, a3)
	a2.Done()
	a3.Done()
	b1.Done()
	b2.Done()
}

func TestQueuedRunnerStopsWithoutStartingAndClearsReport(t *testing.T) {
	p := NewSlotPool(1)
	a := p.Request("a", "a", "a")
	hub := &countingHub{fakeHub: newFakeHub(api.Message{ID: "q", Type: "task", From: "alice", Body: "test"})}
	states := make(chan *api.RunnerSlots, 8)
	r := New(hub, fakeHarness{fn: func(harness.Request) (harness.Result, error) {
		t.Error("closed task started")
		return harness.Result{}, nil
	}}, NewMemorySessions(), nil, Options{Self: "me", HubURL: "b", SlotPool: p, SlotTouch: time.Millisecond, Once: true, OnSlots: func(s *api.RunnerSlots) { states <- s }}, quiet())
	done := make(chan error, 1)
	go func() { done <- r.Run(context.Background()) }()
	select {
	case <-states:
	case <-time.After(time.Second):
		t.Fatal("no waiting report")
	}
	hub.set("q", "failed")
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("did not cancel")
	}
	if s := <-states; len(s.Queue) != 0 || s.Waiting != 0 {
		t.Fatalf("stale queue: %+v", s)
	}
	if p.Snapshot("").InUse != 1 {
		t.Fatal("took another task's slot")
	}
	a.Done()
}

func TestStoppedWaiterIsCheckedAgainOnAdmission(t *testing.T) {
	p := NewSlotPool(1)
	holder := p.Request("a", "a", "a")
	hub := newFakeHub(api.Message{ID: "q", Type: "task", From: "alice", Body: "test"})
	seen := make(chan struct{}, 1)
	r := New(hub, fakeHarness{fn: func(harness.Request) (harness.Result, error) {
		t.Error("stopped task ran")
		return harness.Result{}, nil
	}}, NewMemorySessions(), nil, Options{Self: "me", HubURL: "b", SlotPool: p, SlotTouch: time.Hour, Once: true, OnSlots: func(s *api.RunnerSlots) {
		if len(s.Queue) > 0 {
			select {
			case seen <- struct{}{}:
			default:
			}
		}
	}}, quiet())
	done := make(chan error, 1)
	go func() { done <- r.Run(context.Background()) }()
	select {
	case <-seen:
	case <-time.After(time.Second):
		t.Fatal("never queued")
	}
	hub.set("q", "failed")
	holder.Done()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("never admitted")
	}
	if s := p.Snapshot(""); s.InUse != 0 || s.Waiting != 0 {
		t.Fatal("stopped task leaked admission")
	}
}

// The limit changes while turns run: raising it starts waiting turns at
// once; lowering it stops new starts and leaves running ones alone.
func TestSlotsSetMaxLive(t *testing.T) {
	p := NewSlotPool(1)
	a := p.Request("https://hub/w/a", "a", "m1")
	b := p.Request("https://hub/w/a", "a", "m2")
	c := p.Request("https://hub/w/b", "b", "m3")
	admitted(t, a)
	waiting(t, b)
	waiting(t, c)

	p.SetMax(3) // raised: both waiting turns start now
	admitted(t, b)
	admitted(t, c)
	if s := p.Snapshot(""); s.InUse != 3 || s.MaxConcurrent != 3 || s.Limit != 3 || s.Waiting != 0 {
		t.Fatalf("after raising: %+v", s)
	}

	p.SetMax(1) // lowered: nothing running is stopped
	for _, tk := range []*SlotTicket{a, b, c} {
		if tk.ended {
			t.Fatal("lowering the limit ended a running turn")
		}
	}
	d := p.Request("https://hub/w/a", "a", "m4")
	waiting(t, d)
	// Still reportable to hubs that refuse in_use above max_concurrent.
	if s := p.Snapshot("a"); s.InUse != 3 || s.Limit != 1 || s.MaxConcurrent != 3 || s.Waiting != 1 {
		t.Fatalf("while draining: %+v", s)
	}
	a.Done()
	waiting(t, d) // 2 running, limit 1
	b.Done()
	waiting(t, d) // 1 running, limit 1
	c.Done()
	admitted(t, d)
	if s := p.Snapshot(""); s.InUse != 1 || s.MaxConcurrent != 1 || s.Limit != 1 {
		t.Fatalf("after draining: %+v", s)
	}
	d.Done()
	p.SetMax(0) // never below one
	if p.Max() != 1 {
		t.Fatalf("max = %d", p.Max())
	}
}
