package sqlite_test

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/domain"
	"github.com/chenhg5/myco/internal/infra/sqlite"
)

func TestMessageAfterGateAndMigration(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "hub.db")
	db, err := sqlite.Open(path)
	mustNoErr(t, err)
	// Simulate a database from before the after column existed.
	_, err = db.Exec(`DROP INDEX idx_messages_after`)
	mustNoErr(t, err)
	_, err = db.Exec(`ALTER TABLE messages DROP COLUMN after`)
	mustNoErr(t, err)
	mustNoErr(t, db.Close())
	db, err = sqlite.Open(path)
	mustNoErr(t, err)
	defer db.Close()
	r := sqlite.NewMessageRepo(db)
	parent := newMsg(t, "parent", "alice", "bob", domain.TypeTask, t0)
	child := newMsg(t, "child", "alice", "bob", domain.TypeTask, t0.Add(time.Second))
	child.After = parent.ID
	mustNoErr(t, r.Create(ctx, parent))
	mustNoErr(t, r.Create(ctx, child))
	stored, err := r.Get(ctx, child.ID)
	mustNoErr(t, err)
	if stored.After != parent.ID {
		t.Fatalf("after not persisted: %+v", stored)
	}
	if due, err := r.ListDue(ctx, "bob", t0.Add(time.Minute), 10); err != nil || !slices.Equal(ids(due), []string{"parent"}) {
		t.Fatalf("blocked child due: %v %v", ids(due), err)
	}
	counts, err := r.OpenCounts(ctx)
	mustNoErr(t, err)
	if counts["bob"][domain.StatusQueued] != 1 {
		t.Fatalf("blocked child counted in overview: %v", counts)
	}
	_, err = r.Mutate(ctx, parent.ID, func(m *domain.Message) ([]*domain.Message, error) {
		return nil, m.Stop("alice", t0.Add(time.Minute))
	})
	mustNoErr(t, err)
	if due, err := r.ListDue(ctx, "bob", t0.Add(time.Minute), 10); err != nil || !slices.Equal(ids(due), []string{"child"}) {
		t.Fatalf("released child due: %v %v", ids(due), err)
	}
	counts, err = r.OpenCounts(ctx)
	mustNoErr(t, err)
	if counts["bob"][domain.StatusQueued] != 1 {
		t.Fatalf("released child not counted: %v", counts)
	}
}
