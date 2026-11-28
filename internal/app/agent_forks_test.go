package app_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/domain"
	"github.com/myco-ai/myco/internal/infra/notify"
	"github.com/myco-ai/myco/internal/infra/security"
	"github.com/myco-ai/myco/internal/infra/sqlite"
)

type forkRig struct {
	ctx                   context.Context
	svc                   *app.ForkService
	repo                  *sqlite.ForkRepo
	agents                *sqlite.AgentRepo
	messages              *app.MessageService
	mr                    *sqlite.MessageRepo
	clock                 *fakeClock
	source, node, creator *domain.Agent
}

func newForkRig(t *testing.T) *forkRig {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	r := &forkRig{ctx: context.Background(), repo: sqlite.NewForkRepo(db), agents: sqlite.NewAgentRepo(db), mr: sqlite.NewMessageRepo(db), clock: &fakeClock{t: time.Unix(1800000000, 0)}}
	ids := security.IDs{}
	r.messages = app.NewMessageService(r.agents, r.mr, notify.NewMemory(), ids, r.clock, app.DefaultMessagePolicy())
	machines := sqlite.NewMachineRepo(db)
	if err = machines.Create(r.ctx, &domain.Machine{ID: "machine-demo", Name: "demo", Owner: "owner-a", FingerprintHash: strings.Repeat("a", 64), CreatedAt: r.clock.t, LastSeenAt: r.clock.t}); err != nil {
		t.Fatal(err)
	}
	makeAgent := func(name, cli, sid string) *domain.Agent {
		a, e := domain.NewAgent(ids.New("agt"), domain.AgentName(name), domain.AgentSpec{Binding: domain.Binding{Harness: cli, SessionID: sid, Workdir: "/srv/demo"}}, security.Tokens{}.Hash(name), r.clock.t)
		if e != nil {
			t.Fatal(e)
		}
		a.Owner = "owner-a"
		a.EnrolledBy = "enrollment-demo"
		a.MachineID = "machine-demo"
		a.LastSeenAt = r.clock.t
		if e = r.agents.Create(r.ctx, a); e != nil {
			t.Fatal(e)
		}
		return a
	}
	r.creator = makeAgent("coordinator", "codex", "coord-session")
	r.source = makeAgent("demo-source", "codex", "source-session")
	r.node = makeAgent("demo-machine", "machine", "machine:demo")
	r.node.AllowFrom = []domain.AgentName{r.creator.Name}
	r.node.Runtime.Features = app.ForkFeature
	if err = r.agents.Save(r.ctx, r.node); err != nil {
		t.Fatal(err)
	}
	if err = r.agents.SetRuntime(r.ctx, r.node.Name, r.node.Runtime); err != nil {
		t.Fatal(err)
	}
	slots, _ := json.Marshal(api.RunnerSlots{MaxConcurrent: 8, InUse: 1, At: r.clock.t})
	r.source.Runtime = domain.Runtime{CLI: "codex", Access: "workspace", RunnerSlots: string(slots), Quota: &domain.Quota{Status: "ok", At: r.clock.t, Source: "codex"}}
	if err = r.agents.SetRuntime(r.ctx, r.source.Name, r.source.Runtime); err != nil {
		t.Fatal(err)
	}
	ms := app.NewMachineService(machines, r.agents, r.messages, ids, r.clock, time.Minute)
	r.svc = app.NewForkService(r.repo, r.agents, ms, r.messages, ids, r.clock)
	return r
}
func (r *forkRig) fork(t *testing.T, key string) *domain.AgentFork {
	t.Helper()
	fs, err := r.svc.Fork(r.ctx, app.Actor{Agent: r.creator}, domain.Viewer{Agent: r.creator}, app.ForkInput{From: string(r.source.Name), RequestID: key}, false)
	if err != nil {
		t.Fatal(err)
	}
	return fs[0]
}
func (r *forkRig) activate(t *testing.T, f *domain.AgentFork) *domain.AgentFork {
	t.Helper()
	a, err := domain.NewAgent(security.IDs{}.New("agt"), domain.AgentName(f.Name), domain.AgentSpec{Binding: domain.Binding{Harness: f.CLI, SessionID: "fork:" + f.SourceSession + ":copy", Workdir: f.SourceDir}}, security.Tokens{}.Hash(f.Name), r.clock.t)
	if err != nil {
		t.Fatal(err)
	}
	a.Owner = f.Owner
	a.EnrolledBy = r.node.EnrolledBy
	if err = r.agents.Create(r.ctx, a); err != nil {
		t.Fatal(err)
	}
	out, err := r.svc.Complete(r.ctx, r.node, f.ID, app.ForkCompletion{Workdir: f.SourceDir})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func TestAgentForkDryRunResourcesIdempotencyAndProvenance(t *testing.T) {
	r := newForkRig(t)
	actor := app.Actor{Agent: r.creator}
	viewer := domain.Viewer{Agent: r.creator}
	in := app.ForkInput{From: string(r.source.Name), Count: 2, RequestID: "batch"}
	if _, err := r.svc.Fork(r.ctx, actor, viewer, in, true); err != nil {
		t.Fatal(err)
	}
	if fs, _ := r.repo.List(r.ctx); len(fs) != 0 {
		t.Fatal("dry run reserved copies")
	}
	fs, err := r.svc.Fork(r.ctx, actor, viewer, in, false)
	if err != nil || len(fs) != 2 {
		t.Fatalf("fork: %v %+v", err, fs)
	}
	again, err := r.svc.Fork(r.ctx, actor, viewer, in, false)
	if err != nil || len(again) != len(fs) {
		t.Fatalf("retry duplicated batch: %v", err)
	}
	seen := map[string]bool{}
	for _, f := range again {
		seen[f.ID] = true
	}
	for _, f := range fs {
		if !seen[f.ID] {
			t.Fatal("retry changed reservation identities")
		}
		if f.ForkOf != string(r.source.Name) || f.CreatorID != "agent:"+r.creator.ID || f.SourceSession != r.source.Binding.SessionID || f.MessageID == "" {
			t.Fatalf("missing trusted provenance: %+v", f)
		}
	}
	in.Count = 3
	if _, err := r.svc.Fork(r.ctx, actor, viewer, in, false); !domain.IsCode(err, domain.CodeConflict) {
		t.Fatalf("changed idempotent request: %v", err)
	}
	r.source.Runtime.Quota = nil
	r.agents.SetRuntime(r.ctx, r.source.Name, r.source.Runtime)
	in.Count = 1
	in.RequestID = "missing-quota"
	if _, err := r.svc.Fork(r.ctx, actor, viewer, in, false); !domain.IsCode(err, domain.CodeInvalidState) {
		t.Fatalf("unknown quota accepted: %v", err)
	}
	in.Force = true
	if _, err := r.svc.Fork(r.ctx, actor, viewer, in, false); err != nil {
		t.Fatalf("force resources: %v", err)
	}
	r.node.Runtime.Features = ""
	r.agents.SetRuntime(r.ctx, r.node.Name, r.node.Runtime)
	in.RequestID = "old-runner"
	if _, err := r.svc.Fork(r.ctx, actor, viewer, in, false); !domain.IsCode(err, domain.CodeUnsupported) {
		t.Fatalf("old runner accepted even with force: %v", err)
	}
}

// A quota snapshot from ten hours ago still says "ok", but nobody refreshed
// it: fork must not read it as room, the same rule the Hub shows people.
func TestAgentForkRefusesStaleQuota(t *testing.T) {
	r := newForkRig(t)
	r.source.Runtime.Quota = &domain.Quota{Status: "ok", At: r.clock.t.Add(-10 * time.Hour), Source: "codex"}
	if err := r.agents.SetRuntime(r.ctx, r.source.Name, r.source.Runtime); err != nil {
		t.Fatal(err)
	}
	actor, viewer := app.Actor{Agent: r.creator}, domain.Viewer{Agent: r.creator}
	if _, err := r.svc.Fork(r.ctx, actor, viewer, app.ForkInput{From: string(r.source.Name), RequestID: "stale-quota"}, false); !domain.IsCode(err, domain.CodeInvalidState) {
		t.Fatalf("stale quota accepted: %v", err)
	}
	// Observed a minute ago, the same numbers pass the resource check.
	r.source.Runtime.Quota.At = r.clock.t.Add(-time.Minute)
	if err := r.agents.SetRuntime(r.ctx, r.source.Name, r.source.Runtime); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Fork(r.ctx, actor, viewer, app.ForkInput{From: string(r.source.Name), RequestID: "fresh-quota"}, false); err != nil {
		t.Fatalf("fresh quota refused: %v", err)
	}
}

func TestAgentRetireNarrowPermissionsAndNoNestedFork(t *testing.T) {
	r := newForkRig(t)
	f := r.activate(t, r.fork(t, "one"))
	copy, _ := r.agents.Get(r.ctx, domain.AgentName(f.Name))
	other := *r.creator
	other.ID = "different-identity"
	for _, a := range []app.Actor{{Agent: &other}, {Agent: r.source}, {Agent: copy}} {
		if _, err := r.svc.Retire(r.ctx, a, app.RetireInput{Name: f.Name}, false); !domain.IsCode(err, domain.CodePermissionDenied) {
			t.Fatalf("unauthorized retire: %v", err)
		}
	}
	if _, err := r.svc.Retire(r.ctx, app.Actor{Admin: true}, app.RetireInput{Name: string(r.source.Name)}, false); !domain.IsCode(err, domain.CodePermissionDenied) {
		t.Fatalf("non-copy retired: %v", err)
	}
	if _, err := r.svc.Fork(r.ctx, app.Actor{Agent: copy}, domain.Viewer{Agent: copy}, app.ForkInput{From: string(r.source.Name), RequestID: "nested", Force: true}, false); !domain.IsCode(err, domain.CodePermissionDenied) {
		t.Fatalf("copy forked source: %v", err)
	}
	if _, err := r.svc.Fork(r.ctx, app.Actor{Agent: r.creator}, domain.Viewer{Agent: r.creator}, app.ForkInput{From: f.Name, RequestID: "nested-source", Force: true}, false); !domain.IsCode(err, domain.CodePermissionDenied) {
		t.Fatalf("nested copy source: %v", err)
	}
	for _, a := range []app.Actor{{Agent: r.creator}, {Member: &domain.Member{ID: "owner-a", Role: domain.RoleMember}}, {Admin: true}} {
		if _, err := r.svc.Retire(r.ctx, a, app.RetireInput{Name: f.Name}, true); err != nil {
			t.Fatalf("authorized retirement rejected: %v", err)
		}
	}
	impostor := *r.node
	impostor.ID = "other-node"
	if _, err := r.svc.MachinePlan(r.ctx, &impostor, f.ID); !domain.IsCode(err, domain.CodePermissionDenied) {
		t.Fatalf("machine identity not checked: %v", err)
	}
}
func TestAgentRetireWaitsAndBlocksNewClaims(t *testing.T) {
	r := newForkRig(t)
	f := r.activate(t, r.fork(t, "active"))
	copy, _ := r.agents.Get(r.ctx, domain.AgentName(f.Name))
	sent, err := r.messages.Send(r.ctx, r.creator, app.SendInput{To: f.Name, Type: "task", Body: "do work"})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := r.mr.ClaimDue(r.ctx, copy.Name, r.clock.t, 1, nil, func(m *domain.Message) bool {
		m.Status = domain.StatusAcked
		m.LeaseUntil = r.clock.t.Add(time.Minute)
		return true
	})
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim: %v", err)
	}
	_, err = r.svc.Retire(r.ctx, app.Actor{Agent: r.creator}, app.RetireInput{Name: f.Name}, false)
	if err != nil {
		t.Fatal(err)
	}
	current, _ := r.repo.Get(r.ctx, f.ID)
	if current.State != "retiring" || current.RetireMessageID != "" {
		t.Fatalf("did not wait: %+v", current)
	}
	_, err = r.messages.Send(r.ctx, r.creator, app.SendInput{To: f.Name, Type: "task", Body: "must not start"})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err = r.mr.ClaimDue(r.ctx, copy.Name, r.clock.t, 1, nil, func(m *domain.Message) bool {
		m.Status = domain.StatusAcked
		m.LeaseUntil = r.clock.t.Add(time.Minute)
		return true
	})
	if err != nil || len(claimed) != 0 {
		t.Fatal("retiring copy claimed more work")
	}
	if _, err = r.messages.Reply(r.ctx, copy, sent.Message.ID, "done", false); err != nil {
		t.Fatal(err)
	}
	if err = r.svc.Tick(r.ctx); err != nil {
		t.Fatal(err)
	}
	current, _ = r.repo.Get(r.ctx, f.ID)
	if current.RetireMessageID == "" {
		t.Fatal("retirement not dispatched after task completed")
	}
	if _, err = r.svc.Complete(r.ctx, r.node, f.ID, app.ForkCompletion{Retired: true}); err != nil {
		t.Fatal(err)
	}
	if _, err = r.agents.Get(r.ctx, copy.Name); !domain.IsCode(err, domain.CodeNotFound) {
		t.Fatalf("copy still registered: %v", err)
	}
}
func TestAgentForkIdleUsesTaskActivityNotHeartbeat(t *testing.T) {
	r := newForkRig(t)
	f := r.activate(t, r.fork(t, "idle"))
	r.clock.t = r.clock.t.Add(29 * time.Minute)
	if err := r.svc.Tick(r.ctx); err != nil {
		t.Fatal(err)
	}
	v, _ := r.repo.Get(r.ctx, f.ID)
	if v.State != "ready" {
		t.Fatal("retired early")
	}
	r.clock.t = r.clock.t.Add(2 * time.Minute)
	r.agents.Touch(r.ctx, domain.AgentName(f.Name), r.clock.t)
	if err := r.svc.Tick(r.ctx); err != nil {
		t.Fatal(err)
	}
	v, _ = r.repo.Get(r.ctx, f.ID)
	if v.State != "retiring" {
		t.Fatalf("presence hid idle copy: %+v", v)
	}
}

