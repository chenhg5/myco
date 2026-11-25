package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/domain"
	"github.com/myco-ai/myco/internal/infra/sqlite"
)

func TestWakeHeldRecoversLegacyHoldOnce(t *testing.T) {
	ctx := context.Background()
	repo := sqlite.NewMessageRepo(openMem(t))
	for _, id := range []string{"orphan", "question", "manual", "recent"} {
		m := newMsg(t, id, "alice", "bob", domain.TypeTask, t0)
		m.Status = domain.StatusHeld
		switch id {
		case "question":
			m.Asking = "q"
		case "manual":
			m.HoldUntil = domain.ManualHold
		case "recent":
			m.UpdatedAt = t0.Add(20 * time.Minute)
		}
		mustNoErr(t, repo.Create(ctx, m))
	}
	recovery := func(m *domain.Message) *domain.Message {
		return must(m.Progress("bob", "one-recovery", "automatically resumed", t0.Add(31*time.Minute)))
	}
	at := t0.Add(31 * time.Minute)
	woke, progress, err := repo.WakeHeld(ctx, "bob", at, at.Add(-30*time.Minute), 10, recovery)
	mustNoErr(t, err)
	if len(woke) != 1 || woke[0].ID != "orphan" || woke[0].HoldWake != domain.RecoveredHold ||
		woke[0].Status != domain.StatusQueued || len(progress) != 1 {
		t.Fatalf("woke %v, progress %v", ids(woke), ids(progress))
	}
	_, err = repo.Mutate(ctx, "orphan", func(m *domain.Message) ([]*domain.Message, error) {
		m.Status = domain.StatusHeld
		return nil, nil
	})
	mustNoErr(t, err)
	woke, progress, err = repo.WakeHeld(ctx, "bob", at, at.Add(-30*time.Minute), 10, recovery)
	mustNoErr(t, err)
	if len(woke) != 0 || len(progress) != 0 {
		t.Fatalf("repeated recovery: %v", ids(woke))
	}
}
