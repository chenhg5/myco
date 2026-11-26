package sqlite_test

import (
	"context"
	"errors"
	"github.com/myco-ai/myco/internal/domain"
	"github.com/myco-ai/myco/internal/infra/sqlite"
	"path/filepath"
	"testing"
)

func TestGoalMigrationAtomicWakeAndRollback(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "old.db")
	db, e := sqlite.Open(path)
	mustNoErr(t, e)
	old := newMsg(t, "old", "sender", "worker", domain.TypeTask, t0)
	mustNoErr(t, sqlite.NewMessageRepo(db).Create(ctx, old))
	_, e = db.Exec(`DROP TABLE goals`)
	mustNoErr(t, e)
	mustNoErr(t, db.Close())
	db, e = sqlite.Open(path)
	mustNoErr(t, e)
	defer db.Close()
	_, e = sqlite.NewMessageRepo(db).Get(ctx, "old")
	mustNoErr(t, e)
	repo := sqlite.NewGoalRepo(db)
	g := &domain.GoalRecord{Goal: domain.Goal{ID: "gol_demo", Agent: "worker", State: "active"}, Rounds: []domain.GoalRound{}}
	mustNoErr(t, repo.Create(ctx, g))
	_, _, e = repo.Mutate(ctx, g.Goal.ID, func(r *domain.GoalRecord, _ *domain.Message, _ bool) ([]*domain.Message, error) {
		r.Goal.Round++
		return nil, errors.New("abort")
	})
	if e == nil {
		t.Fatal("rollback error disappeared")
	}
	got, e := repo.Get(ctx, g.Goal.ID)
	mustNoErr(t, e)
	if got.Goal.Round != 0 {
		t.Fatal("mutation survived rollback")
	}
	// Insertion failure must roll back the round as well.
	_, _, e = repo.Mutate(ctx, g.Goal.ID, func(r *domain.GoalRecord, _ *domain.Message, _ bool) ([]*domain.Message, error) {
		r.Goal.Round++
		return []*domain.Message{old}, nil
	})
	if e == nil {
		t.Fatal("duplicate message accepted")
	}
	got, e = repo.Get(ctx, g.Goal.ID)
	mustNoErr(t, e)
	if got.Goal.Round != 0 {
		t.Fatal("round committed without wake")
	}
}
