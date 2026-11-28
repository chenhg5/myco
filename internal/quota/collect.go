package quota

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/api"
)

// Collect never refreshes tokens itself and never submits an inference turn.
// Error bodies, child stderr, and credentials are deliberately not returned.
func Collect(ctx context.Context, s Source) api.Quota { q, _ := collect(ctx, s); return q }

func collect(ctx context.Context, s Source) (api.Quota, time.Time) {
	q := api.Quota{Source: s.Provider, Provider: s.Provider, ProfileRef: s.Ref, Plan: s.Plan, SourceKind: s.Kind, Status: s.Status, CheckedAt: time.Now().UTC(), Windows: []api.QuotaWindow{}}
	if s.Status != "ok" {
		return q, time.Time{}
	}
	var err error
	switch s.Kind {
	case "native":
		err = codexQuota(ctx, s, &q)
	case "cache":
		err = claudeCache(s, &q)
	case "official_api":
		err = apiQuota(ctx, s, &q)
	default:
		q.Status = "unsupported"
		return q, time.Time{}
	}
	q.CheckedAt = time.Now().UTC()
	if err != nil {
		if q.Status == "ok" {
			q.Status = "schema_changed"
		}
		if q.ErrorCode == "" {
			q.ErrorCode = q.Status
		}
		var limited rateLimited
		if errors.As(err, &limited) {
			return q, limited.retry
		}
		q.At = time.Time{}
		q.Windows = []api.QuotaWindow{}
	} else if len(q.Windows) == 0 {
		q.Status = "schema_changed"
		q.ErrorCode = "empty_windows"
	} else if q.At.IsZero() {
		q.At = q.CheckedAt
	}
	return q, time.Time{}
}

func addWindow(q *api.Quota, name string, used float64, duration int, reset *time.Time, scope string) error {
	if len(q.Windows) >= 16 {
		return fmt.Errorf("too many windows")
	}
	if math.IsNaN(used) || math.IsInf(used, 0) || used < 0 || used > 1 || duration < 0 || len(name) > 80 || len(scope) > 80 {
		return fmt.Errorf("invalid window")
	}
	label := ""
	if duration > 0 {
		if duration%86400 == 0 {
			label = strconv.Itoa(duration/86400) + "d"
		} else if duration%3600 == 0 {
			label = strconv.Itoa(duration/3600) + "h"
		}
	}
	q.Windows = append(q.Windows, api.QuotaWindow{Name: name, Label: label, Used: used, DurationSec: duration, ResetsAt: reset, ModelScope: scope})
	return nil
}
func seconds(v int64) *time.Time {
	if v <= 0 {
		return nil
	}
	t := time.Unix(v, 0).UTC()
	return &t
}

// codexQuota talks only to the published native account protocol. No threads,
// tools, login methods, credit consumption, or model turns are requested.
func codexQuota(ctx context.Context, s Source, q *api.Quota) error {
	binary := s.binary
	if binary == "" {
		binary = "codex"
	}
	cmd := exec.CommandContext(ctx, binary, "app-server", "--stdio")
	cmd.Dir = s.dir
	stop := nativeProcess(cmd)
	cmd.WaitDelay = time.Second
	cmd.Env = mergeEnv(s.env, map[string]string{"CODEX_HOME": s.dir, "RUST_LOG": "off"})
	cmd.Stderr = io.Discard
	in, e := cmd.StdinPipe()
	if e != nil {
		q.Status = "unsupported"
		return e
	}
	out, e := cmd.StdoutPipe()
	if e != nil {
		q.Status = "unsupported"
		return e
	}
	cancelPipes := context.AfterFunc(ctx, func() { _ = in.Close(); _ = out.Close() })
	defer cancelPipes()
	if e = cmd.Start(); e != nil {
		q.Status = "unsupported"
		return e
	}
	defer func() { in.Close(); stop(); _ = cmd.Wait() }()
	enc := json.NewEncoder(in)
	if e = enc.Encode(map[string]any{"id": 1, "method": "initialize", "params": map[string]any{"clientInfo": map[string]string{"name": "myco_quota", "version": "1"}}}); e != nil {
		return e
	}
	scan := bufio.NewScanner(out)
	scan.Buffer(make([]byte, 4096), 2<<20)
	for scan.Scan() {
		var msg struct {
			ID    int `json:"id"`
			Error *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
			Result json.RawMessage `json:"result"`
		}
		if json.Unmarshal(scan.Bytes(), &msg) != nil {
			continue
		}
		if msg.Error != nil {
			q.Status = "transient_error"
			message := strings.ToLower(msg.Error.Message)
			switch {
			case msg.Error.Code == -32601 || msg.Error.Code == -32602:
				q.Status = "unsupported"
			case strings.Contains(message, "429") || strings.Contains(message, "rate limit"):
				q.Status = "throttled"
			case strings.Contains(message, "not logged in") || strings.Contains(message, "unauthorized") || strings.Contains(message, "authentication"):
				q.Status = "auth_required"
			}
			return fmt.Errorf("native RPC failed")
		}
		if msg.ID == 1 {
			if e = enc.Encode(map[string]any{"method": "initialized", "params": map[string]any{}}); e != nil {
				return e
			}
			if e = enc.Encode(map[string]any{"id": 2, "method": "account/rateLimits/read", "params": map[string]any{}}); e != nil {
				return e
			}
		} else if msg.ID == 2 {
			return parseCodex(msg.Result, q)
		}
	}
	q.Status = "transient_error"
	return fmt.Errorf("native quota unavailable")
}

