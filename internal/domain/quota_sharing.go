package domain

import (
	"strings"
	"time"
)

// SubscriptionQuotaContext excludes key-scoped API allowances: a CLI's OAuth
// email does not identify its selected API provider or key's spending limit.
func SubscriptionQuotaContext(a *Agent) (provider, plan string) {
	q := a.Runtime.Quota
	if q != nil {
		if q.Status == "unsupported" {
			return "", ""
		}
		provider, plan = q.Provider, q.Plan
		if provider == "" {
			provider = q.Source
		}
	}
	cli := a.Runtime.CLI
	if cli == "" {
		cli = a.Binding.Harness
	}
	if provider == "" {
		switch cli {
		case "claude", "claude-cloud":
			provider = "claude"
		case "codex":
			provider = "codex"
		}
	}
	host := strings.ToLower(a.Runtime.ProviderHost)
	if host != "" && host != "api.anthropic.com" && host != "api.openai.com" && host != "chatgpt.com" {
		return "", ""
	}
	if q != nil && q.SourceKind == "official_api" {
		return "", ""
	}
	if plan == "" {
		cloud := cli == "claude-cloud" || a.Binding.Harness == "claude-cloud" || q != nil && q.SourceKind == "cloud_session"
		native := q != nil && (q.SourceKind == "native" || q.SourceKind == "cache")
		if !cloud && !native {
			return "", ""
		}
		switch provider {
		case "claude":
			plan = "claude_subscription"
		case "codex":
			plan = "chatgpt_subscription"
		}
	}
	if provider == "claude" && plan == "claude_subscription" || provider == "codex" && plan == "chatgpt_subscription" {
		return provider, plan
	}
	return "", ""
}

func NormalizeQuotaAccount(email string) string {
	email = strings.ToLower(strings.TrimSpace(email))
	if len(email) > 320 || strings.Count(email, "@") != 1 || strings.ContainsAny(email, " \t\r\n\x00<>\"") {
		return ""
	}
	local, host, _ := strings.Cut(email, "@")
	if local == "" || host == "" {
		return ""
	}
	return email
}

// FreshQuotaNumbers uses observation time, not a later failed collection time.
func FreshQuotaNumbers(q *Quota, now time.Time) bool {
	if q == nil || q.At.IsZero() || q.At.After(now.Add(5*time.Minute)) || now.Sub(q.At) > QuotaMaxAge {
		return false
	}
	for _, w := range q.Windows {
		if w.ResetsAt == nil || w.ResetsAt.After(now) {
			return true
		}
	}
	return false
}

// ResolveQuotaNumbers borrows numbers only. Target connection failures retain
// their status; another agent's identity/profile/machine never enters the DTO.
func ResolveQuotaNumbers(a *Agent, profile, shared *Quota, now time.Time) *Quota {
	own := a.Runtime.Quota
	var base Quota
	if own != nil {
		base = *own
	} else {
		p, plan := SubscriptionQuotaContext(a)
		if p == "" {
			return nil
		}
		base = Quota{Provider: p, Source: p, Plan: plan, Status: "ok"}
	}
	best := own
	if !FreshQuotaNumbers(best, now) {
		best = nil
	}
	for _, q := range []*Quota{profile, shared} {
		if q == nil || (base.Provider != "" && q.Provider != "" && base.Provider != q.Provider) || (base.Plan != "" && q.Plan != "" && base.Plan != q.Plan) {
			continue
		}
		if FreshQuotaNumbers(q, now) && (best == nil || q.At.After(best.At) || q.At.Equal(best.At) && q.ReportedBy != "" && best.ReportedBy == "") {
			best = q
		}
	}
	if best == nil {
		if own == nil {
			return nil
		}
		if len(base.Windows) == 0 && profile != nil && (base.Plan == "" || base.Plan == profile.Plan) {
			base.At, base.Windows = profile.At, profile.Windows
		}
		legacyStatusOnly := len(base.Windows) == 0 && !base.At.IsZero() && now.Sub(base.At) <= QuotaMaxAge && !base.At.After(now.Add(5*time.Minute))
		if (base.Status == "ok" || base.Status == "") && !FreshQuotaNumbers(&base, now) && !legacyStatusOnly {
			base.Status = "unknown"
		}
		return &base
	}
	base.At, base.Windows = best.At, best.Windows
	if base.Provider == "" {
		base.Provider = best.Provider
	}
	if base.Plan == "" {
		base.Plan = best.Plan
	}
	if base.Source == "" {
		base.Source = best.Source
	}
	if base.SourceKind == "" {
		base.SourceKind = best.SourceKind
	}
	base.Shared = best.ReportedBy != "" && best.ReportedBy != a.ID
	if base.Shared {
		base.SharedAt = best.ReportedAt
	}
	if base.Status == "" || base.Status == "ok" || base.Status == "unknown" || base.Status == "transient_error" && (base.ErrorCode == "pending" || base.ErrorCode == "collector_busy") {
		base.Status = "ok"
		base.CheckedAt = best.At
		base.ErrorCode = ""
	}
	return &base
}
