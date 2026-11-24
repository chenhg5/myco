package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/domain"
	"github.com/myco-ai/myco/internal/infra/security"
	"github.com/myco-ai/myco/internal/infra/sqlite"
)

// Two computers in a workspace never share a name: renaming one to
// another's name (letter case aside) is refused; its own name is fine.
func TestMachineRenameKeepsNamesUnique(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewMachineRepo(db)
	now := time.Unix(1_000_000, 0)
	for _, m := range []*domain.Machine{
		{ID: "mch_a", FingerprintHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Name: "laptop", CreatedAt: now, LastSeenAt: now},
		{ID: "mch_b", FingerprintHash: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Name: "laptop-2", CreatedAt: now, LastSeenAt: now},
	} {
		if err := repo.Create(ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	s := app.NewMachineService(repo, sqlite.NewAgentRepo(db), nil, security.IDs{}, &fakeClock{t: now}, time.Minute)
	if _, err := s.Rename(ctx, "mch_b", "Laptop"); !domain.IsCode(err, domain.CodeConflict) {
		t.Fatalf("renaming to another computer's name: %v", err)
	}
	if _, err := s.Rename(ctx, "mch_b", "WSL"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Rename(ctx, "mch_a", "LAPTOP"); err != nil {
		t.Fatalf("its own name in other letter case: %v", err)
	}
}