func TestAgentForkReservationsAreAtomic(t *testing.T) {
	r := newForkRig(t)
	slots, _ := json.Marshal(api.RunnerSlots{MaxConcurrent: 2, InUse: 1, At: r.clock.t})
	r.source.Runtime.RunnerSlots = string(slots)
	if err := r.agents.SetRuntime(r.ctx, r.source.Name, r.source.Runtime); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 2)
	for _, key := range []string{"a", "b"} {
		go func(key string) {
			_, err := r.svc.Fork(r.ctx, app.Actor{Agent: r.creator}, domain.Viewer{Agent: r.creator}, app.ForkInput{From: string(r.source.Name), RequestID: key}, false)
			result <- err
		}(key)
	}
	successes := 0
	for range 2 {
		err := <-result
		if err == nil {
			successes++
		} else if !domain.IsCode(err, domain.CodeConflict) {
			t.Fatalf("unexpected reservation error: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("slot reservation allowed %d batches", successes)
	}
}
func TestAgentRetirementNeverDeletesAReusedName(t *testing.T) {
	r := newForkRig(t)
	f := r.activate(t, r.fork(t, "reuse"))
	if _, err := r.svc.Retire(r.ctx, app.Actor{Agent: r.creator}, app.RetireInput{Name: f.Name}, false); err != nil {
		t.Fatal(err)
	}
	if err := r.agents.Delete(r.ctx, domain.AgentName(f.Name)); err != nil {
		t.Fatal(err)
	}
	replacement, err := domain.NewAgent("new-identity", domain.AgentName(f.Name), domain.AgentSpec{Binding: domain.Binding{Harness: "codex", SessionID: "new-session", Workdir: "/srv/other"}}, security.Tokens{}.Hash("replacement"), r.clock.t)
	if err != nil {
		t.Fatal(err)
	}
	replacement.Owner = "owner-a"
	if err = r.agents.Create(r.ctx, replacement); err != nil {
		t.Fatal(err)
	}
	if _, err = r.svc.Complete(r.ctx, r.node, f.ID, app.ForkCompletion{Retired: true}); !domain.IsCode(err, domain.CodeConflict) {
		t.Fatalf("reused name removed: %v", err)
	}
	a, err := r.agents.Get(r.ctx, replacement.Name)
	if err != nil || a.ID != replacement.ID {
		t.Fatal("replacement identity was deleted")
	}
	if _, err = r.messages.Send(r.ctx, r.creator, app.SendInput{To: f.Name, Type: "task", Body: "new identity task"}); err != nil {
		t.Fatal(err)
	}
	claimed, err := r.mr.ClaimDue(r.ctx, replacement.Name, r.clock.t, 1, nil, func(m *domain.Message) bool {
		m.Status = domain.StatusAcked
		m.LeaseUntil = r.clock.t.Add(time.Minute)
		return true
	})
	if err != nil || len(claimed) != 1 {
		t.Fatalf("old retirement blocked replacement: %v", err)
	}
}
func TestAgentForkQueuedWorkPreventsIdleExpiration(t *testing.T) {
	r := newForkRig(t)
	f := r.activate(t, r.fork(t, "queue"))
	if _, err := r.messages.Send(r.ctx, r.creator, app.SendInput{To: f.Name, Type: "task", Body: "queued work"}); err != nil {
		t.Fatal(err)
	}
	r.clock.t = r.clock.t.Add(31 * time.Minute)
	if err := r.svc.Tick(r.ctx); err != nil {
		t.Fatal(err)
	}
	now, _ := r.repo.Get(r.ctx, f.ID)
	if now.State != "ready" {
		t.Fatal("queued copy expired")
	}
}
func TestAgentRetireNowIsExplicit(t *testing.T) {
	r := newForkRig(t)
	f := r.activate(t, r.fork(t, "now"))
	copy, _ := r.agents.Get(r.ctx, domain.AgentName(f.Name))
	sent, err := r.messages.Send(r.ctx, r.creator, app.SendInput{To: f.Name, Type: "task", Body: "active"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.mr.ClaimDue(r.ctx, copy.Name, r.clock.t, 1, nil, func(m *domain.Message) bool {
		m.Status = domain.StatusAcked
		m.LeaseUntil = r.clock.t.Add(time.Minute)
		return true
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = r.svc.Retire(r.ctx, app.Actor{Agent: r.creator}, app.RetireInput{Name: f.Name}, false); err != nil {
		t.Fatal(err)
	}
	if _, err = r.svc.Complete(r.ctx, r.node, f.ID, app.ForkCompletion{Retired: true}); !domain.IsCode(err, domain.CodeInvalidState) {
		t.Fatalf("machine bypassed waiting: %v", err)
	}
	if _, err = r.svc.Retire(r.ctx, app.Actor{Agent: r.creator}, app.RetireInput{Name: f.Name, Now: true}, false); err != nil {
		t.Fatal(err)
	}
	pending, _ := r.repo.Get(r.ctx, f.ID)
	if pending.RetireMessageID == "" || !pending.Now {
		t.Fatal("now did not dispatch while task was active")
	}
	if _, err = r.svc.Complete(r.ctx, r.node, f.ID, app.ForkCompletion{Retired: true}); err != nil {
		t.Fatal(err)
	}
	failed, _ := r.mr.Get(r.ctx, sent.Message.ID)
	if failed.Status != domain.StatusFailed {
		t.Fatal("stopped task was left open")
	}
}

func TestAgentForkPreservesPrivateSourceAndChecksDelayedContext(t *testing.T) {
	r := newForkRig(t)
	r.source.Visibility = domain.VisibilityPrivate
	r.source.AllowFrom = []domain.AgentName{r.creator.Name}
	if err := r.agents.Save(r.ctx, r.source); err != nil {
		t.Fatal(err)
	}
	f := r.activate(t, r.fork(t, "private"))
	copy, _ := r.agents.Get(r.ctx, domain.AgentName(f.Name))
	if !copy.Private() || !copy.Accepts(r.creator.Name) || copy.Accepts("unrelated") {
		t.Fatal("private context became publicly callable")
	}
	if _, err := r.svc.Get(r.ctx, app.Actor{Agent: copy}, f.ID); err != nil {
		t.Fatal(err)
	}
	changed := r.source.Binding
	changed.SessionID = "replacement-source-session"
	r.source.Binding = changed
	if err := r.agents.Save(r.ctx, r.source); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Get(r.ctx, app.Actor{Agent: copy}, f.ID); !domain.IsCode(err, domain.CodePermissionDenied) {
		t.Fatalf("delayed fork ignored source replacement: %v", err)
	}
}
func TestAgentRetirementResumesOnlyCapturedWorkAfterRestart(t *testing.T) {
	r := newForkRig(t)
	f := r.activate(t, r.fork(t, "resume"))
	copy, _ := r.agents.Get(r.ctx, domain.AgentName(f.Name))
	current, err := r.messages.Send(r.ctx, r.creator, app.SendInput{To: f.Name, Type: "task", Body: "current"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.mr.ClaimDue(r.ctx, copy.Name, r.clock.t, 1, nil, func(m *domain.Message) bool {
		m.Status = domain.StatusAcked
		m.LeaseUntil = r.clock.t.Add(time.Minute)
		return true
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = r.svc.Retire(r.ctx, app.Actor{Agent: r.creator}, app.RetireInput{Name: f.Name}, false); err != nil {
		t.Fatal(err)
	}
	if _, err = r.messages.Send(r.ctx, r.creator, app.SendInput{To: f.Name, Type: "task", Body: "new must wait"}); err != nil {
		t.Fatal(err)
	}
	if _, err = r.messages.Release(r.ctx, copy, current.Message.ID, 0, false); err != nil {
		t.Fatal(err)
	}
	r.clock.t = r.clock.t.Add(2 * time.Minute)
	reclaimed, err := r.mr.ClaimDue(r.ctx, copy.Name, r.clock.t, 5, nil, func(m *domain.Message) bool {
		m.Status = domain.StatusAcked
		m.LeaseUntil = r.clock.t.Add(time.Minute)
		return true
	})
	if err != nil || len(reclaimed) != 1 || reclaimed[0].ID != current.Message.ID {
		t.Fatalf("restart did not reclaim exactly the current task: %v %+v", err, reclaimed)
	}
	if _, err = r.svc.Complete(r.ctx, r.node, f.ID, app.ForkCompletion{Retired: true}); !domain.IsCode(err, domain.CodeInvalidState) {
		t.Fatalf("retired before resumed task finished: %v", err)
	}
}
func TestUnknownActorCannotRetireAdministratorCopies(t *testing.T) {
	r := newForkRig(t)
	fs, err := r.svc.Fork(r.ctx, app.Actor{Admin: true}, domain.Viewer{All: true}, app.ForkInput{From: string(r.source.Name), RequestID: "admin"}, false)
	if err != nil {
		t.Fatal(err)
	}
	f := r.activate(t, fs[0])
	if _, err = r.svc.Retire(r.ctx, app.Actor{}, app.RetireInput{Name: f.Name}, false); !domain.IsCode(err, domain.CodePermissionDenied) {
		t.Fatalf("unknown actor inherited admin provenance: %v", err)
	}
}

func TestAgentForkUsesSharedAccountWindows(t *testing.T) {
	r := newForkRig(t)
	r.source.Runtime.Account = "shared@example.com"
	r.source.Runtime.Quota = &domain.Quota{Source: "codex", Provider: "codex", Plan: "chatgpt_subscription", Status: "ok", At: r.clock.t.Add(-time.Minute), Windows: []domain.QuotaWindow{{Name: "primary", Used: .2}}}
	r.source.Runtime.QuotaReportedAt = r.clock.t
	if e := r.agents.SetRuntime(r.ctx, r.source.Name, r.source.Runtime); e != nil {
		t.Fatal(e)
	}
	donor, e := domain.NewAgent("donor-id", "donor", domain.AgentSpec{Binding: domain.Binding{Harness: "codex"}}, "donor-token", r.clock.t)
	if e != nil {
		t.Fatal(e)
	}
	donor.Owner = r.source.Owner
	if e = r.agents.Create(r.ctx, donor); e != nil {
		t.Fatal(e)
	}
	rt := domain.Runtime{CLI: "codex", Account: "SHARED@example.com", QuotaReportedAt: r.clock.t, Quota: &domain.Quota{Source: "codex", Provider: "codex", Plan: "chatgpt_subscription", Status: "ok", At: r.clock.t, Windows: []domain.QuotaWindow{{Name: "primary", Used: 1, Status: "rejected"}}}}
	if e = r.agents.SetRuntime(r.ctx, donor.Name, rt); e != nil {
		t.Fatal(e)
	}
	actor, viewer := app.Actor{Agent: r.creator}, domain.Viewer{Agent: r.creator}
	if _, e = r.svc.Fork(r.ctx, actor, viewer, app.ForkInput{From: string(r.source.Name), RequestID: "shared-spent"}, false); !domain.IsCode(e, domain.CodeInvalidState) {
		t.Fatalf("fork ignored shared exhausted account: %v", e)
	}
}
