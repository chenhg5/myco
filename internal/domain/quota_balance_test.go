package domain

import (
	"strings"
	"testing"
	"time"
)

func TestQuotaBalanceValidation(t *testing.T) {
	n, left := 50.0, 17.5
	for _, provider := range []string{"glm", "kimi", "openrouter"} {
		q := Quota{Provider: provider, Source: provider, ProfileRef: "qp_" + strings.Repeat("a", 64), Status: "ok", CheckedAt: time.Now(), Windows: []QuotaWindow{{Name: "balance", Used: .65, Balance: &QuotaBalance{Currency: "USD", Limit: &n, Remaining: &left}}}}
		if e := q.Validate(); e != nil {
			t.Fatal(e)
		}
		reset := time.Now()
		q.Windows[0].ResetsAt = &reset
		if q.Validate() == nil {
			t.Fatal("balance incorrectly accepted a reset")
		}
		q.Windows[0].ResetsAt = nil
		q.Windows[0].Balance.Unlimited = true
		if q.Validate() == nil {
			t.Fatal("unlimited balance retained bounded amounts")
		}
		q.Windows[0].Balance.Limit = nil
		q.Windows[0].Balance.Remaining = nil
		q.Windows[0].Used = 0
		if e := q.Validate(); e != nil {
			t.Fatal(e)
		}
	}
}
