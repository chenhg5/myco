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
		case "codex", "claude", "opencode_go", "minimax", "unknown":
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
		if math.IsNaN(w.Used) || math.IsInf(w.Used, 0) || w.Used < 0 || w.Used > 1 || w.DurationSec < 0 || len(w.Name) > 80 || len(w.Label) > 80 || len(w.ModelScope) > 80 || len(w.Status) > 80 || strings.ContainsAny(w.Name+w.Label+w.ModelScope+w.Status, "\r\n\x00") {
			return Errf(CodeInvalidArgument, "invalid quota window")
		}
	}
	return nil
}
