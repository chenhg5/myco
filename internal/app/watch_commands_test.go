package app

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/domain"
	"github.com/myco-ai/myco/internal/infra/notify"
	"github.com/myco-ai/myco/internal/infra/security"
	"github.com/myco-ai/myco/internal/infra/sqlite"
)

func commandFixture(t *testing.T) (*WatchService, *domain.Agent, *actionResourceClock) {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	clock := &actionResourceClock{now: time.Now()}
	ar, mr, wr := sqlite.NewAgentRepo(db), sqlite.NewMessageRepo(db), sqlite.NewWatchRepo(db)
	a, err := domain.NewAgent("agt_demo", "watcher", domain.AgentSpec{}, "test-hash", clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	a.Binding = domain.Binding{Harness: "codex", Environment: "demo-mac", Workdir: "/demo"}
	a.MachineID = "mch_demo"
	a.Runtime = domain.Runtime{Access: "full", MaxAccess: "full", Features: "command_watch", RunnerInstance: "runner-1"}
	if err = ar.Create(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	bus := NewEventBus()
	m := NewMessageService(ar, mr, notify.NewMemory(), security.IDs{}, clock, DefaultMessagePolicy())
	return NewWatchService(wr, ar, m, nil, nil, nil, bus, security.IDs{}, clock), a, clock
}

func TestWatchCommandHistoryAndSingleWake(t *testing.T) {
	s, a, clock := commandFixture(t)
	ctx := context.Background()
	by := Actor{Agent: a}
	w, err := s.Start(ctx, by, WatchStart{What: "build ready", Kind: "command", Command: "echo ready", Match: "ready", EverySec: 1}, false)
	if err != nil {
		t.Fatal(err)
	}
	var successes atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, e := s.ClaimCommand(ctx, by, w.ID, "runner-1"); e == nil {
				successes.Add(1)
			}
		})
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("concurrent claims: %d", successes.Load())
	}
	claimed, _ := s.repo.Get(ctx, w.ID)
	for i := 0; i < 7; i++ {
		if i > 0 {
			clock.now = clock.now.Add(2 * time.Second)
			claimed, err = s.ClaimCommand(ctx, by, w.ID, "runner-1")
			if err != nil {
				t.Fatal(err)
			}
		}
		check := CommandWatchCheck{Runner: "runner-1", CheckID: claimed.ClaimID, Result: "earlier\nready token=super-private", ExitCode: 1, Matched: i == 6}
		w, err = s.CheckCommand(ctx, by, w.ID, check)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.CheckCommand(ctx, by, w.ID, check); err != nil {
			t.Fatal(err)
		}
	}
	if w.State != "met" || w.Checks != 7 || len(w.History) != 5 || strings.Contains(w.LastResult, "super-private") {
		t.Fatalf("watch: %+v", w)
	}
	for range 3 {
		if err = s.Tick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	msgs, err := s.messages.messages.ListRecent(ctx, a.Name, 100)
	if err != nil || len(msgs) != 1 || msgs[0].Type != domain.TypeEvent {
		t.Fatalf("duplicate wake: %d %v", len(msgs), err)
	}
}

func TestWatchCommandLeaseRestartAndExpiry(t *testing.T) {
	s, a, clock := commandFixture(t)
	ctx := context.Background()
	by := Actor{Agent: a}
	w, err := s.Start(ctx, by, WatchStart{What: "ready", Kind: "command", Command: "echo ready", EverySec: 1, ForSec: 60}, false)
	if err != nil {
		t.Fatal(err)
	}
	old, err := s.ClaimCommand(ctx, by, w.ID, "runner-1")
	if err != nil {
		t.Fatal(err)
	}
	a.Runtime.RunnerInstance = "runner-2"
	if _, err = s.ClaimCommand(ctx, by, w.ID, "runner-2"); !domain.IsCode(err, domain.CodeConflict) {
		t.Fatalf("stole live lease: %v", err)
	}
	clock.now = clock.now.Add(20 * time.Second)
	fresh, err := s.ClaimCommand(ctx, by, w.ID, "runner-2")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CheckCommand(ctx, by, w.ID, CommandWatchCheck{CheckID: old.ClaimID, Runner: "runner-1", Matched: true}); !domain.IsCode(err, domain.CodePermissionDenied) {
		t.Fatalf("old runner: %v", err)
	}
	if _, err = s.CheckCommand(ctx, by, w.ID, CommandWatchCheck{CheckID: old.ClaimID, Runner: "runner-2", Matched: true}); !domain.IsCode(err, domain.CodeConflict) {
		t.Fatalf("old lease: %v", err)
	}
	if fresh.ClaimID == old.ClaimID {
		t.Fatal("reused lease")
	}
	clock.now = w.Deadline.Add(time.Second)
	if err = s.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	w, err = s.repo.Get(ctx, w.ID)
	if err != nil || w.State != "expired" {
		t.Fatalf("old/no runner did not expire: %+v %v", w, err)
	}
}

func TestWatchCommandRejectsPermissions(t *testing.T) {
	for _, change := range []func(*domain.Agent){func(a *domain.Agent) { a.Runtime.Access = "workspace" }, func(a *domain.Agent) { a.Runtime.MaxAccess = "workspace" }, func(a *domain.Agent) { a.Runtime.Features = "" }, func(a *domain.Agent) { a.Binding.Harness = "exec" }, func(a *domain.Agent) { a.MachineID = "" }, func(a *domain.Agent) { a.RunSettings.Access = "readonly" }} {
		s, a, _ := commandFixture(t)
		change(a)
		if _, err := s.Start(context.Background(), Actor{Agent: a}, WatchStart{What: "ready", Kind: "command", Command: "echo ready"}, false); !domain.IsCode(err, domain.CodePermissionDenied) {
			t.Fatalf("permissions: %v", err)
		}
	}
}