func parseCodex(raw []byte, q *api.Quota) error {
	type window struct {
		Used     *float64 `json:"usedPercent"`
		Duration int      `json:"windowDurationMins"`
		Reset    int64    `json:"resetsAt"`
	}
	type bucket struct {
		Primary   *window `json:"primary"`
		Secondary *window `json:"secondary"`
	}
	var body struct {
		RateLimits *bucket           `json:"rateLimits"`
		Buckets    map[string]bucket `json:"rateLimitsByLimitId"`
	}
	if e := json.Unmarshal(raw, &body); e != nil {
		return e
	}
	buckets := body.Buckets
	if len(buckets) == 0 && body.RateLimits != nil {
		buckets = map[string]bucket{"codex": *body.RateLimits}
	}
	ids := make([]string, 0, len(buckets))
	for id := range buckets {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		b := buckets[id]
		for _, entry := range []struct {
			name string
			w    *window
		}{{"primary", b.Primary}, {"secondary", b.Secondary}} {
			if entry.w == nil {
				continue
			}
			w := entry.w
			if w.Used == nil {
				return fmt.Errorf("missing percentage")
			}
			name := entry.name
			scope := ""
			if id != "codex" {
				name = id + "_" + name
				scope = id
			}
			if e := addWindow(q, name, *w.Used/100, w.Duration*60, seconds(w.Reset), scope); e != nil {
				return e
			}
		}
	}
	return nil
}

func claudeCache(s Source, q *api.Quota) error {
	var doc struct {
		Account struct {
			ID string `json:"accountUuid"`
		} `json:"oauthAccount"`
		Cache struct {
			At      int64                      `json:"fetchedAtMs"`
			Account string                     `json:"accountUuid"`
			Usage   map[string]json.RawMessage `json:"utilization"`
		} `json:"cachedUsageUtilization"`
	}
	if !readJSON(s.endpoint, &doc) || doc.Account.ID == "" || doc.Cache.Account != doc.Account.ID || (s.account != "" && doc.Account.ID != s.account) || doc.Cache.At <= 0 {
		q.Status = "auth_required"
		if doc.Account.ID != "" && doc.Cache.At <= 0 {
			q.Status = "transient_error"
			q.ErrorCode = "cache_unavailable"
		}
		return fmt.Errorf("no matching cache")
	}
	q.At = time.UnixMilli(doc.Cache.At).UTC()
	if q.At.After(q.CheckedAt.Add(time.Minute)) {
		return fmt.Errorf("invalid cache time")
	}
	names := make([]string, 0, len(doc.Cache.Usage))
	for name := range doc.Cache.Usage {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		raw := doc.Cache.Usage[name]
		if name == "extra_usage" || name == "limits" {
			continue
		}
		var w struct {
			Used  *float64   `json:"utilization"`
			Reset *time.Time `json:"resets_at"`
		}
		if json.Unmarshal(raw, &w) != nil || w.Used == nil {
			continue
		}
		duration := 0
		if name == "five_hour" {
			duration = 18000
		} else if strings.HasPrefix(name, "seven_day") {
			duration = 604800
		}
		if e := addWindow(q, name, *w.Used/100, duration, w.Reset, ""); e != nil {
			return e
		}
	}
	// Newer native caches report model-scoped limits separately.
	var limits []struct {
		Percent *float64   `json:"percent"`
		Reset   *time.Time `json:"resets_at"`
		Scopes  struct {
			Model string `json:"model"`
		} `json:"scopes"`
	}
	if raw := doc.Cache.Usage["limits"]; raw != nil {
		if e := json.Unmarshal(raw, &limits); e != nil {
			return e
		}
	}
	for i, w := range limits {
		if w.Percent != nil {
			if e := addWindow(q, "limit_"+strconv.Itoa(i), *w.Percent/100, 0, w.Reset, w.Scopes.Model); e != nil {
				return e
			}
		}
	}
	return nil
}

func apiQuota(ctx context.Context, s Source, q *api.Quota) error {
	return apiQuotaClient(ctx, s, q, &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }})
}

