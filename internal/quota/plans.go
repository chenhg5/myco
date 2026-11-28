package quota

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/myco-ai/myco/internal/api"
)

var errPlanFormat = errors.New("unrecognized quota response")

// Plan APIs sometimes encode counts as decimal strings. Missing/null never
// means zero. Errors deliberately contain no provider response or credentials.
func planNumber(raw json.RawMessage) (float64, error) {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return 0, errPlanFormat
	}
	value := string(raw)
	if raw[0] == '"' {
		if json.Unmarshal(raw, &value) != nil {
			return 0, errPlanFormat
		}
	}
	n, e := strconv.ParseFloat(value, 64)
	if e != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, errPlanFormat
	}
	return n, nil
}
func planReset(raw json.RawMessage) (*time.Time, error) {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, nil
	}
	if raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return nil, errPlanFormat
		}
		if t, e := time.Parse(time.RFC3339Nano, s); e == nil {
			t = t.UTC()
			return &t, nil
		}
	}
	n, e := planNumber(raw)
	if e != nil || n < 946684800 || n > 32503680000000 || math.Trunc(n) != n {
		return nil, errPlanFormat
	}
	if n > 32503680000 {
		n /= 1000
	}
	t := time.UnixMilli(int64(n * 1000)).UTC()
	return &t, nil
}

// Contract: zai-org/ZCode docs/bigmodel-coding-plan-usage-quota.md.
// Unknown item types/periods invalidate the snapshot rather than silently
// hiding a potentially tighter window. Unknown additional fields are harmless.
func parseGLM(b []byte, q *api.Quota) error {
	var body struct {
		Code    json.RawMessage `json:"code"`
		Success *bool           `json:"success"`
		Data    struct {
			Limits []struct {
				Type       string          `json:"type"`
				Unit       json.RawMessage `json:"unit"`
				Number     json.RawMessage `json:"number"`
				Percentage json.RawMessage `json:"percentage"`
				Reset      json.RawMessage `json:"nextResetTime"`
			} `json:"limits"`
		} `json:"data"`
	}
	if json.Unmarshal(b, &body) != nil || len(body.Data.Limits) == 0 || len(body.Data.Limits) > 16 {
		return errPlanFormat
	}
	if body.Success != nil && !*body.Success {
		return errPlanFormat
	}
	if len(body.Code) > 0 {
		code, e := planNumber(body.Code)
		if e != nil || (code != 0 && code != 200) {
			return errPlanFormat
		}
	}
	seen := map[string]bool{}
	for _, w := range body.Data.Limits {
		unit, e := planNumber(w.Unit)
		if e != nil {
			return e
		}
		count, e := planNumber(w.Number)
		if e != nil {
			return e
		}
		name, scope, duration := "", "", 0
		switch {
		case w.Type == "TOKENS_LIMIT" && unit == 3 && count == 5:
			name, duration = "five_hour", 18000
		case w.Type == "TOKENS_LIMIT" && unit == 6 && count == 1:
			name, duration = "weekly", 604800
		case w.Type == "TIME_LIMIT" && unit == 5 && count == 1:
			name, scope = "monthly", "MCP"
		default:
			return errPlanFormat
		}
		if seen[name] {
			return errPlanFormat
		}
		seen[name] = true
		used, e := planNumber(w.Percentage)
		if e != nil {
			return e
		}
		reset, e := planReset(w.Reset)
		if e != nil {
			return e
		}
		if e = addWindow(q, name, used/100, duration, reset, scope); e != nil {
			return errPlanFormat
		}
	}
	return nil
}

type kimiUsage struct {
	Limit      json.RawMessage `json:"limit"`
	Used       json.RawMessage `json:"used"`
	Remaining  json.RawMessage `json:"remaining"`
	ResetTime  json.RawMessage `json:"resetTime"`
	ResetAt    json.RawMessage `json:"reset_at"`
	ResetCamel json.RawMessage `json:"resetAt"`
	ResetSnake json.RawMessage `json:"reset_time"`
}

