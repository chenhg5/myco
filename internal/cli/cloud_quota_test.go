package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/harness/claude"
)

// What a cloud session's event list gives: rate_limit_events, oldest first.
const rateLimitEvents = `{"ccr":{"data":[
 {"created_at":"2026-11-26T05:08:46.300421Z","rate_limit_event":{"uuid":"a","internal_anthropic_catchall":{"rate_limit_info":{
   "isUsingOverage":false,"overageStatus":"rejected","rateLimitType":"five_hour","resetsAt":1795687200,"status":"allowed",
   "unifiedWindows":{"five_hour":{"resetsAt":1795687200,"status":"allowed","utilization":0},"seven_day":{"resetsAt":1796090400,"status":"allowed","utilization":0.36}}},
   "session_id":"s"}}},
 {"created_at":"2026-11-26T08:37:29.921996Z","rate_limit_event":{"uuid":"b","internal_anthropic_catchall":{"rate_limit_info":{
   "isUsingOverage":false,"overageStatus":"rejected","rateLimitType":"five_hour","resetsAt":1795687200,"status":"allowed",
   "unifiedWindows":{"five_hour":{"resetsAt":1795687200,"status":"allowed","utilization":0.15},"seven_day":{"resetsAt":1796090400,"status":"allowed","utilization":0.4}}},
   "session_id":"s"}}}]}}`

func TestNewestRateLimitInfo(t *testing.T) {
	info, at, ok := newestRateLimitInfo([]byte(rateLimitEvents))
	if !ok || !at.Equal(time.Date(2026, 11, 26, 8, 37, 29, 921996000, time.UTC)) {
		t.Fatalf("newest: %v %v", ok, at)
	}
	var r struct {
		UnifiedWindows map[string]struct{ Utilization float64 } `json:"unifiedWindows"`
	}
	_ = json.Unmarshal(info, &r)
	if r.UnifiedWindows["five_hour"].Utilization != 0.15 {
		t.Fatalf("info: %s", info)
	}
	// Its rate_limit_info alone, or wrapped.
	for _, in := range []string{
		`{"rateLimitType":"five_hour","unifiedWindows":{"five_hour":{"utilization":0.2}}}`,
		`{"rate_limit_info":{"rateLimitType":"five_hour","unifiedWindows":{"five_hour":{"utilization":0.2}}}}`,
	} {
		if _, _, ok := newestRateLimitInfo([]byte(in)); !ok {
			t.Fatalf("%s", in)
		}
	}
	for _, in := range []string{`{}`, `[1,2]`, `nope`} {
		if _, _, ok := newestRateLimitInfo([]byte(in)); ok {
			t.Fatalf("%s found one", in)
		}
	}
}

func TestSessionQuotaIsNumbersAndTimesOnly(t *testing.T) {
	now := time.Date(2026, 11, 26, 9, 0, 0, 0, time.UTC)
	info, at, _ := newestRateLimitInfo([]byte(rateLimitEvents))
	q := sessionQuota(claude.RateLimitQuota(info, at), now)
	if q == nil || q.Source != "claude" || q.SourceKind != quotaSourceKind || q.ProfileRef != "" || len(q.Windows) != 2 {
		t.Fatalf("quota: %+v", q)
	}
	five, seven := q.Windows[0], q.Windows[1]
	if five.Name != "five_hour" || five.Used != 0.15 || five.DurationSec != 18000 || five.ResetsAt == nil ||
		seven.Name != "seven_day" || seven.Used != 0.4 || seven.DurationSec != 604800 {
		t.Fatalf("windows: %+v", q.Windows)
	}
	// Nothing of the account goes with it.
	raw, _ := json.Marshal(q)
	for _, word := range []string{"overage", "session_id", "isUsing", "uuid"} {
		if strings.Contains(string(raw), word) {
			t.Fatalf("%q in %s", word, raw)
		}
	}
	// A runner's quota is not replaced, nor a newer report.
	if mayReportSessionQuota(&api.Quota{ProfileRef: "acct_x"}, q) || mayReportSessionQuota(&api.Quota{At: now.Add(time.Hour)}, q) || !mayReportSessionQuota(nil, q) {
		t.Fatal("may report")
	}
}

