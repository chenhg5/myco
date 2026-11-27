package app_test

import (
	"context"
	"encoding/json"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/domain"
	"github.com/myco-ai/myco/internal/infra/notify"
	"github.com/myco-ai/myco/internal/infra/security"
	"github.com/myco-ai/myco/internal/infra/sqlite"
	"testing"
	"time"
)

func TestMachineHealthNotificationsDeduplicateAndRecover(t *testing.T) {
	ctx := context.Background()
	db, e := sqlite.Open(":memory:")
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	agents := sqlite.NewAgentRepo(db)
	machines := sqlite.NewMachineRepo(db)
	messages := sqlite.NewMessageRepo(db)
	projects := sqlite.NewProjectRepo(db)
	clock := &fakeClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	for _, a := range []*domain.Agent{{ID: "node", Name: "node", MachineID: "computer", Owner: "owner", Binding: domain.Binding{Harness: "machine"}, TokenHash: "hash", CreatedAt: clock.t}, {ID: "coord", Name: "coord", Owner: "owner", TokenHash: "hash", CreatedAt: clock.t}} {
		a.TokenHash = string(a.Name) + "-hash"
		if e = agents.Create(ctx, a); e != nil {
			t.Fatal(e)
		}
	}
	if e = machines.Create(ctx, &domain.Machine{ID: "computer", Name: "test computer", Owner: "owner", FingerprintHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", CreatedAt: clock.t, LastSeenAt: clock.t}); e != nil {
		t.Fatal(e)
	}
	for _, id := range []string{"project-a", "project-b"} {
		if e = projects.Create(ctx, &domain.Project{ID: id, Name: id, Machines: []string{"computer"}, CreatedAt: clock.t, UpdatedAt: clock.t}); e != nil {
			t.Fatal(e)
		}
		if e = projects.PutMember(ctx, &domain.ProjectMember{Project: id, Agent: "coord", Role: domain.ProjectRoleCoordinator, AddedAt: clock.t}); e != nil {
			t.Fatal(e)
		}
	}
	messageSvc := app.NewMessageService(agents, messages, notify.NewMemory(), security.IDs{}, clock, app.DefaultMessagePolicy())
	service := app.NewMachineService(machines, agents, messageSvc, security.IDs{}, clock, time.Minute)
	service.SetHealthProjects(projects)
	node, e := agents.Get(ctx, "node")
	if e != nil {
		t.Fatal(e)
	}
	h := domain.MachineHealth{OwnerID: "owner", At: clock.t, RunnerInstance: "runner", Incident: "incident-1", Complete: true, Alert: true, Cores: 8, MemoryTotal: 1000, MemoryAvailable: 90, LoadMetric: "load1_per_core", LoadRatio: .9}
	put := func() {
		t.Helper()
		b, _ := json.Marshal(h)
		if e = service.SetRuntime(ctx, node, domain.Runtime{CLI: "machine", MachineHealth: string(b)}); e != nil {
			t.Fatal(e)
		}
	}
	put()
	put()
	put()
	recent, e := messages.ListRecent(ctx, "coord", 20)
	if e != nil || len(recent) != 1 {
		t.Fatalf("alert was not deduplicated across projects and samples: %d %v", len(recent), e)
	}
	h.Complete = false
	put()
	recent, _ = messages.ListRecent(ctx, "coord", 20)
	if len(recent) != 1 {
		t.Fatal("incomplete sample sent recovery")
	}
	h.Complete = true
	h.Alert = false
	h.MemoryAvailable = 500
	h.LoadRatio = .2
	put()
	put()
	recent, e = messages.ListRecent(ctx, "coord", 20)
	if e != nil || len(recent) != 2 {
		t.Fatalf("recovery was not deduplicated: %d %v", len(recent), e)
	}
}

func TestSingleAgentRunnerStillExposesMachinePressure(t *testing.T) {
	ctx := context.Background()
	db, e := sqlite.Open(":memory:")
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	agents := sqlite.NewAgentRepo(db)
	machines := sqlite.NewMachineRepo(db)
	clock := &fakeClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	h := domain.MachineHealth{OwnerID: "owner", At: clock.t, Complete: true, Alert: true, Cores: 8, MemoryTotal: 100, MemoryAvailable: 9, LoadMetric: "load1_per_core", LoadRatio: 1, RunnerInstance: "single"}
	b, _ := json.Marshal(h)
	a := &domain.Agent{ID: "regular", Name: "regular", MachineID: "computer", Owner: "owner", Binding: domain.Binding{Harness: "codex"}, TokenHash: "regular-hash", CreatedAt: clock.t, LastSeenAt: clock.t, Runtime: domain.Runtime{CLI: "codex", RunnerInstance: "single", MachineHealth: string(b)}}
	if e = agents.Create(ctx, a); e != nil {
		t.Fatal(e)
	}
	if e = machines.Create(ctx, &domain.Machine{ID: "computer", Name: "test computer", Owner: "owner", FingerprintHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", CreatedAt: clock.t, LastSeenAt: clock.t}); e != nil {
		t.Fatal(e)
	}
	service := app.NewMachineService(machines, agents, nil, security.IDs{}, clock, time.Minute)
	views, _, e := service.List(ctx)
	if e != nil || len(views) != 1 || views[0].Health == nil || !views[0].Health.Alert {
		t.Fatal("single-agent pressure missing", views, e)
	}
	if views[0].CanStopProcess {
		t.Fatal("offered controls without a matching machine runner")
	}
}
