package app_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/domain"
	"github.com/chenhg5/myco/internal/infra/security"
	"github.com/chenhg5/myco/internal/infra/sqlite"
)

type stepClock struct{ now time.Time }

func (c *stepClock) Now() time.Time { return c.now }

func TestConsoleServiceTouchesAtMostOncePerMinute(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	clock := &stepClock{now: time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)}
	repo := sqlite.NewConsoleTokenRepo(db)
	svc := app.NewConsoleService(repo, security.Tokens{}, security.IDs{}, clock)
	ctx := context.Background()

	if _, _, err := svc.Create(ctx, "  "); !domain.IsCode(err, domain.CodeInvalidArgument) {
		t.Fatalf("blank name: %v", err)
	}
	tok, plain, err := svc.Create(ctx, "glenn-mac")
	if err != nil || !strings.HasPrefix(plain, app.ConsoleTokenPrefix) || tok.TokenHash == "" || strings.Contains(tok.TokenHash, plain) {
		t.Fatalf("create: %v %+v", err, tok)
	}
	if _, ok := svc.Authenticate(ctx, plain); !ok {
		t.Fatal("fresh token rejected")
	}
	first, _ := repo.Get(ctx, tok.ID)
	clock.now = clock.now.Add(30 * time.Second)
	svc.Authenticate(ctx, plain)
	if again, _ := repo.Get(ctx, tok.ID); !again.LastUsedAt.Equal(first.LastUsedAt) {
		t.Fatal("last used written twice within a minute")
	}
	clock.now = clock.now.Add(time.Minute)
	svc.Authenticate(ctx, plain)
	if later, _ := repo.Get(ctx, tok.ID); !later.LastUsedAt.Equal(clock.now) {
		t.Fatalf("last used not updated after a minute: %v", later.LastUsedAt)
	}
	if _, ok := svc.Authenticate(ctx, "myco_"+strings.TrimPrefix(plain, app.ConsoleTokenPrefix)); ok {
		t.Fatal("an agent-style token with the same secret must not pass")
	}
	if _, err := svc.Revoke(ctx, tok.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := svc.Authenticate(ctx, plain); ok {
		t.Fatal("revoked token accepted")
	}
	if _, err := svc.Revoke(ctx, "con_nope"); !domain.IsCode(err, domain.CodeNotFound) {
		t.Fatalf("revoke unknown: %v", err)
	}
}
