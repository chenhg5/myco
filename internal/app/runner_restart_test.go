package app_test

import (
	"context"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/domain"
	"github.com/myco-ai/myco/internal/infra/security"
	"github.com/myco-ai/myco/internal/infra/sqlite"
	"testing"
	"time"
)

func TestRuntimeRestartLeaseExpiresAndOnlyNewInstanceClearsIt(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	agents := sqlite.NewAgentRepo(db)
	machines := sqlite.NewMachineRepo(db)
	clock := &fakeClock{t: time.Now().UTC()}
	a := &domain.Agent{Name: "worker", ID: "a_worker", TokenHash: "hash", CreatedAt: clock.t}
	if err := agents.Create(ctx, a); err != nil {
		t.Fatal(err)
	}
	svc := app.NewMachineService(machines, agents, nil, security.IDs{}, clock, time.Minute)
	put := func(rt domain.Runtime) {
		t.Helper()
		if err := svc.SetRuntime(ctx, a, rt); err != nil {
			t.Fatal(err)
		}
	}
	get := func() domain.Runtime {
		t.Helper()
		got, err := agents.Get(ctx, a.Name)
		if err != nil {
			t.Fatal(err)
		}
		return got.Runtime
	}
	put(domain.Runtime{CLI: "exec", RunnerInstance: "old"})
	put(domain.Runtime{CLI: "exec", RunnerInstance: "old", Away: "updating", RestartingUntil: clock.t.Add(time.Hour)})
	if rt := get(); !rt.Updating(clock.t) || !rt.RestartingUntil.Equal(clock.t.Add(domain.RestartLease)) {
		t.Fatalf("unbounded lease: %+v", rt)
	}
	put(domain.Runtime{CLI: "exec", RunnerInstance: "old"})
	if !get().Updating(clock.t) {
		t.Fatal("old process cleared restart status")
	}
	put(domain.Runtime{CLI: "exec", RunnerInstance: "new"})
	if get().Updating(clock.t) {
		t.Fatal("replacement did not clear lease")
	}
	put(domain.Runtime{CLI: "exec", RunnerInstance: "old", Away: "stopped"})
	if get().RunnerInstance != "new" {
		t.Fatal("late goodbye overwrote new process")
	}
	put(domain.Runtime{CLI: "exec", RunnerInstance: "new", Away: "updating"}) // old announcer, no deadline
	rt := get()
	if !rt.Updating(clock.t) {
		t.Fatal("missing bounded default")
	}
	clock.t = clock.t.Add(domain.RestartLease + time.Second)
	if rt.Updating(clock.t) {
		t.Fatal("failed restart stayed updating")
	}
}
