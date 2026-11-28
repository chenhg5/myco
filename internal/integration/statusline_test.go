package integration_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/cli"
)

// A Claude Code session with no runner keeps its account's limits through the
// status line; myco inbox wait reports them by itself, and the hub presents
// only the windows that still describe now.
func TestInboxWaitReportsStatusLineQuota(t *testing.T) {
	home := isolatedHome(t)
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	cloud := newCLI(t, h, filepath.Join(home, "cloud.yaml"))
	stale := newCLI(t, h, filepath.Join(home, "stale.yaml"))
	cloud.must(c, "agent", "register", "--name", "cloudy", "--harness", "claude-cloud")
	stale.must(c, "agent", "register", "--name", "sleepy", "--harness", "claude-cloud")

	statusline := func(env *cliEnv, in string) {
		t.Helper()
		var out, errb bytes.Buffer
		code := cli.Run(c, []string{"agent", "runtime", "statusline"}, strings.NewReader(in), &out, &errb, func(k string) string { return env.env[k] }, false)
		if code != 0 {
			t.Fatalf("statusline: %d %s%s", code, out.String(), errb.String())
		}
	}
	quota := func(env *cliEnv) map[string]any {
		t.Helper()
		out := env.must(c, "agent", "get", "--format", "json")
		var a struct {
			Runtime struct {
				Quota map[string]any `json:"quota"`
			} `json:"runtime"`
		}
		_ = json.Unmarshal([]byte(out), &a)
		return a.Runtime.Quota
	}
	future := time.Now().Add(3 * time.Hour).Unix()
	past := time.Now().Add(-time.Hour).Unix()
	// One window has already reset: it no longer describes now, so it is
	// kept on disk but not shown.
	statusline(cloud, `{"rate_limits":{"five_hour":{"used_percentage":10,"resets_at":`+strconv.FormatInt(future, 10)+`},
	  "seven_day":{"used_percentage":90,"resets_at":`+strconv.FormatInt(past, 10)+`}}}`)
	if code, _, _ := cloud.run(c, "inbox", "wait", "--timeout", "1ms"); code == 0 {
		t.Fatalf("inbox wait: exit %d", code)
	}
	q := quota(cloud)
	if q == nil || q["source_kind"] != "statusline" || q["source"] != "claude" || q["status"] != "ok" {
		t.Fatalf("reported: %v", q)
	}
	windows, _ := q["windows"].([]any)
	if len(windows) != 1 || windows[0].(map[string]any)["name"] != "five_hour" {
		t.Fatalf("expired window shown: %v", windows)
	}

	// An observation nobody refreshed for two hours counts as unknown, not
	// as an ok with no numbers.
	obs := map[string]any{"observed_at": time.Now().Add(-2 * time.Hour).UTC(), "windows": []map[string]any{
		{"name": "five_hour", "used": 0.2, "resets_at": time.Now().Add(3 * time.Hour).UTC(), "status": "allowed"},
	}}
	b, _ := json.Marshal(obs)
	file := filepath.Join(home, ".myco", "quota", "sleepy.json")
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, b, 0o600); err != nil {
		t.Fatal(err)
	}
	stale.run(c, "inbox", "wait", "--timeout", "1ms")
	if q := quota(stale); q == nil || q["status"] != "unknown" {
		t.Fatalf("stale observation: %v", q)
	}
}
