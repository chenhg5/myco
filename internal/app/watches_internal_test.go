package app

import (
	"context"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/domain"
	"github.com/myco-ai/myco/internal/infra/notify"
	"github.com/myco-ai/myco/internal/infra/security"
	"github.com/myco-ai/myco/internal/infra/sqlite"
)

func TestWatchOwnerPermission(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ar, wr := sqlite.NewAgentRepo(db), sqlite.NewWatchRepo(db)
	a, err := domain.NewAgent("agt_watch", "watcher", domain.AgentSpec{}, "test-hash", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	a.Owner = "member-owner"
	if err = ar.Create(ctx, a); err != nil {
		t.Fatal(err)
	}
	w := &domain.Watch{ID: "wat_test", Agent: "watcher", State: domain.WatchWatching, What: "Test", StartedAt: time.Now(), Deadline: time.Now().Add(time.Minute)}
	if err = wr.Create(ctx, w); err != nil {
		t.Fatal(err)
	}
	s := &WatchService{repo: wr, agents: ar}
	for _, by := range []Actor{{Agent: a}, {Member: &domain.Member{ID: "member-owner", Role: domain.RoleMember}}, {Admin: true}} {
		if _, err = s.Get(ctx, by, w.ID); err != nil {
			t.Fatalf("allowed actor %+v: %v", by, err)
		}
	}
	for _, by := range []Actor{{Agent: &domain.Agent{Name: "peer", Owner: a.Owner}}, {Member: &domain.Member{ID: "another-owner", Role: domain.RoleMember}}} {
		if _, err = s.Get(ctx, by, w.ID); !domain.IsCode(err, domain.CodePermissionDenied) {
			t.Fatalf("unrelated actor %+v: %v", by, err)
		}
	}
}

func TestWatchLateEvaluationUsesCompletionTime(t *testing.T) {
	for _, timely := range []bool{true, false} {
		t.Run(map[bool]string{true: "before deadline", false: "after deadline"}[timely], func(t *testing.T) {
			ctx := context.Background()
			db, err := sqlite.Open(":memory:")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			clock := &actionResourceClock{now: now.Add(2 * time.Hour)}
			ar, mr, wr := sqlite.NewAgentRepo(db), sqlite.NewMessageRepo(db), sqlite.NewWatchRepo(db)
			a, err := domain.NewAgent("agt_watch", "watcher", domain.AgentSpec{}, "hash", now)
			if err != nil {
				t.Fatal(err)
			}
			if err = ar.Create(ctx, a); err != nil {
				t.Fatal(err)
			}
			completed := now.Add(2 * time.Hour)
			if timely {
				completed = now.Add(time.Minute)
			}
			child := &domain.Message{ID: "child", From: "watcher", To: "worker", ThreadID: "child", Type: domain.TypeTask, Body: "Check", Status: domain.StatusDone, CreatedAt: now, UpdatedAt: completed}
			if err = mr.Create(ctx, child); err != nil {
				t.Fatal(err)
			}
			w := &domain.Watch{ID: "wat_time", Agent: "watcher", Target: child.ID, Kind: "task", What: "Check", StartedAt: now, Deadline: now.Add(time.Hour), State: domain.WatchWatching}
			if err = wr.Create(ctx, w); err != nil {
				t.Fatal(err)
			}
			bus := NewEventBus()
			messages := NewMessageService(ar, mr, notify.NewMemory(), security.IDs{}, clock, DefaultMessagePolicy())
			s := NewWatchService(wr, ar, messages, nil, nil, nil, bus, security.IDs{}, clock)
			if err = s.Tick(ctx); err != nil {
				t.Fatal(err)
			}
			got, err := wr.Get(ctx, w.ID)
			if err != nil {
				t.Fatal(err)
			}
			want := domain.WatchExpired
			if timely {
				want = domain.WatchMet
			}
			if got.State != want {
				t.Fatalf("state=%s want=%s", got.State, want)
			}
		})
	}
}
