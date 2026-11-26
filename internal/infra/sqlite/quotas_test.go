package sqlite

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/domain"
)

func TestQuotaMigrationNormalizedAndAtomicRuntimeBinding(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	// Simulate an existing database before the additive quota table migration.
	if _, e = db.Exec(`DROP TABLE account_quotas`); e != nil {
		t.Fatal(e)
	}
	db.Close()
	db, e = Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	repo := NewAgentRepo(db)
	ctx := context.Background()
	now := time.Now().UTC()
	for _, name := range []domain.AgentName{"one", "two"} {
		a, e := domain.NewAgent("agt_"+string(name), name, domain.AgentSpec{}, "hash_"+string(name), now)
		if e != nil {
			t.Fatal(e)
		}
		a.Owner = "member"
		a.MachineID = "machine"
		if e = repo.Create(ctx, a); e != nil {
			t.Fatal(e)
		}
	}
	ref := "qp_" + strings.Repeat("a", 64)
	q := &domain.Quota{Provider: "codex", ProfileRef: ref, Source: "codex", Status: "ok", At: now, CheckedAt: now, Windows: []domain.QuotaWindow{{Name: "primary", Used: .82}}}
	for _, name := range []domain.AgentName{"one", "two"} {
		if e = repo.SetRuntime(ctx, name, domain.Runtime{CLI: "codex", Quota: q}); e != nil {
			t.Fatal(e)
		}
	}
	var n int
	if e = db.QueryRow(`SELECT count(*) FROM account_quotas`).Scan(&n); e != nil || n != 1 {
		t.Fatal("one login was duplicated")
	}
	a, e := repo.Get(ctx, "two")
	if e != nil {
		t.Fatal(e)
	}
	if len(a.Runtime.Quota.Windows) != 0 || a.Runtime.Quota.ProfileRef != ref {
		t.Fatal("runtime contains copied quota instead of reference")
	}
	if _, e = repo.GetQuota(ctx, "other-owner", "machine", "codex", ref); !domain.IsCode(e, domain.CodeNotFound) {
		t.Fatal("owner namespace not isolated")
	}
	// Force the runtime update to fail after the shared upsert. Both must roll back.
	if _, e = db.Exec(`CREATE TRIGGER fail_quota_binding BEFORE UPDATE OF runtime ON agents BEGIN SELECT RAISE(ABORT,'fixture'); END;`); e != nil {
		t.Fatal(e)
	}
	changed := *q
	changed.CheckedAt = now.Add(time.Minute)
	changed.Windows = []domain.QuotaWindow{{Name: "primary", Used: .9}}
	if e = repo.SetRuntime(ctx, "one", domain.Runtime{Quota: &changed}); e == nil {
		t.Fatal("expected update failure")
	}
	got, e := repo.GetQuota(ctx, "member", "machine", "codex", ref)
	if e != nil || got.Windows[0].Used != .82 {
		t.Fatal("failed binding changed shared snapshot")
	}
}
