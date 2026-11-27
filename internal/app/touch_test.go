package app_test

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/domain"
	"github.com/myco-ai/myco/internal/infra/security"
	"github.com/myco-ai/myco/internal/infra/sqlite"
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

// Hold every authentication after it reads the same old last_seen_at.
// This deterministically reproduces the read/check/write race without sleeps.
type concurrentAuthReads struct {
	domain.AgentRepository
	loaded  chan struct{}
	release <-chan struct{}
}

func (r concurrentAuthReads) GetByTokenHash(ctx context.Context, hash string) (*domain.Agent, error) {
	a, err := r.AgentRepository.GetByTokenHash(ctx, hash)
	if err != nil {
		return nil, err
	}
	r.loaded <- struct{}{}
	select {
	case <-r.release:
		return a, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Each request gets a distinct persisted millisecond, as with a real clock.
type authTickClock struct {
	start time.Time
	n     atomic.Int64
}

func (c *authTickClock) Now() time.Time {
	return c.start.Add(time.Duration(c.n.Add(1)) * time.Millisecond)
}

func TestAuthenticateCoalescesConcurrentStaleHeartbeats(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := sqlite.NewAgentRepo(db)
	clock := &stepClock{now: time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)}
	plain := app.NewAgentService(repo, security.Tokens{}, security.IDs{}, clock, time.Minute, nil)
	reg, err := plain.Register(t.Context(), app.RegisterInput{Name: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	clock.now = clock.now.Add(30 * time.Second)
	const requests = 16
	loaded := make(chan struct{}, requests)
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	authClock := &authTickClock{start: clock.now}
	svc := app.NewAgentService(concurrentAuthReads{repo, loaded, release}, security.Tokens{}, security.IDs{}, authClock, time.Minute, nil)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	changes := func() int64 {
		t.Helper()
		var n int64
		if e := db.QueryRow(`SELECT total_changes()`).Scan(&n); e != nil {
			t.Fatal(e)
		}
		return n
	}
	before := changes()
	results := make(chan error, requests)
	for i := 0; i < requests; i++ {
		go func() { _, e := svc.Authenticate(ctx, reg.Token); results <- e }()
	}
	for i := 0; i < requests; i++ {
		select {
		case <-loaded:
		case <-ctx.Done():
			t.Fatal("authentication reads did not reach the barrier")
		}
	}
	unblock()
	for i := 0; i < requests; i++ {
		select {
		case e := <-results:
			if e != nil {
				t.Fatal(e)
			}
		case <-ctx.Done():
			t.Fatal("authentication did not finish")
		}
	}
	if delta := changes() - before; delta != 1 {
		t.Fatalf("concurrent stale snapshots caused %d writes, want one", delta)
	}
	a, err := repo.Get(t.Context(), "demo")
	if err != nil || !a.LastSeenAt.After(clock.now) || a.LastSeenAt.After(clock.now.Add(requests*time.Millisecond)) {
		t.Fatalf("last seen: %+v err=%v", a, err)
	}
	// Authentication itself is never cached: a sign-out still takes effect.
	if _, err = plain.Kick(t.Context(), "demo", false); err != nil {
		t.Fatal(err)
	}
	if _, err = plain.Authenticate(t.Context(), reg.Token); !domain.IsCode(err, domain.CodeUnauthenticated) {
		t.Fatalf("revoked token accepted: %v", err)
	}
	a, err = repo.Get(t.Context(), "demo")
	if err != nil || !a.LastSeenAt.IsZero() {
		t.Fatalf("sign-out did not clear presence: %+v %v", a, err)
	}
}
