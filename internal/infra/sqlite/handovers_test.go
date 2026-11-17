package sqlite_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/domain"
	"github.com/chenhg5/myco/internal/infra/sqlite"
)

// Each take-over retires the credential it replaces (at most
// MaxRetiredTokens kept per agent), makes the new one current, and uses its
// link once; other waiting links of the agent stop working.
func TestHandoverRepoTakeOver(t *testing.T) {
	ctx := context.Background()
	db := openMem(t)
	agents, repo := sqlite.NewAgentRepo(db), sqlite.NewHandoverRepo(db)
	mustNoErr(t, agents.Create(ctx, newAgent(t, "docs", "tok-0", domain.AgentSpec{Binding: domain.Binding{Harness: "claude", SessionID: "s-0"}})))
	for i := 1; i <= domain.MaxRetiredTokens+3; i++ {
		h := &domain.Handover{ID: fmt.Sprintf("hov_%02d", i), Agent: "docs", At: t0, State: domain.HandoverWaiting, ExpiresAt: t0.Add(domain.HandoverLinkTTL)}
		mustNoErr(t, repo.Create(ctx, h, fmt.Sprintf("link-%d", i)))
		if i == 1 {
			// A second waiting link stops working once the first is taken.
			mustNoErr(t, repo.Create(ctx, &domain.Handover{ID: "hov_spare", Agent: "docs", At: t0, State: domain.HandoverWaiting, ExpiresAt: t0.Add(domain.HandoverLinkTTL)}, "link-spare"))
		}
		got, err := repo.TakeOver(ctx, domain.TakeOverInput{LinkHash: fmt.Sprintf("link-%d", i), Binding: domain.Binding{Harness: "codex", SessionID: fmt.Sprintf("s-%d", i)},
			TokenHash: fmt.Sprintf("tok-%d", i), Now: t0.Add(time.Duration(i) * time.Minute)})
		mustNoErr(t, err)
		if got.State != domain.HandoverDone || got.From.Binding.SessionID != fmt.Sprintf("s-%d", i-1) || got.To.Binding.SessionID != fmt.Sprintf("s-%d", i) {
			t.Fatalf("take-over %d: %+v", i, got)
		}
		if _, err := repo.TakeOver(ctx, domain.TakeOverInput{LinkHash: fmt.Sprintf("link-%d", i), Binding: domain.Binding{Harness: "codex", SessionID: "x"}, TokenHash: "x", Now: t0}); err == nil {
			t.Fatal("a link works once")
		}
	}
	if _, err := repo.PeekLink(ctx, "link-spare", t0); err == nil {
		t.Fatal("the spare link should have stopped working")
	}
	a, err := agents.GetByTokenHash(ctx, fmt.Sprintf("tok-%d", domain.MaxRetiredTokens+3))
	if err != nil || a.Binding.SessionID != fmt.Sprintf("s-%d", domain.MaxRetiredTokens+3) {
		t.Fatalf("current: %v %+v", err, a)
	}
	if h, err := repo.Retired(ctx, "tok-5"); err != nil || h.ID != "hov_06" {
		t.Fatalf("retired tok-5: %v %+v", err, h)
	}
	if _, err := repo.Retired(ctx, "tok-0"); err == nil {
		t.Fatal("only the newest retired credentials are kept")
	}
}
