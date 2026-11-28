package domain

import (
	"context"
	"math"
	"regexp"
	"strings"
)

// QuotaRepository stores one account snapshot shared by agents on a machine.
// The authenticated agent supplies owner and machine; neither comes from JSON.
type QuotaRepository interface {
	GetQuota(context.Context, string, string, string, string) (*Quota, error)
}

var regexpAccountHash = regexp.MustCompile(`^qa_[a-f0-9]{64}$`)

type AccountQuotaRepository interface {
	GetAccountQuota(context.Context, string, *Agent) (*Quota, error)
}

var quotaRef = regexp.MustCompile(`^qp_[a-f0-9]{64}$`)

func (q *Quota) Validate() error {
	if q == nil {
		return nil
	}
	if len(q.Windows) > 16 {
		return Errf(CodeInvalidArgument, "too many quota windows")
	}
	for _, s := range []string{q.Source, q.Provider, q.SourceKind, q.Status, q.ErrorCode, q.Plan} {
		if len(s) > 80 || strings.ContainsAny(s, "\x00\n\r") {
			return Errf(CodeInvalidArgument, "invalid quota metadata")
		}
	}
	if q.ProfileRef != "" {
		if !quotaRef.MatchString(q.ProfileRef) {
			return Errf(CodeInvalidArgument, "invalid quota profile reference")
		}
		switch q.Provider {
		case "codex", "claude", "opencode_go", "minimax", "glm", "kimi", "openrouter", "unknown":
		default:
			return Errf(CodeInvalidArgument, "invalid quota provider")
		}
		switch q.Status {
		case "ok", "unsupported", "no_credentials", "auth_required", "throttled", "transient_error", "schema_changed":
		default:
			return Errf(CodeInvalidArgument, "invalid quota status")
		}
		if q.CheckedAt.IsZero() {
			return Errf(CodeInvalidArgument, "quota check time is required")
		}
	}
	for _, w := range q.Windows {
		if b := w.Balance; b != nil {
			valid := b.Currency == "USD" && w.DurationSec == 0 && w.ResetsAt == nil
			if b.Unlimited {
				valid = valid && b.Limit == nil && b.Remaining == nil && w.Used == 0
			} else {
				valid = valid && b.Limit != nil && b.Remaining != nil
				if valid {
					valid = !math.IsNaN(*b.Limit) && !math.IsInf(*b.Limit, 0) && *b.Limit >= 0 && !math.IsNaN(*b.Remaining) && !math.IsInf(*b.Remaining, 0) && *b.Remaining >= 0 && *b.Remaining <= *b.Limit
					expected := 1.0
					if *b.Limit > 0 {
						expected = 1 - *b.Remaining / *b.Limit
					}
					valid = valid && math.Abs(w.Used-expected) <= 1e-9
				}
			}
			if !valid {
				return Errf(CodeInvalidArgument, "invalid quota balance")
			}
		}
		if math.IsNaN(w.Used) || math.IsInf(w.Used, 0) || w.Used < 0 || w.Used > 1 || w.DurationSec < 0 || len(w.Name) > 80 || len(w.Label) > 80 || len(w.ModelScope) > 80 || len(w.Status) > 80 || strings.ContainsAny(w.Name+w.Label+w.ModelScope+w.Status, "\r\n\x00") {
			return Errf(CodeInvalidArgument, "invalid quota window")
		}
	}
	return nil
}