func TestAgentRuntimeQuotaAndInboxWaitReportTheSessionsLimits(t *testing.T) {
	hub := newHub(t)
	dir := t.TempDir()
	h := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(dir, "agent.yaml")}}
	if code, _, e := h.run("agent", "register", "--name", "cloudy"); code != 0 {
		t.Fatalf("register: %s", e)
	}
	quota := func() map[string]any {
		code, out, e := h.run("agent", "get", "--format", "json")
		if code != 0 {
			t.Fatalf("get: %s", e)
		}
		var a struct {
			Runtime struct {
				Quota map[string]any `json:"quota"`
			} `json:"runtime"`
		}
		_ = json.Unmarshal([]byte(out), &a)
		return a.Runtime.Quota
	}
	// The fixture's dates are fixed. Shift them to now: the Hub drops a
	// window whose reset has passed, and presents an observation over an
	// hour old as unknown, so a fixed date would stop testing the report.
	now := time.Now().UTC()
	newest := now.Add(-30 * time.Minute).Format(time.RFC3339Nano)
	eventsJSON := strings.NewReplacer(
		"2026-11-26T05:08:46.300421Z", now.Add(-3*time.Hour).Format(time.RFC3339Nano),
		"2026-11-26T08:37:29.921996Z", newest,
		"1795687200", itoa(now.Add(time.Hour).Unix()),
		"1796090400", itoa(now.Add(6*24*time.Hour).Unix()),
	).Replace(rateLimitEvents)
	events := filepath.Join(dir, "events.json")
	if err := os.WriteFile(events, []byte(eventsJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	// A dry run says what it would report, and reports nothing.
	if code, out, e := h.run("agent", "runtime", "quota", "--file", events, "--dry-run", "--format", "json"); code != ExitDryRun || !strings.Contains(out, "five_hour") {
		t.Fatalf("dry run: %d %s %s", code, out, e)
	}
	if q := quota(); q != nil {
		t.Fatalf("a dry run reported: %v", q)
	}
	// Reported: both windows, as people see them.
	if code, _, e := h.run("agent", "runtime", "quota", "--file", events); code != 0 {
		t.Fatalf("quota: %s", e)
	}
	q := quota()
	windows, _ := q["windows"].([]any)
	if q["source"] != "claude" || q["source_kind"] != quotaSourceKind || len(windows) != 2 {
		t.Fatalf("reported: %v", q)
	}
	if w := windows[0].(map[string]any); w["name"] != "five_hour" || w["used"] != 0.15 {
		t.Fatalf("five hours: %v", w)
	}
	// Nothing usable: a usage error, nothing changed.
	bad := filepath.Join(dir, "bad.json")
	_ = os.WriteFile(bad, []byte(`{"hello":1}`), 0o600)
	if code, _, _ := h.run("agent", "runtime", "quota", "--file", bad); code == 0 {
		t.Fatalf("bad file: %d", code)
	}

	// The session's transcript says a limit was reached: myco inbox wait
	// reports it by itself (used up, until it resets).
	claudeDir := filepath.Join(dir, "claude")
	proj := filepath.Join(claudeDir, "projects", "-work")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	resets := time.Now().Add(3 * time.Hour).Unix()
	line := func(at time.Time, resets int64) string {
		return `{"type":"assistant","timestamp":"` + at.UTC().Format(time.RFC3339Nano) + `","isApiErrorMessage":true,"apiErrorParams":{"rate_limit_info":{"status":"rejected","resetsAt":` +
			itoa(resets) + `,"rateLimitType":"seven_day","overageStatus":"rejected","isUsingOverage":false}}}` + "\n"
	}
	transcript := filepath.Join(proj, "sess-9.jsonl")
	_ = os.WriteFile(transcript, []byte(`{"type":"user","message":{"content":"hi"}}`+"\n"+line(time.Now(), resets)), 0o600)
	h.env["CLAUDE_CODE_SESSION_ID"] = "sess-9"
	h.env["CLAUDE_CONFIG_DIR"] = claudeDir
	h.run("inbox", "wait", "--timeout", "1ms")
	q = quota()
	windows, _ = q["windows"].([]any)
	if len(windows) != 1 {
		t.Fatalf("used up: %v", q)
	}
	if w := windows[0].(map[string]any); w["name"] != "seven_day" || w["used"] != 1.0 || w["status"] != "rejected" {
		t.Fatalf("used up: %v", w)
	}
	// An older event does not replace it.
	if code, out, e := h.run("agent", "runtime", "quota", "--file", events, "--format", "json"); code != 0 || !strings.Contains(out, `"reported": false`) {
		t.Fatalf("older: %d %s %s", code, out, e)
	}
	// A newer one does. Then a limit that has reset says nothing about
	// now: nothing reported.
	fresh := filepath.Join(dir, "fresh.json")
	_ = os.WriteFile(fresh, []byte(strings.Replace(eventsJSON, newest, time.Now().Add(time.Second).UTC().Format(time.RFC3339Nano), 1)), 0o600)
	if code, _, e := h.run("agent", "runtime", "quota", "--file", fresh); code != 0 {
		t.Fatalf("quota: %s", e)
	}
	if windows, _ := quota()["windows"].([]any); len(windows) != 2 {
		t.Fatalf("a newer report was kept out: %v", windows)
	}
	_ = os.WriteFile(transcript, []byte(line(time.Now(), time.Now().Add(-time.Hour).Unix())), 0o600)
	h.run("inbox", "wait", "--timeout", "1ms")
	if windows, _ := quota()["windows"].([]any); len(windows) != 2 {
		t.Fatalf("a past limit replaced the report: %v", windows)
	}
	// An agent its runner started does not report from here.
	_ = os.WriteFile(transcript, []byte(line(time.Now().Add(time.Minute), resets)), 0o600)
	h.env["MYCO_MESSAGE_ID"] = "msg_x"
	h.run("inbox", "wait", "--timeout", "1ms")
	if windows, _ := quota()["windows"].([]any); len(windows) != 2 {
		t.Fatalf("a runner's agent reported: %v", windows)
	}
}

func itoa(n int64) string { b, _ := json.Marshal(n); return string(b) }

const warningRateLimitEvent = `{"created_at":"2026-11-28T12:11:24.557854Z","rate_limit_event":{"internal_anthropic_catchall":{"rate_limit_info":{"isUsingOverage":false,"overageStatus":"rejected","rateLimitType":"seven_day","resetsAt":1796090400,"status":"allowed_warning","surpassedThreshold":0.75,"unifiedWindows":{"five_hour":{"resetsAt":1795885200,"status":"allowed","utilization":0},"seven_day":{"resetsAt":1796090400,"status":"allowed_warning","surpassedThreshold":0.75,"utilization":0.81}},"utilization":0.81}}}}`

func TestRawCloudListEventsWarning(t *testing.T) {
	for _, raw := range []string{warningRateLimitEvent, "[" + warningRateLimitEvent + "]", `{"ccr":{"data":[` + warningRateLimitEvent + `]}}`} {
		info, at, ok := newestRateLimitInfo([]byte(raw))
		if !ok || at.Format(time.RFC3339Nano) != "2026-11-28T12:11:24.557854Z" {
			t.Fatalf("event timestamp: %v %v", at, ok)
		}
		q := sessionQuota(claude.RateLimitQuota(info, at), at.Add(time.Minute))
		if q == nil || len(q.Windows) != 2 || q.Windows[0].Used != 0 || q.Windows[1].Used != .81 || q.Windows[1].Status != "warning" {
			t.Fatalf("warning lost: %+v", q)
		}
		b, _ := json.Marshal(q)
		for _, field := range []string{"internal_anthropic", "session_id", "overage", "surpassedThreshold"} {
			if strings.Contains(string(b), field) {
				t.Fatalf("raw metadata leaked: %s", b)
			}
		}
	}
}

func TestSessionQuotaHintFreshness(t *testing.T) {
	now := time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name string
		q    *api.Quota
		want bool
	}{
		{"missing", nil, true}, {"zero", &api.Quota{}, true},
		{"stale", &api.Quota{At: now.Add(-30*time.Minute - time.Nanosecond)}, true},
		{"boundary", &api.Quota{At: now.Add(-30 * time.Minute)}, false},
		{"fresh", &api.Quota{At: now.Add(-time.Minute)}, false},
		{"newer", &api.Quota{At: now.Add(time.Minute)}, false},
		{"runner", &api.Quota{ProfileRef: "qp_demo"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sessionQuotaHint(tc.q, now); (got != "") != tc.want {
				t.Fatalf("hint %q", got)
			}
		})
	}
}

