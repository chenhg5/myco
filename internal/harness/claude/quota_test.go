package claude

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

// A rate_limit_event line as Claude Code 2.1 prints it in stream-json.
const rateLimitLine = `{"type":"rate_limit_event","rate_limit_info":{"isUsingOverage":false,"overageStatus":"rejected","rateLimitType":"five_hour","resetsAt":1794587400,"status":"allowed","unifiedWindows":{"five_hour":{"resetsAt":1794587400,"status":"allowed","utilization":0.17},"seven_day":{"resetsAt":1794880800,"status":"allowed","utilization":0.58}}},"session_id":"s1"}`

func TestRateLimitEventIsQuota(t *testing.T) {
	evs := lineEvents([]byte(rateLimitLine))
	if len(evs) != 1 || evs[0].Kind != "quota" || evs[0].Quota == nil {
		t.Fatalf("events: %+v", evs)
	}
	q := evs[0].Quota
	if q.Source != "claude" || len(q.Windows) != 2 {
		t.Fatalf("quota: %+v", q)
	}
	five, week := q.Windows[0], q.Windows[1]
	if five.Name != "five_hour" || five.Used != 0.17 || !five.ResetsAt.Equal(time.Unix(1794587400, 0)) || five.Status != "allowed" {
		t.Fatalf("five_hour: %+v", five)
	}
	if week.Name != "seven_day" || week.Used != 0.58 {
		t.Fatalf("seven_day: %+v", week)
	}
	if evs := lineEvents([]byte(`{"type":"rate_limit_event"}`)); len(evs) != 0 {
		t.Fatalf("an empty event is no quota: %+v", evs)
	}
}

func TestRunPassesModelAndEffort(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell")
	}
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	bin := filepath.Join(dir, "claude")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + argsFile + "\ncat >/dev/null\necho '{\"type\":\"result\",\"result\":\"ok\",\"session_id\":\"s1\"}'\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	h, err := New(harness.Options{Binary: bin, Model: "machine-model"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.Run(context.Background(), harness.Request{Prompt: "hi", Model: "app-model", Effort: "high"}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(argsFile)
	joined := strings.Join(strings.Fields(string(b)), " ")
	if !strings.Contains(joined, "--model app-model") || strings.Contains(joined, "machine-model") || !strings.Contains(joined, "--effort high") {
		t.Fatalf("args: %s", joined)
	}
	// Without run settings, the configured model and no effort.
	if _, err := h.Run(context.Background(), harness.Request{Prompt: "hi"}); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(argsFile)
	joined = strings.Join(strings.Fields(string(b)), " ")
	if !strings.Contains(joined, "--model machine-model") || strings.Contains(joined, "--effort") {
		t.Fatalf("args: %s", joined)
	}
}
