package sqlite

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/domain"
)

func TestAccountQuotaAcrossMachinesAndCloud(t *testing.T) {
	db, e := Open(filepath.Join(t.TempDir(), "hub.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	repo := NewAgentRepo(db)
	ctx := context.Background()
	now := time.Now().UTC()
	makeAgent := func(name, owner, machine string) *domain.Agent {
		a, e := domain.NewAgent("id-"+name, domain.AgentName(name), domain.AgentSpec{}, "token-"+name, now)
		if e != nil {
			t.Fatal(e)
		}
		a.Owner = owner
		a.MachineID = machine
		a.Binding.Harness = "claude-cloud"
		if e = repo.Create(ctx, a); e != nil {
			t.Fatal(e)
		}
		return a
	}
	a := makeAgent("one", "alice", "mac-one")
	b := makeAgent("two", "alice", "win-two")
	c := makeAgent("cloud", "alice", "")
	other := makeAgent("other", "bob", "")
	runtime := func(email string, at time.Time, used float64) domain.Runtime {
		return domain.Runtime{CLI: "claude-cloud", Account: email, Quota: &domain.Quota{Source: "claude", Provider: "claude", Plan: "claude_subscription", Status: "ok", At: at, CheckedAt: now, Windows: []domain.QuotaWindow{{Name: "five_hour", Used: used}}}}
	}
	put := func(a *domain.Agent, rt domain.Runtime) {
		t.Helper()
		if e = repo.SetRuntime(ctx, a.Name, rt); e != nil {
			t.Fatal(e)
		}
		fresh, e := repo.Get(ctx, a.Name)
		if e != nil {
			t.Fatal(e)
		}
		*a = *fresh
	}
	put(a, runtime(" Demo@Example.com ", now, .8))
	for _, target := range []*domain.Agent{b, c, other} {
		put(target, domain.Runtime{CLI: "claude-cloud", Account: "demo@example.com"})
	}
	for _, target := range []*domain.Agent{b, c} {
		q, e := repo.GetAccountQuota(ctx, target.Owner, target)
		if e != nil || q.Windows[0].Used != .8 {
			t.Fatalf("cross machine/cloud unavailable: %v", e)
		}
	}
	if _, e = repo.GetAccountQuota(ctx, other.Owner, other); !domain.IsCode(e, domain.CodeNotFound) {
		t.Fatal("cross-owner merge")
	}
	put(b, runtime("demo@example.com", now.Add(-time.Minute), .1))
	q, e := repo.GetAccountQuota(ctx, "alice", b)
	if e != nil || q.Windows[0].Used != .8 {
		t.Fatal("older report overwrote newer")
	}
	// Auth failures remain local; another account's failure is not published.
	failed := runtime("demo@example.com", now, .1)
	failed.Quota.Status = "auth_required"
	failed.Quota.Windows = nil
	put(b, failed)
	q, e = repo.GetAccountQuota(ctx, "alice", b)
	if e != nil || q.Windows[0].Used != .8 {
		t.Fatal("auth failure overwrote snapshot")
	}
	resolved := domain.ResolveQuotaNumbers(b, nil, q, now)
	if resolved.Status != "auth_required" || !resolved.Shared || resolved.Windows[0].Used != .8 {
		t.Fatal("shared numbers replaced local status")
	}
	for _, tc := range []struct {
		name string
		rt   domain.Runtime
	}{
		{"provider", domain.Runtime{CLI: "codex", Account: "demo@example.com", Quota: &domain.Quota{Source: "codex", Provider: "codex", Plan: "chatgpt_subscription"}}},
		{"plan", domain.Runtime{CLI: "claude-cloud", Account: "demo@example.com", Quota: &domain.Quota{Source: "claude", Provider: "claude", Plan: "api"}}},
		{"missing", domain.Runtime{CLI: "claude-cloud"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			put(c, tc.rt)
			if _, e = repo.GetAccountQuota(ctx, "alice", c); !domain.IsCode(e, domain.CodeNotFound) {
				t.Fatal("incompatible group merged")
			}
		})
	}
	var raw string
	if e = db.QueryRow(`SELECT account_key FROM quota_account_snapshots LIMIT 1`).Scan(&raw); e != nil || len(raw) != 64 || strings.Contains(raw, "demo") {
		t.Fatal("invalid private key")
	}
}

func TestAccountQuotaStableIdentityConflict(t *testing.T) {
	db, e := Open(":memory:")
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	repo := NewAgentRepo(db)
	ctx := context.Background()
	now := time.Now().UTC()
	var agents []*domain.Agent
	for _, name := range []string{"a", "b", "legacy"} {
		a, _ := domain.NewAgent(name, domain.AgentName(name), domain.AgentSpec{}, "token"+name, now)
		a.Owner = "owner"
		a.Binding.Harness = "claude-cloud"
		repo.Create(ctx, a)
		agents = append(agents, a)
	}
	rt := domain.Runtime{CLI: "claude-cloud", Account: "same@example.com", AccountIDHash: "qa_" + strings.Repeat("a", 64), Quota: &domain.Quota{Provider: "claude", Source: "claude", Plan: "claude_subscription", Status: "ok", At: now, Windows: []domain.QuotaWindow{{Name: "five_hour", Used: .7}}}}
	if e = repo.SetRuntime(ctx, "a", rt); e != nil {
		t.Fatal(e)
	}
	legacy := agents[2]
	legacy.Runtime = domain.Runtime{CLI: "claude-cloud", Account: "same@example.com"}
	if q, e := repo.GetAccountQuota(ctx, "owner", legacy); e != nil || q.Windows[0].Used != .7 {
		t.Fatal("email-only legacy target not resolved")
	}
	rt.AccountIDHash = "qa_" + strings.Repeat("b", 64)
	rt.Quota.At = now.Add(time.Second)
	rt.Quota.Windows[0].Used = .2
	if e = repo.SetRuntime(ctx, "b", rt); e != nil {
		t.Fatal(e)
	}
	if _, e = repo.GetAccountQuota(ctx, "owner", legacy); !domain.IsCode(e, domain.CodeNotFound) {
		t.Fatal("ambiguous email still shared")
	}
	a, _ := repo.Get(ctx, "a")
	q, e := repo.GetAccountQuota(ctx, "owner", a)
	if e != nil || q.Windows[0].Used != .7 {
		t.Fatal("different stable IDs merged")
	}
}

func TestAccountQuotaMigrationBackfillsAndKeepsHubKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	repo := NewAgentRepo(db)
	now := time.Now().UTC()
	a, _ := domain.NewAgent("id", "legacy", domain.AgentSpec{}, "token", now)
	a.Owner = "owner"
	a.Binding.Harness = "claude-cloud"
	repo.Create(context.Background(), a)
	rt := domain.Runtime{Account: "demo@example.com", Quota: &domain.Quota{Source: "claude", At: now, Windows: []domain.QuotaWindow{{Name: "five_hour", Used: .9}}}}
	data, _ := json.Marshal(rt)
	db.Exec(`UPDATE agents SET runtime=?`, string(data))
	for _, table := range []string{"quota_account_snapshots", "quota_account_aliases", "quota_account_secret"} {
		if _, e = db.Exec("DROP TABLE " + table); e != nil {
			t.Fatal(e)
		}
	}
	db.Close()
	db, e = Open(path)
	if e != nil {
		t.Fatal(e)
	}
	repo = NewAgentRepo(db)
	a, _ = repo.Get(context.Background(), "legacy")
	q, e := repo.GetAccountQuota(context.Background(), "owner", a)
	if e != nil || q.Windows[0].Used != .9 {
		t.Fatalf("migration did not backfill: %v", e)
	}
	var key []byte
	db.QueryRow(`SELECT secret FROM quota_account_secret`).Scan(&key)
	db.Close()
	db, e = Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	var again []byte
	db.QueryRow(`SELECT secret FROM quota_account_secret`).Scan(&again)
	if string(key) != string(again) {
		t.Fatal("Hub key changed on reopen")
	}
}

func TestAccountQuotaRuntimeAndSnapshotRollbackTogether(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	repo := NewAgentRepo(db)
	now := time.Now().UTC()
	a, err := domain.NewAgent("id-atomic", "atomic", domain.AgentSpec{}, "demo-token", now)
	if err != nil {
		t.Fatal(err)
	}
	a.Owner, a.Binding.Harness = "owner", "claude-cloud"
	if err = repo.Create(ctx, a); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TRIGGER reject_runtime BEFORE UPDATE OF runtime ON agents BEGIN SELECT RAISE(ABORT,'test runtime failure'); END`); err != nil {
		t.Fatal(err)
	}
	rt := domain.Runtime{CLI: "claude-cloud", Account: "demo@example.com", AccountIDHash: "qa_" + strings.Repeat("a", 64), Quota: &domain.Quota{Source: "claude", At: now, Windows: []domain.QuotaWindow{{Name: "five_hour", Used: .5}}}}
	if err = repo.SetRuntime(ctx, a.Name, rt); err == nil {
		t.Fatal("runtime failure not returned")
	}
	for _, table := range []string{"quota_account_snapshots", "quota_account_aliases"} {
		var count int
		if err = db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("partial account write in %s: %d, %v", table, count, err)
		}
	}
}
