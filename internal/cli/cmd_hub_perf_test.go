package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
)

func TestPrintPerfPoolCountersAreNotWindowDurations(t *testing.T) {
	r := &api.PerfReport{Minutes: 5}
	var out bytes.Buffer
	printPerf(&out, r)
	if strings.Contains(out.String(), "Database pool") {
		t.Fatal("older hub should not display missing pool stats")
	}
	r.Database = &api.DatabasePoolStats{At: time.Date(2026, 11, 14, 12, 0, 10, 0, time.UTC), StartedAt: time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC), MaxOpenConnections: 1, OpenConnections: 1, InUse: 1, WaitCount: 7, WaitDurationMS: 123.5}
	out.Reset()
	printPerf(&out, r)
	for _, want := range []string{"current workspace; cumulative since 2026-11-14T12:00:00Z", "in use 1, idle 0", "connection waits: 7", "123.500 ms", "last 5 min"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q: %s", want, out.String())
		}
	}
}
