package domain

import "time"

func QuotaAvailability(q *Quota, now time.Time) string {
	if q == nil {
		return "unknown"
	}
	at := q.CheckedAt
	if at.IsZero() {
		at = q.At
	}
	if at.IsZero() || now.Sub(at) > QuotaMaxAge {
		return "unknown"
	}
	blocked := func(s string) bool {
		switch s {
		case "blocked", "rejected", "limit_reached", "rate_limit", "rate_limited":
			return true
		}
		return false
	}
	if blocked(q.Status) {
		return "blocked"
	}
	freshWindows := !q.At.IsZero() && now.Sub(q.At) <= QuotaMaxAge
	if !freshWindows {
		return "unknown"
	}
	for _, w := range q.Windows {
		if w.ResetsAt != nil && !w.ResetsAt.After(now) {
			continue
		}
		if blocked(w.Status) || (w.Used >= 1 && w.Status != "allowed" && w.Status != "allowed_warning" && w.Status != "warning") {
			return "blocked"
		}
	}
	if q.Status != "" && q.Status != "ok" && q.Status != "allowed" && q.Status != "allowed_warning" && q.Status != "warning" && q.Status != "available" {
		return "unknown"
	}
	return "available"
}
