package httpapi

import (
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/domain"
)

func TestQuotaDTOFreshnessAndExpiry(t *testing.T) {
	now := time.Now().UTC()
	future := now.Add(time.Hour)
	past := now.Add(-time.Minute)
	at := func(t time.Time) *time.Time { return &t }
	windows := []domain.QuotaWindow{
		{Name: "five_hour", Used: 0.2, DurationSec: 18000, ResetsAt: at(future)},
		{Name: "seven_day", Used: 0.9, DurationSec: 604800, ResetsAt: at(past)},
	}

	// A window whose reset has passed is gone; the rest are as they were.
	q := quotaDTO(&domain.Quota{Source: "claude", Status: "ok", At: now.Add(-time.Minute), CheckedAt: now, Windows: windows})
	if q.Status != "ok" || len(q.Windows) != 1 || q.Windows[0].Name != "five_hour" {
		t.Fatalf("fresh: %+v %+v", q, q.Windows)
	}

	// A session report nobody refreshed for over an hour is unknown.
	q = quotaDTO(&domain.Quota{Source: "claude", SourceKind: "statusline", Status: "ok", At: now.Add(-2 * time.Hour), CheckedAt: now.Add(-2 * time.Hour), Windows: windows})
	if q.Status != "unknown" {
		t.Fatalf("stale session report: %q", q.Status)
	}

	// A runner's shared snapshot may keep a weekly window's older time on
	// purpose: its collection time is what says whether it is fresh. The
	// weekly use stays where the source got it (MapAt), not as "now".
	oldWindow := now.Add(-20 * time.Hour)
	q = quotaDTO(&domain.Quota{Source: "claude", Status: "ok", ProfileRef: "acct_1", At: oldWindow, CheckedAt: now.Add(-time.Minute),
		Windows: []domain.QuotaWindow{{Name: "seven_day", Used: 0.4, DurationSec: 604800, ResetsAt: at(now.Add(3 * 24 * time.Hour))}}})
	if q.Status != "ok" || len(q.Windows) != 1 {
		t.Fatalf("fresh runner snapshot: %+v", q)
	}
	q = quotaDTO(&domain.Quota{Source: "claude", Status: "ok", ProfileRef: "acct_1", At: oldWindow, CheckedAt: now.Add(-2 * time.Hour),
		Windows: []domain.QuotaWindow{{Name: "seven_day", Used: 0.4, DurationSec: 604800}}})
	if q.Status != "unknown" {
		t.Fatalf("stale runner snapshot: %q", q.Status)
	}

	// Failures keep their own status; they are not relabelled.
	q = quotaDTO(&domain.Quota{Source: "claude", Status: "no_credentials", At: now.Add(-5 * time.Hour), CheckedAt: now.Add(-5 * time.Hour)})
	if q.Status != "no_credentials" {
		t.Fatalf("failure status: %q", q.Status)
	}

	// An ok whose windows have all reset has nothing to say: unknown.
	q = quotaDTO(&domain.Quota{Source: "claude", Status: "ok", At: now, CheckedAt: now,
		Windows: []domain.QuotaWindow{{Name: "five_hour", Used: 0.2, ResetsAt: at(past)}}})
	if q.Status != "unknown" || len(q.Windows) != 0 {
		t.Fatalf("all reset: %+v", q)
	}
}