func TestCloudQuotaHintsThroughCommands(t *testing.T) {
	hub := newHub(t)
	dir := t.TempDir()
	cloud := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(dir, "cloud.yaml"), "CLAUDE_CODE_REMOTE_SESSION_ID": "demo-session"}}
	sender := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(dir, "sender.yaml")}}
	must := func(h *harnessT, args ...string) string {
		t.Helper()
		code, out, e := h.run(args...)
		if code != 0 {
			t.Fatalf("%v: %d %s %s", args, code, out, e)
		}
		return out
	}
	must(cloud, "agent", "register", "--name", "cloud", "--harness", "claude-cloud", "--session-id", "demo-session")
	must(sender, "agent", "register", "--name", "sender")
	hasHint := func(out string, want bool) {
		t.Helper()
		var v map[string]any
		if err := json.Unmarshal([]byte(out), &v); err != nil {
			t.Fatal(err)
		}
		_, got := v["quota_hint"]
		if got != want {
			t.Fatalf("want hint %v: %s", want, out)
		}
	}
	for _, verb := range []string{"reply", "done"} {
		out := must(sender, "message", "send", "--to", "cloud", "--body", "demo")
		var sent struct {
			Message api.Message `json:"message"`
		}
		if err := json.Unmarshal([]byte(out), &sent); err != nil {
			t.Fatal(err)
		}
		hasHint(must(cloud, "inbox", "wait", "--timeout", "1ms"), true)
		must(cloud, "inbox", "pull")
		must(cloud, "message", "ack", "--id", sent.Message.ID)
		args := []string{"message", verb, "--id", sent.Message.ID}
		if verb == "reply" {
			args = append(args, "--body", "done")
		}
		hasHint(must(cloud, args...), true)
	}
	code, out, stderr := cloud.run("inbox", "wait", "--timeout", "1ms")
	if code != ExitTimeout {
		t.Fatalf("timeout: %d %s", code, out)
	}
	hasHint(out+stderr, true)
	delete(cloud.env, "CLAUDE_CODE_REMOTE_SESSION_ID")
	_, out, stderr = cloud.run("inbox", "wait", "--timeout", "1ms")
	hasHint(out+stderr, false)
	cloud.env["CLAUDE_CODE_REMOTE_SESSION_ID"] = "demo-session"
	fresh := strings.NewReplacer("2026-11-28T12:11:24.557854Z", time.Now().UTC().Format(time.RFC3339Nano), "1795885200", itoa(time.Now().Add(time.Hour).Unix()), "1796090400", itoa(time.Now().Add(24*time.Hour).Unix())).Replace(warningRateLimitEvent)
	file := filepath.Join(dir, "events.json")
	if err := os.WriteFile(file, []byte(`{"ccr":{"data":[`+fresh+`]}}`), 0600); err != nil {
		t.Fatal(err)
	}
	must(cloud, "agent", "runtime", "quota", "--file", file)
	_, out, stderr = cloud.run("inbox", "wait", "--timeout", "1ms")
	hasHint(out+stderr, false)
}

