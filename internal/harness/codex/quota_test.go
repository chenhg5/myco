package codex

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/harness"
)

// A real token_count line from a Codex session file (Codex 0.x, Nov 2026).
const tokenCountLine = `{"timestamp":"2026-11-13T09:00:00.000Z","type":"event_msg","payload":{"type":"token_count","info":null,"rate_limits":{"limit_id":"codex","limit_name":null,"primary":{"used_percent":7.0,"window_minutes":10080,"resets_at":1794735584},"secondary":null,"credits":{"has_credits":true,"unlimited":false,"balance":"1.0"},"individual_limit":null,"spend_control_reached":null,"plan_type":"pro","rate_limit_reached_type":null}}}`

func TestSessionQuotaReadsTheLastRateLimits(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	dir := filepath.Join(home, "sessions", "2026", "11", "13")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	older := strings.Replace(tokenCountLine, `"used_percent":7.0`, `"used_percent":3.0`, 1)
	body := older + "\n" + `{"type":"event_msg","payload":{"type":"agent_message"}}` + "\n" + tokenCountLine + "\n"
	if err := os.WriteFile(filepath.Join(dir, "rollout-2026-11-13T09-00-00-sess-1.jsonl"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	q := sessionQuota("sess-1")
	if q == nil || q.Source != "codex" || len(q.Windows) != 1 {
		t.Fatalf("quota: %+v", q)
	}
	w := q.Windows[0]
	if w.Name != "primary" || w.Label != "7d" || w.Used != 0.07 || !w.ResetsAt.Equal(time.Unix(1794735584, 0)) {
		t.Fatalf("window: %+v", w)
	}
	if sessionQuota("missing") != nil {
		t.Fatal("no file, no quota")
	}
}

func TestOlderCodexResetsInSeconds(t *testing.T) {
	at := time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)
	reached := "primary"
	q := (&rateLimits{Primary: &rateWindow{UsedPercent: 100, WindowMinutes: 300, ResetsInSeconds: 600}, ReachedType: &reached}).quota(at)
	if q == nil || q.Windows[0].Label != "5h" || !q.Windows[0].ResetsAt.Equal(at.Add(10*time.Minute)) || q.Windows[0].Status != "primary" {
		t.Fatalf("quota: %+v", q)
	}
}

// The model and effort chosen in the app reach the CLI, and the effort's
// -c comes after the runner's own args so it wins over theirs.
func TestRunPassesModelAndEffort(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell")
	}
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	bin := filepath.Join(dir, "codex")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + argsFile + "\ncat >/dev/null\necho '{\"type\":\"thread.started\",\"thread_id\":\"t1\"}'\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", t.TempDir())
	h, err := New(harness.Options{Binary: bin, Model: "machine-model", Args: []string{"-c", "model_reasoning_effort=medium"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.Run(context.Background(), harness.Request{Prompt: "hi", Model: "app-model", Effort: "high"}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(argsFile)
	args := strings.Split(strings.TrimSpace(string(b)), "\n")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-m app-model") || strings.Contains(joined, "machine-model") {
		t.Fatalf("model: %v", args)
	}
	if strings.LastIndex(joined, `model_reasoning_effort="high"`) < strings.Index(joined, "model_reasoning_effort=medium") {
		t.Fatalf("the app's effort must come last: %v", args)
	}
}
