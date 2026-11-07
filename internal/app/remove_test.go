package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/domain"
	"github.com/chenhg5/myco/internal/infra/security"
	"github.com/chenhg5/myco/internal/infra/sqlite"
)

// agentRepoFailingDelete simulates the hub stopping between the two deletes
// of an agent removal: the agent row survives.
type agentRepoFailingDelete struct{ *sqlite.AgentRepo }

func (agentRepoFailingDelete) Delete(context.Context, domain.AgentName) error {
	return errors.New("hub stopped")
}

func aliasFixture(t *testing.T, repo domain.AgentRepository, db *sqlite.AliasRepo) *app.AgentService {
	t.Helper()
	s := app.NewAgentService(repo, security.Tokens{}, security.IDs{}, app.SystemClock{}, time.Minute, nil)
	s.SetAliases(db)
	return s
}

func TestRemoveAgentRemovesItsAliases(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	aliases := sqlite.NewAliasRepo(db)
	s := aliasFixture(t, sqlite.NewAgentRepo(db), aliases)
	if _, err := s.Register(ctx, app.RegisterInput{Name: "bob"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetAlias(ctx, "reviewer", "bob", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Remove(ctx, "bob", false); err != nil {
		t.Fatal(err)
	}
	if _, err := aliases.Get(ctx, "reviewer"); domain.CodeOf(err) != domain.CodeNotFound {
		t.Fatalf("alias survived its agent: %v", err)
	}
	// A new agent with the same name must not inherit the old alias.
	if _, err := s.Register(ctx, app.RegisterInput{Name: "bob"}); err != nil {
		t.Fatal(err)
	}
	if all, _ := aliases.List(ctx); len(all) != 0 {
		t.Fatalf("aliases after re-registering bob: %+v", all)
	}
}

// If removal is interrupted after the first step, what is left must be
// harmless: the agent without its aliases, never aliases without their agent.
func TestInterruptedRemoveNeverLeavesDanglingAliases(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	agents, aliases := sqlite.NewAgentRepo(db), sqlite.NewAliasRepo(db)
	s := aliasFixture(t, agents, aliases)
	if _, err := s.Register(ctx, app.RegisterInput{Name: "bob"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetAlias(ctx, "reviewer", "bob", false); err != nil {
		t.Fatal(err)
	}
	broken := aliasFixture(t, agentRepoFailingDelete{agents}, aliases)
	if _, err := broken.Remove(ctx, "bob", false); err == nil {
		t.Fatal("expected the simulated failure")
	}
	if _, err := agents.Get(ctx, "bob"); err != nil {
		t.Fatalf("agent should still exist after the interrupted removal: %v", err)
	}
	if _, err := aliases.Get(ctx, "reviewer"); domain.CodeOf(err) != domain.CodeNotFound {
		t.Fatalf("alias must be gone before the agent is: %v", err)
	}
	// Removing again finishes the job.
	if _, err := s.Remove(ctx, "bob", false); err != nil {
		t.Fatal(err)
	}
}