func TestCloudQuotaHintRequiresRemoteSessionAndHubBinding(t *testing.T) {
	calls := 0
	harness := "claude-cloud"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		json.NewEncoder(w).Encode(api.Agent{Binding: api.Binding{Harness: harness}})
	}))
	defer srv.Close()
	c, err := client.New(srv.URL, "demo-token")
	if err != nil {
		t.Fatal(err)
	}
	get := func(string) string { return "" }
	if cloudQuotaHint(context.Background(), c, get, time.Now()) != "" || calls != 0 {
		t.Fatal("non-cloud environment made a request")
	}
	get = func(k string) string {
		if k == "CLAUDE_CODE_REMOTE_SESSION_ID" {
			return "demo"
		}
		return ""
	}
	for _, h := range []string{"claude", "codex", "inbox", ""} {
		harness = h
		if cloudQuotaHint(context.Background(), c, get, time.Now()) != "" {
			t.Fatalf("hint for %q", h)
		}
	}
	harness = "claude-cloud"
	if got := cloudQuotaHint(context.Background(), c, get, time.Now()); !strings.Contains(got, "limit 100") || !strings.Contains(got, "get_session with no id") {
		t.Fatalf("missing instructions: %q", got)
	}
	if hint := cloudQuotaHint(context.Background(), c, get, time.Now(), "/demo/a'b.yaml"); !strings.Contains(hint, "--config '/demo/a'\\''b.yaml'") {
		t.Fatalf("wrong report profile: %q", hint)
	}
	srv.Close()
	if cloudQuotaHint(context.Background(), c, get, time.Now()) != "" {
		t.Fatal("failed discovery returned a hint")
	}
}
