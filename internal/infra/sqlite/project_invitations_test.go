package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/domain"
)

func TestProjectInvitationAtomicJoinRollback(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	now := time.Now().UTC()
	p := &domain.Project{ID: "p", Name: "fixture", Owner: "owner", CreatedAt: now, UpdatedAt: now}
	if err = NewProjectRepo(db).Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	coord, _ := domain.NewAgent("c", "coord", domain.AgentSpec{}, "hash", now)
	if err = NewAgentRepo(db).Create(ctx, coord); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO project_members(project,agent,role,added_at)VALUES('p','coord','coordinator',1)`); err != nil {
		t.Fatal(err)
	}
	repo := NewProjectInvitationRepo(db)
	v := &domain.ProjectInvitation{ID: "inv", Project: "p", Requester: "coord", Owner: "owner", Note: "review", Status: "approved", CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	if err = repo.Create(ctx, v); err != nil {
		t.Fatal(err)
	}
	e, err := domain.NewEnrollment("enr", "hash", domain.EnrollmentSpec{NamePrefix: "invited-", Owner: "owner", MaxUses: 1, TTL: 24 * time.Hour}, now)
	if err != nil {
		t.Fatal(err)
	}
	e.InvitationID = v.ID
	if err = repo.Issue(ctx, v.ID, e, now); err != nil {
		t.Fatal(err)
	}
	if err = repo.Issue(ctx, v.ID, e, now); err == nil {
		t.Fatal("issued twice")
	}
	a, _ := domain.NewAgent("new", "new-agent", domain.AgentSpec{Binding: domain.Binding{Harness: "inbox", SessionID: "new-session"}}, "new-hash", now)
	a.Owner = "owner"
	a.InvitedBy = "coord"
	a.EnrolledBy = e.ID
	if err = repo.Use(ctx, v.ID, a, now.Add(25*time.Hour)); err == nil {
		t.Fatal("expired invitation admitted")
	}
	if _, err = db.Exec(`CREATE TRIGGER fail_project_join BEFORE INSERT ON project_members WHEN NEW.agent='new-agent' BEGIN SELECT RAISE(ABORT,'fixture'); END`); err != nil {
		t.Fatal(err)
	}
	if err = repo.Use(ctx, v.ID, a, now); err == nil {
		t.Fatal("failed member insertion was ignored")
	}
	v, err = repo.Get(ctx, v.ID)
	if err != nil || v.Status != "ready" {
		t.Fatal("invitation consumed on failed join")
	}
	stored, err := NewEnrollmentRepo(db).Get(ctx, e.ID)
	if err != nil || stored.Uses != 0 {
		t.Fatal("enrollment consumed on failed join")
	}
	if _, err = NewAgentRepo(db).Get(ctx, a.Name); err == nil {
		t.Fatal("orphan agent survived rollback")
	}
	if _, err = db.Exec(`DROP TRIGGER fail_project_join`); err != nil {
		t.Fatal(err)
	}
	if err = repo.Use(ctx, v.ID, a, now); err != nil {
		t.Fatal(err)
	}
	if err = repo.Use(ctx, v.ID, a, now); err == nil {
		t.Fatal("invitation consumed twice")
	}
	ms, err := NewProjectRepo(db).Members(ctx, p.ID)
	if err != nil || len(ms) != 2 {
		t.Fatal("missing membership")
	}
}
