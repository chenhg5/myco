package app_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/domain"
	"github.com/myco-ai/myco/internal/infra/notify"
	"github.com/myco-ai/myco/internal/infra/security"
	"github.com/myco-ai/myco/internal/infra/sqlite"
)

func TestTaskMonitorBatchesAcrossProjectsAndCancelsResolvedPending(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	clock := &fakeClock{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	agents := sqlite.NewAgentRepo(db)
	messages := sqlite.NewMessageRepo(db)
	repo := sqlite.NewTaskMonitorRepo(db)
	for _, name := range []string{"lead", "other", "worker"} {
		if err = agents.Create(ctx, &domain.Agent{ID: name, Name: domain.AgentName(name), TokenHash: name, CreatedAt: clock.t, LastSeenAt: clock.t}); err != nil {
			t.Fatal(err)
		}
	}
	svc := app.NewMessageService(agents, messages, notify.NewMemory(), security.IDs{}, clock, app.DefaultMessagePolicy())
	monitor := func() *app.TaskMonitorService {
		return app.NewTaskMonitorService(repo, nil, svc, nil, security.IDs{}, clock, time.Minute)
	}
	add := func(id, from, project string) {
		t.Helper()
		m, _ := domain.NewMessage(id, domain.AgentName(from), "worker", domain.TypeTask, "private task body", "", "", "", clock.t.Add(-time.Hour))
		m.Project = project
		if err := messages.Create(ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	count := func(who domain.AgentName) (int, string) {
		t.Helper()
		xs, err := messages.ListRecent(ctx, who, 100)
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		var text string
		for _, x := range xs {
			if x.Type == domain.TypeNote {
				n++
				text += x.Body
			}
		}
		return n, text
	}
	add("a", "lead", "one")
	add("b", "lead", "two")
	add("c", "other", "")
	if err = monitor().Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if n, text := count("lead"); n != 1 || !strings.Contains(text, "a to worker") || !strings.Contains(text, "b to worker") || strings.Contains(text, "private task body") {
		t.Fatalf("cross-project batch %d %s", n, text)
	}
	if n, _ := count("other"); n != 1 {
		t.Fatal(n)
	}
	add("later", "lead", "")
	add("still-open", "other", "")
	if err = monitor().Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if n, _ := count("lead"); n != 1 {
		t.Fatal("cooldown reset on service restart", n)
	}
	if _, err = messages.Mutate(ctx, "later", func(m *domain.Message) ([]*domain.Message, error) { m.Status = domain.StatusDone; return nil, nil }); err != nil {
		t.Fatal(err)
	}
	clock.t = clock.t.Add(app.TaskAlertCooldown)
	if err = monitor().Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if n, text := count("lead"); n != 1 || strings.Contains(text, "later") {
		t.Fatalf("resolved pending alert sent %d %s", n, text)
	}
	if n, text := count("other"); n != 2 || strings.Count(text, "c to worker") != 1 {
		t.Fatalf("dedup or delivery %d %s", n, text)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err = monitor().Tick(cancelled); err == nil {
		t.Fatal("cancelled evaluation succeeded")
	}
}
func TestTaskBoardPermissionsPoliciesAndActiveAt(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	clock := &fakeClock{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	agents := sqlite.NewAgentRepo(db)
	projects := sqlite.NewProjectRepo(db)
	messages := sqlite.NewMessageRepo(db)
	repo := sqlite.NewTaskMonitorRepo(db)
	for _, name := range []string{"coord", "member", "worker"} {
		if err = agents.Create(ctx, &domain.Agent{ID: name, Name: domain.AgentName(name), TokenHash: name, Owner: "owner", CreatedAt: clock.t}); err != nil {
			t.Fatal(err)
		}
	}
	p := &domain.Project{ID: "p", Name: "acme", Owner: "owner", Visibility: domain.ProjectTeam, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err = projects.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"coord", "member"} {
		role := domain.ProjectRoleMember
		if name == "coord" {
			role = domain.ProjectRoleCoordinator
		}
		if err = projects.PutMember(ctx, &domain.ProjectMember{Project: p.ID, Agent: domain.AgentName(name), Role: role, AddedAt: clock.t}); err != nil {
			t.Fatal(err)
		}
	}
	projectSvc := app.NewProjectService(projects, agents, security.IDs{}, clock)
	m := app.NewTaskMonitorService(repo, projectSvc, nil, nil, security.IDs{}, clock, time.Minute)
	a, _ := agents.Get(ctx, "coord")
	member, _ := agents.Get(ctx, "member")
	by := app.Actor{Agent: a}
	ordinary := app.Actor{Agent: member}
	owner := app.Actor{Member: &domain.Member{ID: "owner", Name: "owner", Role: domain.RoleMember}}
	d := 20 * time.Minute
	if _, err = m.AlertPolicy(ctx, ordinary, "acme", &d, false); !domain.IsCode(err, domain.CodePermissionDenied) {
		t.Fatal("ordinary agent inherited its owner's rights", err)
	}
	if _, err = m.AlertPolicy(ctx, owner, "acme", &d, false); err != nil {
		t.Fatal(err)
	}
	p.Title = "unrelated project edit"
	if err = projects.Save(ctx, p); err != nil {
		t.Fatal(err)
	}
	policy, err := m.AlertPolicy(ctx, by, "acme", nil, false)
	if err != nil || policy.StaleSeconds != 1200 {
		t.Fatal("ordinary project save lost policy", policy, err)
	}
	add := func(id, from string, status domain.Status, at time.Time) {
		t.Helper()
		x, _ := domain.NewMessage(id, domain.AgentName(from), "worker", domain.TypeTask, "private", "", "", "", clock.t.Add(-time.Hour))
		x.Project = p.Name
		x.Status = status
		if err = messages.Create(ctx, x); err != nil {
			t.Fatal(err)
		}
		if err = messages.TouchActive(ctx, id, at); err != nil {
			t.Fatal(err)
		}
	}
	add("fresh", "coord", domain.StatusAcked, clock.t.Add(-time.Minute))
	add("stale", "coord", domain.StatusHeld, clock.t.Add(-21*time.Minute))
	add("other", "member", domain.StatusQueued, time.Time{})
	board, err := m.Board(ctx, by, "", false, 0)
	if err != nil || len(board.Tasks) != 2 || board.Tasks[0].ID != "stale" || !board.Tasks[0].Stale || board.Tasks[1].Stale {
		t.Fatal(board, err)
	}
	all, err := m.Board(ctx, owner, "acme", true, 0)
	if err != nil || len(all.Tasks) != 3 {
		t.Fatal(all, err)
	}
	if _, err = m.Board(ctx, ordinary, "acme", true, 0); !domain.IsCode(err, domain.CodePermissionDenied) {
		t.Fatal(err)
	}
	if _, err = m.Board(ctx, by, "", true, 0); !domain.IsCode(err, domain.CodeInvalidArgument) {
		t.Fatal(err)
	}
	// Board override affects display only.
	board, err = m.Board(ctx, by, "acme", false, 30*time.Minute)
	if err != nil || board.Tasks[0].Stale {
		t.Fatal(board, err)
	}
	policy, err = m.AlertPolicy(ctx, by, "acme", nil, false)
	if err != nil || policy.StaleSeconds != 1200 {
		t.Fatal(policy, err)
	}
	// Revocation is checked afresh, including for dry runs.
	if _, err = db.ExecContext(ctx, `UPDATE project_members SET role='member' WHERE project='p' AND agent='coord'`); err != nil {
		t.Fatal(err)
	}
	if _, err = m.AlertPolicy(ctx, by, "acme", &d, true); !domain.IsCode(err, domain.CodePermissionDenied) {
		t.Fatal("revoked coordinator accepted", err)
	}
}
