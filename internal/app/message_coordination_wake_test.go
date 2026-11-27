package app_test

import (
	"context"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/domain"
	"github.com/myco-ai/myco/internal/infra/sqlite"
	"io"
	"log/slog"
	"testing"
	"time"
)

type staleWakeRepo struct {
	domain.MessageRepository
	before func()
}

func (r staleWakeRepo) ListDue(ctx context.Context, name domain.AgentName, now time.Time, limit int) ([]*domain.Message, error) {
	out, err := r.MessageRepository.ListDue(ctx, name, now, limit)
	if r.before != nil {
		r.before()
	}
	return out, err
}

func TestReassignAndPushDeliveryDoNotDiscloseToOldRecipient(t *testing.T) {
	for _, moveFirst := range []bool{true, false} {
		t.Run(map[bool]string{true: "move_wins", false: "wake_wins"}[moveFirst], func(t *testing.T) {
			ctx := context.Background()
			db, err := sqlite.Open(":memory:")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			repo := sqlite.NewMessageRepo(db)
			m, err := domain.NewMessage("task", "sender", "old", domain.TypeTask, "private task body", "", "", "", time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if err := repo.Create(ctx, m); err != nil {
				t.Fatal(err)
			}
			move := func() error {
				_, err := repo.ReassignTask(ctx, m.ID, func(cur *domain.Message) (*domain.AuditEntry, error) {
					cur.To = "new"
					return &domain.AuditEntry{ID: "audit", At: time.Now(), Action: "message.reassign", Target: m.ID}, nil
				})
				return err
			}
			wrapped := staleWakeRepo{MessageRepository: repo}
			if moveFirst {
				wrapped.before = func() {
					if err := move(); err != nil {
						t.Fatal(err)
					}
				}
			}
			wake := &fakeWaker{}
			d := app.NewDispatcher(nil, wrapped, app.NewWakers(wake), app.SystemClock{}, app.DispatchPolicy{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
			if err := d.WakeNow(ctx, &domain.Agent{Name: "old", Delivery: domain.Delivery{Mode: "fake"}}); err != nil {
				t.Fatal(err)
			}
			if moveFirst {
				if wake.count() != 0 {
					t.Fatal("stale push disclosed reassigned task to old recipient")
				}
			} else {
				if wake.count() != 1 {
					t.Fatal("expected a wake")
				}
				if err := move(); !domain.IsCode(err, domain.CodeInvalidState) {
					t.Fatalf("task possibly disclosed by push was reassigned: %v", err)
				}
			}
		})
	}
}
