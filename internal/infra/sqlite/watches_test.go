package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/domain"
	"github.com/myco-ai/myco/internal/infra/sqlite"
)

func TestWatchMigrationAndStaleExpiry(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sqlite.Open(path)
	mustNoErr(t, err)
	old := newMsg(t, "old-message", "sender", "watcher", domain.TypeTask, t0)
	mustNoErr(t, sqlite.NewMessageRepo(db).Create(ctx, old))
	_, err = db.Exec(`DROP TABLE watches`)
	mustNoErr(t, err)
	mustNoErr(t, db.Close())
	db, err = sqlite.Open(path)
	mustNoErr(t, err)
	defer db.Close()
	mr, wr := sqlite.NewMessageRepo(db), sqlite.NewWatchRepo(db)
	_, err = mr.Get(ctx, "old-message")
	mustNoErr(t, err)
	ws, err := wr.List(ctx, true)
	mustNoErr(t, err)
	if len(ws) != 0 {
		t.Fatal(ws)
	}
	original := domain.Watch{ID: "wat_test", Agent: "watcher", What: "Child task", Kind: "task", Target: "child", State: domain.WatchWatching, StartedAt: t0, Deadline: t0.Add(time.Minute)}
	mustNoErr(t, wr.Create(ctx, &original))
	extended := original
	extended.Deadline = t0.Add(time.Hour)
	saved, _, _, err := wr.Save(ctx, &extended, original.Deadline, original.Checks, nil)
	mustNoErr(t, err)
	if !saved {
		t.Fatal("extend lost")
	}
	stale := original
	mustNoErr(t, stale.End(original.Deadline, domain.WatchExpired, "hub", "expired"))
	wake := newMsg(t, "late-wake", "watcher", "watcher", domain.TypeEvent, original.Deadline)
	saved, _, _, err = wr.Save(ctx, &stale, original.Deadline, original.Checks, wake)
	mustNoErr(t, err)
	if saved {
		t.Fatal("stale expiry overwrote extension")
	}
	_, err = mr.Get(ctx, wake.ID)
	wantCode(t, err, domain.CodeNotFound)
	current, err := wr.Get(ctx, original.ID)
	mustNoErr(t, err)
	if current.State != domain.WatchWatching || !current.Deadline.Equal(extended.Deadline) {
		t.Fatal(current)
	}
	mustNoErr(t, current.End(t0.Add(2*time.Minute), domain.WatchStopped, "owner", "stop"))
	saved, _, _, err = wr.Save(ctx, current, extended.Deadline, extended.Checks, wake)
	mustNoErr(t, err)
	if !saved {
		t.Fatal("stop not saved")
	}
	saved, _, _, err = wr.Save(ctx, current, extended.Deadline, extended.Checks, wake)
	mustNoErr(t, err)
	if saved {
		t.Fatal("duplicate terminal transition")
	}
}

func TestWatchExpiryWinsOverLegacyHoldWake(t *testing.T) {
	ctx := context.Background()
	db := openMem(t)
	mr, wr := sqlite.NewMessageRepo(db), sqlite.NewWatchRepo(db)
	parent := newMsg(t, "parent", "sender", "watcher", domain.TypeTask, t0)
	parent.Status = domain.StatusHeld
	parent.HoldUntil = "child"
	parent.Attempts = 1
	child := newMsg(t, "child", "watcher", "worker", domain.TypeTask, t0)
	child.Status = domain.StatusDone
	mustNoErr(t, mr.Create(ctx, parent))
	mustNoErr(t, mr.Create(ctx, child))
	w := &domain.Watch{ID: "wat_expired", Agent: "watcher", Task: "parent", Kind: "task", Target: "child", State: domain.WatchWatching, StartedAt: t0, Deadline: t0.Add(time.Minute)}
	mustNoErr(t, wr.Create(ctx, w))
	at := t0.Add(2 * time.Minute)
	changed, _, err := mr.WakeHeld(ctx, "watcher", at, t0, 10, func(*domain.Message) *domain.Message { t.Fatal("unexpected legacy recovery"); return nil })
	mustNoErr(t, err)
	if len(changed) != 0 {
		t.Fatal("runner woke expired watch as met")
	}
	mustNoErr(t, w.End(at, domain.WatchExpired, "hub", "expired"))
	wake := newMsg(t, "expiry", "watcher", "watcher", domain.TypeEvent, at)
	saved, p, n, err := wr.Save(ctx, w, w.Deadline, 0, wake)
	mustNoErr(t, err)
	if !saved || p == nil || p.Status != domain.StatusQueued || p.Attempts != 0 || p.HoldWake != wake.ID || n.Status != domain.StatusDone {
		t.Fatalf("expiry wake: %v %+v %+v", saved, p, n)
	}
}

func TestWatchWakeInsertionFailureRollsBack(t *testing.T) {
	ctx := context.Background()
	db := openMem(t)
	mr, wr := sqlite.NewMessageRepo(db), sqlite.NewWatchRepo(db)
	parent := newMsg(t, "parent", "sender", "watcher", domain.TypeTask, t0)
	parent.Status = domain.StatusHeld
	parent.HoldUntil = "child"
	parent.Attempts = 1
	mustNoErr(t, mr.Create(ctx, parent))
	existing := newMsg(t, "wake", "watcher", "watcher", domain.TypeEvent, t0)
	mustNoErr(t, mr.Create(ctx, existing))
	w := &domain.Watch{ID: "wat_rollback", Agent: "watcher", Task: "parent", Kind: "task", Target: "child", State: domain.WatchWatching, StartedAt: t0, Deadline: t0.Add(time.Minute)}
	mustNoErr(t, wr.Create(ctx, w))
	mustNoErr(t, w.End(t0.Add(2*time.Minute), domain.WatchExpired, "hub", "deadline"))
	if _, _, _, err := wr.Save(ctx, w, w.Deadline, 0, existing); err == nil {
		t.Fatal("duplicate wake insertion succeeded")
	}
	current, err := wr.Get(ctx, w.ID)
	mustNoErr(t, err)
	if current.State != domain.WatchWatching {
		t.Fatal("watch terminal state escaped rollback")
	}
	p, err := mr.Get(ctx, "parent")
	mustNoErr(t, err)
	if p.Status != domain.StatusHeld || p.HoldUntil != "child" || p.HoldWake != "" || p.Attempts != 1 {
		t.Fatalf("parent escaped rollback: %+v", p)
	}
}
