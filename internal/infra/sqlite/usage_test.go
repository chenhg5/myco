package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/domain"
)

func TestUsageUpsertAndKeysAreAtomic(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r := NewUsageRepo(db)
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	ctx := context.Background()
	report := domain.UsageReport{Key: "one", Day: "2026-11-24", Model: "claude", Calls: 1, InputTokens: 5, CacheReadTokens: 9, CacheWriteTokens: 2, OutputTokens: 3, CostUSD: 0.1}
	if n, err := r.Record(ctx, "alice", "session", []domain.UsageReport{report, report}, now); err != nil || n != 1 {
		t.Fatalf("record %d %v", n, err)
	}
	report.Key = "two"
	if n, err := r.Record(ctx, "alice", "session", []domain.UsageReport{report}, now); err != nil || n != 1 {
		t.Fatal(err)
	}
	rows, err := r.List(ctx, domain.UsageFilter{From: "2026-11-24", To: "2026-11-24"})
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows %v %v", rows, err)
	}
	if x := rows[0]; x.Calls != 2 || x.InputTokens != 10 || x.CacheReadTokens != 18 || x.CacheWriteTokens != 4 || x.OutputTokens != 6 || x.CostUSD != 0.2 {
		t.Fatalf("sum %+v", x)
	}
	// Keys are namespaced by agent and source aggregates stay independent.
	if n, err := r.Record(ctx, "bob", "runner", []domain.UsageReport{report}, now); err != nil || n != 1 {
		t.Fatalf("agent namespace %d %v", n, err)
	}
	// A failure after inserting a key must not consume it.
	if _, err := db.Exec(`CREATE TRIGGER fail_usage BEFORE INSERT ON usage_daily BEGIN SELECT RAISE(ABORT,'fixture failure'); END;`); err != nil {
		t.Fatal(err)
	}
	report.Key = "rollback"
	if _, err := r.Record(ctx, "alice", "session", []domain.UsageReport{report}, now); err == nil {
		t.Fatal("write unexpectedly succeeded")
	}
	if _, err := db.Exec("DROP TRIGGER fail_usage"); err != nil {
		t.Fatal(err)
	}
	if n, err := r.Record(ctx, "alice", "session", []domain.UsageReport{report}, now); err != nil || n != 1 {
		t.Fatalf("key consumed by failed write %d %v", n, err)
	}
	// Expired keys are pruned inside the transaction.
	if _, err := r.Record(ctx, "alice", "session", nil, now.AddDate(0, 0, 31)); err != nil {
		t.Fatal(err)
	}
	var keys int
	if err := db.QueryRow("SELECT count(*) FROM usage_keys").Scan(&keys); err != nil || keys != 0 {
		t.Fatalf("expired keys=%d %v", keys, err)
	}
}
