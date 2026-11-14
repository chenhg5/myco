package app_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/infra/security"
	"github.com/chenhg5/myco/internal/infra/sqlite"
)

// Every request with an agent token goes through Authenticate; it must not
// write to the database each time.
func TestAuthenticateWritesLastSeenAtMostEvery15Seconds(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	clock := &stepClock{now: time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)}
	repo := sqlite.NewAgentRepo(db)
	svc := app.NewAgentService(repo, security.Tokens{}, security.IDs{}, clock, 90*time.Second, nil)
	ctx := context.Background()
	reg, err := svc.Register(ctx, app.RegisterInput{Name: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	stored := func() time.Time {
		t.Helper()
		a, err := repo.Get(ctx, "alice")
		if err != nil {
			t.Fatal(err)
		}
		return a.LastSeenAt
	}
	start := clock.now
	if a, err := svc.Authenticate(ctx, reg.Token); err != nil || !a.LastSeenAt.Equal(start) || !stored().Equal(start) {
		t.Fatalf("first call not recorded: %v, stored %v", err, stored())
	}
	clock.now = start.Add(10 * time.Second)
	a, err := svc.Authenticate(ctx, reg.Token)
	if err != nil || !stored().Equal(start) {
		t.Fatalf("last seen written again within 15 s: %v, stored %v", err, stored())
	}
	if !a.IsOnline(clock.now, svc.OnlineWindow()) {
		t.Fatal("agent not online between two writes")
	}
	clock.now = start.Add(15 * time.Second)
	if _, err := svc.Authenticate(ctx, reg.Token); err != nil || !stored().Equal(clock.now) {
		t.Fatalf("last seen not written after 15 s: %v, stored %v", err, stored())
	}

	// A short online window shortens the interval: an agent calling every
	// few seconds must not drop offline between writes.
	short := app.NewAgentService(repo, security.Tokens{}, security.IDs{}, clock, 8*time.Second, nil)
	at := clock.now
	clock.now = at.Add(2 * time.Second)
	if _, err := short.Authenticate(ctx, reg.Token); err != nil || !stored().Equal(clock.now) {
		t.Fatalf("with an 8 s window, last seen not written after 2 s: %v, stored %v", err, stored())
	}

	// Signed out: offline at once, not when the window runs out.
	if _, err := svc.Kick(ctx, "alice", false); err != nil {
		t.Fatal(err)
	}
	if !stored().IsZero() {
		t.Fatalf("kicked agent still has a last seen: %v", stored())
	}
}
