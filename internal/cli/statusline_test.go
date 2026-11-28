package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/config"
)

const statusLineJSON = `{"session_id":"s","model":{"display_name":"Opus"},
 "rate_limits":{"five_hour":{"used_percentage":23,"resets_at":1795687200},
                "seven_day":{"used_percentage":12,"resets_at":1796090400}}}`

func TestParseStatusLineQuota(t *testing.T) {
	windows, ok := parseStatusLineQuota([]byte(statusLineJSON))
	if !ok || len(windows) != 2 {
		t.Fatalf("windows: %v %v", windows, ok)
	}
	five, seven := windows[0], windows[1]
	if five.Name != "five_hour" || five.Used != 0.23 || five.ResetsAt == nil || five.ResetsAt.Unix() != 1795687200 {
		t.Fatalf("five hours: %+v", five)
	}
	if seven.Name != "seven_day" || seven.Used != 0.12 || seven.ResetsAt == nil {
		t.Fatalf("seven days: %+v", seven)
	}
	// A missing window is not an error.
	if w, ok := parseStatusLineQuota([]byte(`{"rate_limits":{"seven_day":{"used_percentage":40}}}`)); !ok || len(w) != 1 || w[0].Name != "seven_day" || w[0].ResetsAt != nil {
		t.Fatalf("seven only: %v %v", w, ok)
	}
	// No rate limits (older Claude Code, API-key account, no response yet).
	for _, in := range []string{`{"session_id":"s"}`, `{"rate_limits":{}}`, ``, `nope`, `[]`} {
		if w, ok := parseStatusLineQuota([]byte(in)); ok || w != nil {
			t.Fatalf("%q: %v %v", in, w, ok)
		}
	}
	// Bad numbers and nameless windows are dropped, not fatal.
	in := `{"rate_limits":{"five_hour":{"used_percentage":101},"seven_day":{"used_percentage":-1},
	  "seven_day_opus":{"used_percentage":40},"":{"used_percentage":2},"junk":{"nope":1}}}`
	w, ok := parseStatusLineQuota([]byte(in))
	if !ok || len(w) != 1 || w[0].Name != "seven_day_opus" || w[0].Used != 0.4 {
		t.Fatalf("bad numbers: %v %v", w, ok)
	}
}

func TestStatusLineChainsTheLineThatWasThere(t *testing.T) {
	dir := t.TempDir()
	old := `{"type":"command","command":"old-line"}`
	if err := writeStatusLineState(dir, statusLineState{Previous: json.RawMessage(old)}); err != nil {
		t.Fatal(err)
	}
	orig := statusLineRun
	defer func() { statusLineRun = orig }()
	var ran, got string
	statusLineRun = func(_ context.Context, command string, stdin []byte) ([]byte, error) {
		ran, got = command, string(stdin)
		return []byte("custom 42%\n"), nil
	}
	out, ok := statusLinePrevious(dir, []byte(statusLineJSON))
	if !ok || out != "custom 42%\n" || ran != "old-line" || got != statusLineJSON {
		t.Fatalf("chained: %q %v %q %q", out, ok, ran, got)
	}
	// A previous command that fails leaves myco's own line to show.
	statusLineRun = func(context.Context, string, []byte) ([]byte, error) { return nil, os.ErrNotExist }
	if _, ok := statusLinePrevious(dir, nil); ok {
		t.Fatal("a failed command was used")
	}
}

