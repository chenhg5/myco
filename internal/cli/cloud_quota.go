package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/harness"
	"github.com/myco-ai/myco/internal/harness/claude"
)

// An agent in its own Claude Code session (a claude.ai cloud session that
// receives with myco inbox wait) has no runner to collect its account's
// quota. The session itself knows it: each turn's rate_limit_event (in the
// session's event stream) has both windows' use, and the session transcript
// keeps the rate_limit_info of an error when a limit was reached. These
// report it, as numbers and times only (no account, no tokens).

// quotaSourceKind marks quota a session reported about itself.
const quotaSourceKind = "cloud_session"

// newestRateLimitInfo finds the last rate_limit_info in some JSON: a
// rate_limit_event, a list of them (oldest first, as an event list gives
// them), {"rate_limit_info": {...}}, or the object itself. at is the time
// next to it (created_at or timestamp of the object holding it), if any.
func newestRateLimitInfo(data []byte) (info []byte, at time.Time, ok bool) {
	var doc any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if dec.Decode(&doc) != nil {
		return nil, time.Time{}, false
	}
	var found any
	var foundAt time.Time
	var walk func(v any, at time.Time)
	walk = func(v any, at time.Time) {
		switch x := v.(type) {
		case map[string]any:
			for _, key := range []string{"created_at", "timestamp"} {
				if s, ok := x[key].(string); ok {
					if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
						at = t
					}
				}
			}
			// The newest by its time; with no times, the last.
			if r, ok := x["rate_limit_info"]; ok && (found == nil || !at.Before(foundAt)) {
				found, foundAt = r, at
			}
			for k, child := range x {
				if k != "rate_limit_info" {
					walk(child, at)
				}
			}
		case []any:
			for _, child := range x {
				walk(child, at)
			}
		}
	}
	walk(doc, time.Time{})
	if found == nil {
		// The object itself.
		if m, ok := doc.(map[string]any); ok && (m["unifiedWindows"] != nil || m["rateLimitType"] != nil) {
			found = m
		}
	}
	if found == nil {
		return nil, time.Time{}, false
	}
	raw, err := json.Marshal(found)
	if err != nil {
		return nil, time.Time{}, false
	}
	return raw, foundAt, true
}

// sessionQuota is a session's own rate limits as the hub keeps quota.
func sessionQuota(q *harness.Quota, now time.Time) *api.Quota {
	if q == nil || len(q.Windows) == 0 {
		return nil
	}
	at := q.At
	if at.IsZero() || at.After(now) {
		at = now
	}
	out := &api.Quota{Source: "claude", Provider: "claude", SourceKind: quotaSourceKind, Status: "ok", At: at.UTC(), CheckedAt: now.UTC(), Windows: []api.QuotaWindow{}}
	for _, w := range q.Windows {
		if math.IsNaN(w.Used) || math.IsInf(w.Used, 0) || w.Used < 0 || w.Used > 1 {
			return nil
		}
		aw := api.QuotaWindow{Name: w.Name, Used: w.Used, Status: w.Status}
		if aw.Status == "allowed_warning" {
			aw.Status = "allowed"
		}
		switch {
		case w.Name == "five_hour":
			aw.DurationSec = 18000
		case strings.HasPrefix(w.Name, "seven_day"):
			aw.DurationSec = 604800
		}
		if !w.ResetsAt.IsZero() {
			t := w.ResetsAt.UTC()
			aw.ResetsAt = &t
		}
		out.Windows = append(out.Windows, aw)
	}
	return out
}

// mayReportSessionQuota: a runner's own collection is not overwritten (it
// refers to the computer's account), nor a newer observation.
func mayReportSessionQuota(old *api.Quota, q *api.Quota) bool {
	if old == nil {
		return true
	}
	if old.ProfileRef != "" {
		return false
	}
	return !old.At.After(q.At)
}

// lastTranscriptRateLimit is the rate_limit_info of the latest API error in
// a Claude Code session transcript, with when it was written.
func lastTranscriptRateLimit(path string) ([]byte, time.Time, bool) {
	lines := tailLines(path)
	for i := len(lines) - 1; i >= 0; i-- {
		if !bytes.Contains(lines[i], []byte(`"rate_limit_info"`)) {
			continue
		}
		var ev struct {
			Timestamp      string `json:"timestamp"`
			APIErrorParams struct {
				RateLimitInfo json.RawMessage `json:"rate_limit_info"`
			} `json:"apiErrorParams"`
		}
		if json.Unmarshal(lines[i], &ev) != nil || len(ev.APIErrorParams.RateLimitInfo) == 0 {
			continue
		}
		at, _ := time.Parse(time.RFC3339Nano, ev.Timestamp)
		return ev.APIErrorParams.RateLimitInfo, at, true
	}
	return nil, time.Time{}, false
}

// reportSessionQuota: when this Claude Code session's transcript says a
// limit was reached and it has not reset yet, the hub is told ("used up,
// resets at ..."), without a model call. Never for an agent its runner
// started, nor over the runner's own quota or a newer report. Best effort:
// quiet, a few seconds at most.
func reportSessionQuota(ctx context.Context, c *client.Client, getenv func(string) string, now time.Time) {
	if c == nil || getenv("MYCO_MESSAGE_ID") != "" {
		return
	}
	id := getenv("CLAUDE_CODE_SESSION_ID")
	if id == "" {
		return
	}
	dir := getenv("CLAUDE_CONFIG_DIR")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".claude")
	}
	path := newestMatch(filepath.Join(dir, "projects", "*", id+".jsonl"))
	if path == "" {
		return
	}
	info, at, ok := lastTranscriptRateLimit(path)
	if !ok {
		return
	}
	q := sessionQuota(claude.RateLimitQuota(info, at), now)
	if q == nil {
		return
	}
	// Only a limit still in force is worth saying: once it has reset, the
	// error says nothing about now.
	limited := false
	for _, w := range q.Windows {
		if w.Status == "rejected" && w.ResetsAt != nil && w.ResetsAt.After(now) {
			limited = true
		}
	}
	if !limited {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	me, err := c.Me(ctx)
	if err != nil {
		return
	}
	rt := api.Runtime{}
	if me.Runtime != nil {
		rt = *me.Runtime
	}
	if !mayReportSessionQuota(rt.Quota, q) || sameQuota(rt.Quota, q) {
		return
	}
	rt.Quota = q
	if rt.CLI == "" {
		rt.CLI = me.Binding.Harness
	}
	rt.Account = runtimeAccount(me.Binding.Harness, accountHome(getenv), getenv)
	_ = c.ReportRuntime(ctx, rt)
}

// sameQuota: the same windows, use and reset times (already reported).
func sameQuota(a, b *api.Quota) bool {
	if a == nil || b == nil || len(a.Windows) != len(b.Windows) || a.SourceKind != b.SourceKind {
		return false
	}
	for i := range a.Windows {
		x, y := a.Windows[i], b.Windows[i]
		if x.Name != y.Name || x.Used != y.Used || x.Status != y.Status || (x.ResetsAt == nil) != (y.ResetsAt == nil) ||
			(x.ResetsAt != nil && !x.ResetsAt.Equal(*y.ResetsAt)) {
			return false
		}
	}
	return true
}