func kimiWindow(q *api.Quota, w kimiUsage, name string, duration int) error {
	limit, e := planNumber(w.Limit)
	if e != nil || limit <= 0 {
		return errPlanFormat
	}
	var used float64
	if len(w.Used) > 0 {
		used, e = planNumber(w.Used)
	} else {
		var left float64
		left, e = planNumber(w.Remaining)
		used = limit - left
	}
	if e != nil || used < 0 || used > limit {
		return errPlanFormat
	}
	if len(w.Remaining) > 0 {
		left, err := planNumber(w.Remaining)
		if err != nil || left < 0 || left > limit || math.Abs(used+left-limit) > math.Max(1e-9, limit*1e-9) {
			return errPlanFormat
		}
	}
	raw := w.ResetTime
	for _, r := range []json.RawMessage{w.ResetAt, w.ResetCamel, w.ResetSnake} {
		if len(raw) == 0 {
			raw = r
		}
	}
	reset, e := planReset(raw)
	if e != nil {
		return e
	}
	return addWindow(q, name, used/limit, duration, reset, "")
}

// Matches the count-based /usages contract consumed by MoonshotAI/kimi-cli.
// Ratio-only OAuth response variants are not inferred or treated as zero.
func parseKimi(b []byte, q *api.Quota) error {
	var body struct {
		Usage  *kimiUsage `json:"usage"`
		Limits []struct {
			kimiUsage
			Detail *kimiUsage `json:"detail"`
			Window struct {
				Duration json.RawMessage `json:"duration"`
				Unit     string          `json:"timeUnit"`
			} `json:"window"`
		} `json:"limits"`
	}
	if json.Unmarshal(b, &body) != nil || body.Usage == nil || len(body.Limits) > 15 {
		return errPlanFormat
	}
	if e := kimiWindow(q, *body.Usage, "weekly", 604800); e != nil {
		return e
	}
	for i, w := range body.Limits {
		count, e := planNumber(w.Window.Duration)
		if e != nil || count <= 0 || math.Trunc(count) != count {
			return errPlanFormat
		}
		mul := 0
		switch w.Window.Unit {
		case "TIME_UNIT_MINUTE":
			mul = 60
		case "TIME_UNIT_HOUR":
			mul = 3600
		case "TIME_UNIT_DAY":
			mul = 86400
		case "TIME_UNIT_SECOND":
			mul = 1
		default:
			return errPlanFormat
		}
		if count > 366*86400/float64(mul) {
			return errPlanFormat
		}
		usage := w.kimiUsage
		if w.Detail != nil {
			usage = *w.Detail
		}
		if e = kimiWindow(q, usage, fmt.Sprintf("limit_%d", i), int(count)*mul); e != nil {
			return e
		}
	}
	return nil
}

// /key describes the key's spend allowance, not account credit balance. The
// most restrictive effective budget takes precedence. Never infer a reset.
func parseOpenRouter(b []byte, q *api.Quota) error {
	var body struct {
		Data *struct {
			Limit     json.RawMessage `json:"limit"`
			Remaining json.RawMessage `json:"limit_remaining"`
			Budget    json.RawMessage `json:"effective_budget"`
		} `json:"data"`
	}
	if json.Unmarshal(b, &body) != nil || body.Data == nil {
		return errPlanFormat
	}
	rawLimit, rawRemaining := body.Data.Limit, body.Data.Remaining
	if len(body.Data.Budget) > 0 && !bytes.Equal(body.Data.Budget, []byte("null")) {
		var budget struct {
			Limit     json.RawMessage `json:"limit_usd"`
			Remaining json.RawMessage `json:"remaining_usd"`
		}
		if json.Unmarshal(body.Data.Budget, &budget) != nil {
			return errPlanFormat
		}
		rawLimit, rawRemaining = budget.Limit, budget.Remaining
	}
	balance := &api.QuotaBalance{Currency: "USD"}
	used := 0.0
	if bytes.Equal(rawRemaining, []byte("null")) {
		// Explicit null is different from an absent field/schema change.
		if len(rawLimit) == 0 {
			return errPlanFormat
		}
		if !bytes.Equal(rawLimit, []byte("null")) {
			n, e := planNumber(rawLimit)
			if e != nil || n < 0 {
				return errPlanFormat
			}
		}
		balance.Unlimited = true
	} else {
		limit, e := planNumber(rawLimit)
		if e != nil || limit < 0 {
			return errPlanFormat
		}
		remaining, e := planNumber(rawRemaining)
		if e != nil || remaining > limit {
			return errPlanFormat
		}
		// The service can report overspend as a negative remaining allowance.
		remaining = math.Max(0, remaining)
		used = 1
		if limit > 0 {
			used = 1 - remaining/limit
		}
		balance.Limit, balance.Remaining = &limit, &remaining
	}
	if e := addWindow(q, "balance", used, 0, nil, ""); e != nil {
		return e
	}
	q.Windows[len(q.Windows)-1].Balance = balance
	return nil
}
