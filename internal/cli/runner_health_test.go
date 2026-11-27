package cli

import (
	"context"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/processwatch"
)

func TestHealthUpdateDoesNotMutatePublishedRuntime(t *testing.T) {
	base := api.Runtime{CLI: "codex", Account: "test@example.invalid", ModelsFrom: "cli"}
	reporter := &runtimeReporter{base: &base, stopped: true}
	manager := processwatch.New("fixture")
	manager.Observe(processwatch.Sample{At: time.Now(), Complete: true, Cores: 8, Total: 100, Available: 50, LoadMetric: "cpu_busy"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	reporter.watchHealth(ctx, manager, "workspace", "owner")
	if base.MachineHealth != nil {
		t.Fatal("health writer mutated the base held by startup discovery")
	}
	if reporter.base.MachineHealth == nil || reporter.base.Account != base.Account || reporter.base.ModelsFrom != base.ModelsFrom {
		t.Fatal("health publication lost runtime fields")
	}
}