func apiQuotaClient(ctx context.Context, s Source, q *api.Quota, client *http.Client) error {
	if (s.Provider == "glm" || s.Provider == "kimi" || s.Provider == "openrouter") && !planQuotaEndpoint(s.Provider, s.endpoint) {
		q.Status = "unsupported"
		return fmt.Errorf("invalid quota origin")
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, s.endpoint, nil)
	if e != nil {
		return e
	}
	req.Header.Set("Authorization", "Bearer "+s.key)
	if s.Provider == "glm" {
		req.Header.Set("Authorization", s.key)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	resp, e := client.Do(req)
	if e != nil {
		q.Status = "transient_error"
		return fmt.Errorf("quota transport failed")
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case 200:
	case 401:
		q.Status = "auth_required"
		return fmt.Errorf("unauthorized")
	case 403:
		q.Status = "unsupported"
		if s.Provider == "glm" || s.Provider == "kimi" || s.Provider == "openrouter" {
			q.Status = "auth_required"
		}
		return fmt.Errorf("no plan")
	case 429:
		q.Status = "throttled"
		return rateLimited{retry: retryAt(resp.Header.Get("Retry-After"), q.CheckedAt)}
	default:
		q.Status = "transient_error"
		return fmt.Errorf("quota HTTP failure")
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if e != nil {
		q.Status = "transient_error"
		return fmt.Errorf("quota response read failed")
	}
	if len(b) > 1<<20 {
		return fmt.Errorf("quota response too large")
	}
	switch s.Provider {
	case "opencode_go":
		return parseGo(b, q)
	case "glm":
		return parseGLM(b, q)
	case "kimi":
		return parseKimi(b, q)
	case "openrouter":
		return parseOpenRouter(b, q)
	default:
		return parseMiniMax(b, q)
	}
}
func parseGo(b []byte, q *api.Quota) error {
	var body struct {
		Usage map[string]struct {
			Percent *float64   `json:"percent"`
			Reset   *time.Time `json:"resetsAt"`
			Status  string     `json:"status"`
		} `json:"usage"`
	}
	if e := json.Unmarshal(b, &body); e != nil {
		return e
	}
	for _, v := range []struct {
		name string
		d    int
	}{{"rolling", 18000}, {"weekly", 604800}, {"monthly", 0}} {
		if w, ok := body.Usage[v.name]; ok {
			if w.Percent == nil {
				return fmt.Errorf("missing percentage")
			}
			if e := addWindow(q, v.name, *w.Percent/100, v.d, w.Reset, ""); e != nil {
				return e
			}
			if w.Status == "rate-limited" {
				q.Windows[len(q.Windows)-1].Status = "rate_limited"
			}
		}
	}
	return nil
}
func parseMiniMax(b []byte, q *api.Quota) error {
	// Only known fields are accepted. A new or incompatible schema remains
	// unknown, never a synthetic full allowance.
	var body struct {
		Base *struct {
			Code int `json:"status_code"`
		} `json:"base_resp"`
		Models []struct {
			Name       string   `json:"model_name"`
			Remaining  *float64 `json:"current_interval_remaining_percent"`
			End        int64    `json:"end_time"`
			Weekly     *float64 `json:"current_weekly_remaining_percent"`
			WeekEnd    int64    `json:"weekly_end_time"`
			WeekStatus int      `json:"current_weekly_status"`
		} `json:"model_remains"`
	}
	if e := json.Unmarshal(b, &body); e != nil {
		return e
	}
	if body.Base != nil && body.Base.Code != 0 {
		q.Status = "transient_error"
		q.ErrorCode = "provider_rejected"
		return fmt.Errorf("plan rejected")
	}
	sort.SliceStable(body.Models, func(i, j int) bool { return body.Models[i].Name == "general" && body.Models[j].Name != "general" })
	for _, m := range body.Models {
		if m.Name == "general" || strings.HasPrefix(m.Name, "MiniMax-M") {
			if m.Remaining != nil {
				if e := addWindow(q, "five_hour", 1-*m.Remaining/100, 18000, seconds(m.End/1000), ""); e != nil {
					return e
				}
			}
			if (m.WeekStatus == 1 || m.WeekStatus == 2) && m.Weekly != nil {
				return addWindow(q, "seven_day", 1-*m.Weekly/100, 604800, seconds(m.WeekEnd/1000), "")
			}
			return nil
		}
	}
	return fmt.Errorf("unrecognized plan schema")
}
func mergeEnv(base, override map[string]string) []string {
	out := map[string]string{}
	for _, v := range os.Environ() {
		k, v, _ := strings.Cut(v, "=")
		out[k] = v
	}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range override {
		out[k] = v
	}
	vars := make([]string, 0, len(out))
	for k, v := range out {
		vars = append(vars, k+"="+v)
	}
	return vars
}

type rateLimited struct{ retry time.Time }

func (rateLimited) Error() string { return "rate limited" }
func retryAt(v string, now time.Time) time.Time {
	if sec, err := strconv.Atoi(v); err == nil && sec > 0 {
		return now.Add(time.Duration(min(sec, 86400)) * time.Second)
	}
	if at, err := http.ParseTime(v); err == nil && at.After(now) {
		return at
	}
	return now.Add(15 * time.Minute)
}