func TestStatusLineInstallKeepsTheRestAndRestores(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dir := t.TempDir()
	claudeDir := t.TempDir()
	getenv := func(k string) string {
		if k == "CLAUDE_CONFIG_DIR" {
			return claudeDir
		}
		return ""
	}
	path := filepath.Join(claudeDir, "settings.json")
	original := `{"model":"opus","statusLine":{"type":"command","command":"old-line"},"permissions":{"allow":["Bash(ls)"]}}`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	plan, err := statusLinePlan(dir, getenv, false)
	if err != nil || !plan.Changed || !plan.Chained || plan.Previous != "old-line" || !strings.Contains(plan.Command, "agent runtime statusline") {
		t.Fatalf("plan: %+v %v", plan, err)
	}
	// A dry run writes neither file.
	if res, err := statusLineApply(dir, getenv, false, true); err != nil || !res.Changed {
		t.Fatalf("dry: %+v %v", res, err)
	}
	if b, _ := os.ReadFile(path); string(b) != original {
		t.Fatalf("dry run wrote settings: %s", b)
	}
	if _, err := os.Stat(statusLineStatePath(dir)); !os.IsNotExist(err) {
		t.Fatal("dry run wrote state")
	}

	if _, err := statusLineApply(dir, getenv, false, false); err != nil {
		t.Fatal(err)
	}
	var doc map[string]json.RawMessage
	b, _ := os.ReadFile(path)
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	if string(doc["model"]) != `"opus"` || !strings.Contains(string(doc["permissions"]), "Bash(ls)") {
		t.Fatalf("other keys lost: %s", b)
	}
	if cmd := statusLinePreviousCommand(doc["statusLine"]); !isOurStatusLine(cmd) {
		t.Fatalf("not installed: %s", doc["statusLine"])
	}
	// Installing again changes nothing.
	if again, err := statusLineApply(dir, getenv, false, false); err != nil || again.Changed {
		t.Fatalf("idempotent: %+v %v", again, err)
	}
	// Uninstall puts the exact value back (as JSON) and forgets it.
	res, err := statusLineApply(dir, getenv, true, false)
	if err != nil || !res.Changed || res.Previous != "old-line" {
		t.Fatalf("uninstall: %+v %v", res, err)
	}
	b, _ = os.ReadFile(path)
	doc = nil
	_ = json.Unmarshal(b, &doc)
	var want, got any
	_ = json.Unmarshal([]byte(`{"type":"command","command":"old-line"}`), &want)
	_ = json.Unmarshal(doc["statusLine"], &got)
	wantJSON, _ := json.Marshal(want)
	gotJSON, _ := json.Marshal(got)
	if string(wantJSON) != string(gotJSON) {
		t.Fatalf("restored: %s", doc["statusLine"])
	}
	if _, err := os.Stat(statusLineStatePath(dir)); !os.IsNotExist(err) {
		t.Fatal("state kept")
	}
	// A settings file that is not JSON is reported, not overwritten.
	if err := os.WriteFile(path, []byte("{nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := statusLineApply(dir, getenv, false, false); err == nil {
		t.Fatal("invalid settings accepted")
	}
	if b, _ := os.ReadFile(path); string(b) != "{nope" {
		t.Fatalf("invalid settings overwritten: %s", b)
	}
}

// statusLineTestAgent registers an agent on a test hub and returns the
// ~/.myco folder (in a temporary HOME), its profile and the hub URL.
func statusLineTestAgent(t *testing.T) (dir, profile, hub string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dir = config.Dir(config.DefaultPath())
	hub = newHub(t)
	profile = filepath.Join(t.TempDir(), "laptop.yaml")
	h := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": profile}}
	if code, _, e := h.run("agent", "register", "--name", "laptop"); code != 0 {
		t.Fatalf("register: %s", e)
	}
	return dir, profile, hub
}

func TestStatusLineCommandWritesOnlyNumbers(t *testing.T) {
	dir, profile, _ := statusLineTestAgent(t)
	var out, errb bytes.Buffer
	code := Run(context.Background(), []string{"agent", "runtime", "statusline"}, strings.NewReader(statusLineJSON), &out, &errb,
		func(k string) string {
			if k == "MYCO_CONFIG" {
				return profile
			}
			return ""
		}, false)
	if code != 0 {
		t.Fatalf("statusline: %d %s", code, errb.String())
	}
	if got := strings.TrimSpace(out.String()); got != "myco · 5h 23% · 7d 12%" {
		t.Fatalf("line: %q", got)
	}
	file := statusLineQuotaPath(dir, "laptop")
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	for _, word := range []string{"session_id", "display_name", "Opus", "account", "token"} {
		if strings.Contains(string(b), word) {
			t.Fatalf("%q in %s", word, b)
		}
	}
	windows, at, ok := readStatusLineQuota(dir, "laptop")
	if !ok || len(windows) != 2 || at.IsZero() || windows[0].Used != 0.23 {
		t.Fatalf("kept: %v %v %v", windows, at, ok)
	}
	// No rate limits: one line, nothing kept, still exit 0.
	_ = os.WriteFile(file, []byte("old"), 0o600)
	out.Reset()
	code = Run(context.Background(), []string{"agent", "runtime", "statusline"}, strings.NewReader(`{"session_id":"s"}`), &out, &errb,
		func(k string) string {
			if k == "MYCO_CONFIG" {
				return profile
			}
			return ""
		}, false)
	if code != 0 || strings.TrimSpace(out.String()) != "myco" {
		t.Fatalf("no limits: %d %q", code, out.String())
	}
	if b, _ := os.ReadFile(file); string(b) != "old" {
		t.Fatalf("no limits wrote: %s", b)
	}
}

func TestReportStatusLineQuotaThrottles(t *testing.T) {
	dir, profile, hub := statusLineTestAgent(t)
	cfg, err := config.Load(profile)
	if err != nil {
		t.Fatal(err)
	}
	c, err := client.New(hub, cfg.Token)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	// Account-scoped quotas (a runner's shared snapshot) need the machine.
	if err := c.ReportMachine(ctx, api.MachineReport{Fingerprint: strings.Repeat("a", 64), Hostname: "test", OS: "linux", Arch: "amd64", MycoVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	quota := func() *api.Quota {
		me, err := c.Me(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if me.Runtime == nil {
			return nil
		}
		return me.Runtime.Quota
	}
	now := time.Now().UTC()
	w := func(used float64) []statusLineQuotaWindow {
		return []statusLineQuotaWindow{{Name: "five_hour", Used: used, Status: "allowed"}}
	}

	first := now.Add(-10 * time.Minute)
	if err := writeStatusLineQuota(dir, "laptop", w(0.2), first); err != nil {
		t.Fatal(err)
	}
	reportStatusLineQuota(ctx, c, dir, "laptop", now)
	q := quota()
	if q == nil || q.SourceKind != "statusline" || !q.At.Equal(first) || len(q.Windows) != 1 || q.Windows[0].Used != 0.2 {
		t.Fatalf("reported: %+v", q)
	}

	// A newer observation less than five minutes after the last report
	// waits; the hub keeps the first.
	second := now.Add(-8 * time.Minute)
	_ = writeStatusLineQuota(dir, "laptop", w(0.3), second)
	reportStatusLineQuota(ctx, c, dir, "laptop", now.Add(2*time.Minute))
	if q := quota(); !q.At.Equal(first) {
		t.Fatalf("throttled report: %+v", q)
	}
	// Five minutes on, it goes.
	reportStatusLineQuota(ctx, c, dir, "laptop", now.Add(6*time.Minute))
	q = quota()
	if !q.At.Equal(second) || q.Windows[0].Used != 0.3 {
		t.Fatalf("throttled forever: %+v", q)
	}

	// An observation the hub already has news of (a runner reported a newer
	// one) is marked seen without a second report.
	newer := &api.Quota{Source: "claude", Provider: "claude", SourceKind: "event", Status: "ok", ProfileRef: "qp_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", At: now.Add(-6 * time.Minute), CheckedAt: now}
	me, _ := c.Me(ctx)
	rt := api.Runtime{Quota: newer}
	if me.Runtime != nil {
		rt = *me.Runtime
		rt.Quota = newer
	}
	if err := c.ReportRuntime(ctx, rt); err != nil {
		t.Fatal(err)
	}
	third := now.Add(-7 * time.Minute)
	_ = writeStatusLineQuota(dir, "laptop", w(0.4), third)
	reportStatusLineQuota(ctx, c, dir, "laptop", now.Add(20*time.Minute))
	if q := quota(); !q.At.Equal(newer.At) || q.ProfileRef != "qp_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("newer replaced: %+v", q)
	}
	if rep, ok := readStatusLineReport(dir, "laptop"); !ok || !rep.ObservedAt.Equal(third) {
		t.Fatalf("marker: %+v %v", rep, ok)
	}

	// A healthy native collection is left alone even when this observation
	// is newer than the time the runner kept (its weekly window can be old).
	resets := now.Add(time.Hour)
	native := &api.Quota{Source: "claude", Provider: "claude", SourceKind: "event", Status: "ok", ProfileRef: "qp_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", At: now.Add(-6*time.Minute + 30*time.Second), CheckedAt: now, Windows: []api.QuotaWindow{{Name: "five_hour", Used: 0.1, ResetsAt: &resets}}}
	me, _ = c.Me(ctx)
	rt = api.Runtime{Quota: native}
	if me.Runtime != nil {
		rt = *me.Runtime
		rt.Quota = native
	}
	if err := c.ReportRuntime(ctx, rt); err != nil {
		t.Fatal(err)
	}
	fourth := now.Add(-5 * time.Minute)
	_ = writeStatusLineQuota(dir, "laptop", w(0.5), fourth)
	reportStatusLineQuota(ctx, c, dir, "laptop", now.Add(40*time.Minute))
	if q := quota(); q.ProfileRef != "qp_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("native replaced: %+v", q)
	}
}

func TestStatusLineQuotaForRunner(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC)
	if _, _, ok := statusLineQuotaFor(dir, "laptop", now, nil); ok {
		t.Fatal("no observation")
	}
	at := now.Add(-time.Minute)
	if err := writeStatusLineQuota(dir, "laptop", []statusLineQuotaWindow{{Name: "five_hour", Used: 0.2}}, at); err != nil {
		t.Fatal(err)
	}
	q, got, ok := statusLineQuotaFor(dir, "laptop", now, nil)
	if !ok || !got.Equal(at) || q.SourceKind != "statusline" || q.ProfileRef != "" {
		t.Fatalf("for runner: %+v %v %v", q, got, ok)
	}
	// The runner records the report, which settles both throttles.
	writeStatusLineReport(dir, "laptop", statusLineReport{ObservedAt: at, ReportedAt: now})
	if _, _, ok := statusLineQuotaFor(dir, "laptop", now.Add(time.Minute), nil); ok {
		t.Fatal("same observation twice")
	}
	if err := writeStatusLineQuota(dir, "laptop", []statusLineQuotaWindow{{Name: "five_hour", Used: 0.3}}, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := statusLineQuotaFor(dir, "laptop", now.Add(3*time.Minute), nil); ok {
		t.Fatal("reported inside five minutes")
	}
	q, _, ok = statusLineQuotaFor(dir, "laptop", now.Add(6*time.Minute), nil)
	if !ok || q.Windows[0].Used != 0.3 {
		t.Fatalf("after five minutes: %+v %v", q, ok)
	}
	// Never over a newer report, nor over a healthy native collection.
	if _, _, ok := statusLineQuotaFor(dir, "laptop", now.Add(30*time.Minute), &api.Quota{Status: "transient_error", At: now.Add(10 * time.Minute)}); ok {
		t.Fatal("newer hub report replaced")
	}
	if _, _, ok := statusLineQuotaFor(dir, "laptop", now.Add(30*time.Minute), &api.Quota{ProfileRef: "qp_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Status: "ok", At: now.Add(-time.Hour)}); ok {
		t.Fatal("healthy native replaced")
	}
}
