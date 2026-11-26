package claude

import (
	"encoding/json"
	"time"

	"github.com/myco-ai/myco/internal/harness"
)

// RateLimitQuota reads a rate_limit_info object as quota: the payload of
// Claude Code's rate_limit_event (each window's utilization), or the one an
// API error carries in a session transcript when a limit was reached (only
// the refused window and when it resets: that window is full). at is when it
// was seen (zero: now). Nil when it says nothing usable.
func RateLimitQuota(info []byte, at time.Time) *harness.Quota {
	var r rateLimitInfo
	if json.Unmarshal(info, &r) != nil {
		return nil
	}
	q := r.quota()
	if q == nil && r.Status == "rejected" && r.RateLimitType != "" && r.ResetsAt > 0 {
		q = &harness.Quota{Source: "claude", Windows: []harness.QuotaWindow{
			{Name: r.RateLimitType, Used: 1, ResetsAt: unix(r.ResetsAt), Status: r.Status},
		}}
	}
	if q != nil && !at.IsZero() {
		q.At = at.UTC()
	}
	return q
}
