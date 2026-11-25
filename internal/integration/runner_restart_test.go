package integration_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
)

func TestRunnerRestartPresenceAcrossHTTP(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	admin := h.admin()
	names := []string{"demo-worker", "demo-shell", "demo-machine"}
	for i, name := range names {
		cli := []string{"codex", "exec", "machine"}[i]
		node := h.client(h.register(api.RegisterRequest{Name: name, Binding: api.Binding{Harness: cli}}))
		if err := node.ReportMachine(c, api.MachineReport{Fingerprint: "demo-machine-0123456789abcdef", Name: "demo-mac", Hostname: "demo-mac", OS: "darwin"}); err != nil {
			t.Fatal(err)
		}
		if err := node.ReportRuntime(c, api.Runtime{CLI: cli, RunnerInstance: "old"}); err != nil {
			t.Fatal(err)
		}
		until := time.Now().UTC().Add(time.Minute)
		if err := node.ReportRuntime(c, api.Runtime{CLI: cli, RunnerInstance: "old", Away: "updating", RestartingUntil: until}); err != nil {
			t.Fatal(err)
		}
		a, err := admin.GetAgent(c, name)
		if err != nil || !a.Updating || a.Runtime == nil || !a.Runtime.RestartingUntil.Equal(until) {
			t.Fatalf("restart DTO: %+v %v", a, err)
		}
	}
	ms, err := admin.Machines(c)
	if err != nil || len(ms) != 1 || !ms[0].Updating {
		t.Fatalf("machine restart DTO: %+v %v", ms, err)
	}
	for _, name := range names {
		// Use the same credentials via a saved registration is unnecessary:
		// rotate just this fixture's credential, retaining runtime on the Hub.
		node := h.client(h.register(api.RegisterRequest{Name: name, RotateToken: true}))
		if err := node.ReportRuntime(c, api.Runtime{RunnerInstance: "new"}); err != nil {
			t.Fatal(err)
		}
		a, err := admin.GetAgent(c, name)
		if err != nil || a.Updating || a.Runtime.Away != "" {
			t.Fatalf("replacement DTO: %+v %v", a, err)
		}
	}
	ms, err = admin.Machines(c)
	if err != nil || ms[0].Updating {
		t.Fatalf("machine did not clear: %+v %v", ms, err)
	}
}
