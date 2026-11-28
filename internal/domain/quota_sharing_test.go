package domain

import (
	"testing"
	"time"
)

func TestResolveQuotaKeepsLocalConnectionStatus(t *testing.T) {
	now := time.Now().UTC()
	reset := now.Add(time.Hour)
	for _, status := range []string{"ok", "auth_required", "no_credentials", "transient_error"} {
		t.Run(status, func(t *testing.T) {
			a := &Agent{ID: "target", Runtime: Runtime{CLI: "claude", Account: "demo@example.com", Quota: &Quota{Provider: "claude", Plan: "claude_subscription", Status: status, At: now.Add(-time.Minute), Windows: []QuotaWindow{{Name: "five_hour", Used: .1}}}}}
			shared := &Quota{Provider: "claude", Plan: "claude_subscription", Status: "ok", At: now, ReportedBy: "donor", ReportedAt: now, Windows: []QuotaWindow{{Name: "five_hour", Used: .9, ResetsAt: &reset}}}
			got := ResolveQuotaNumbers(a, nil, shared, now)
			if got.Status != status || got.At != now || got.Windows[0].Used != .9 || !got.Shared {
				t.Fatalf("wrong shared state: %+v", got)
			}
			shared.At = now.Add(-2 * time.Hour)
			got = ResolveQuotaNumbers(a, nil, shared, now)
			if got.Windows[0].Used != .1 || got.Shared {
				t.Fatal("expired shared numbers won")
			}
			shared.At = now
			shared.Windows[0].ResetsAt = &shared.At
			got = ResolveQuotaNumbers(a, nil, shared, now)
			if got.Windows[0].Used != .1 {
				t.Fatal("reset shared window won")
			}
		})
	}
}
func TestSubscriptionQuotaContextDoesNotBorrowOAuthForAPI(t *testing.T) {
	for _, rt := range []Runtime{
		{CLI: "claude", Account: "a@example.com"},
		{CLI: "claude", Account: "a@example.com", ProviderHost: "gateway.example", Quota: &Quota{Provider: "claude", Plan: "claude_subscription"}},
		{CLI: "claude", Quota: &Quota{Provider: "glm", Plan: "coding_plan"}},
		{CLI: "claude", Quota: &Quota{Provider: "claude", Plan: "api"}},
		{CLI: "claude", Quota: &Quota{Provider: "claude", Plan: "claude_subscription", Status: "unsupported"}},
		{CLI: "claude", Quota: &Quota{Provider: "claude", SourceKind: "statusline"}},
	} {
		if p, _ := SubscriptionQuotaContext(&Agent{Runtime: rt}); p != "" {
			t.Fatal("unqualified/API context shared")
		}
	}
	if p, plan := SubscriptionQuotaContext(&Agent{Binding: Binding{Harness: "claude-cloud"}, Runtime: Runtime{Account: "a@example.com"}}); p != "claude" || plan != "claude_subscription" {
		t.Fatal("legacy cloud identity unavailable")
	}
}
