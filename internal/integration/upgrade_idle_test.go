package integration_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/cli"
	"github.com/myco-ai/myco/internal/config"
	"github.com/myco-ai/myco/internal/console"
	"github.com/myco-ai/myco/internal/release"
	"github.com/myco-ai/myco/internal/runnerupdate"
)

func TestUpgradeWhenIdleKeepsReceiptAndSchedulesOnlyRunner(t *testing.T) {
	home := isolatedHome(t)
	setVersion(t, "v0.16.6")
	bin := []byte("#!/bin/sh\necho fixture\n")
	sum := sha256.Sum256(bin)
	rel := fakeReleases(t, "v0.16.7", bin, hex.EncodeToString(sum[:]))
	target := filepath.Join(t.TempDir(), "myco")
	os.WriteFile(target, []byte("old"), 0755)
	receipt := filepath.Join(t.TempDir(), "receipt.json")
	base := filepath.Join(home, ".myco")
	e := &cliEnv{t: t, env: map[string]string{"HOME": home, "MYCO_FORMAT": "json", "PATH": os.Getenv("PATH"), "MYCO_MESSAGE_ID": "current-turn", release.EnvAPI: rel.URL + "/api/latest", release.EnvDownload: rel.URL + "/dl", cli.EnvUpgradeTarget: target}}
	if code, _, _ := e.run(ctx(t), "upgrade", "--restart-when-idle", "--no-restart"); code != cli.ExitUsage {
		t.Fatalf("conflicting flags: %d", code)
	}
	if code, _, _ := e.run(ctx(t), "upgrade", "--restart-when-idle", "--result-file", receipt, "--dry-run"); code != cli.ExitDryRun {
		t.Fatalf("dry run: %d", code)
	}
	if _, err := os.Stat(receipt); !os.IsNotExist(err) {
		t.Fatal("dry run wrote receipt")
	}
	if _, err := runnerupdate.LoadRequest(base); !os.IsNotExist(err) {
		t.Fatal("dry run scheduled restart")
	}
	out := e.must(ctx(t), "upgrade", "--restart-when-idle", "--result-file", receipt)
	if !strings.Contains(out, "runner_restart") {
		t.Fatalf("missing schedule: %s", out)
	}
	req, err := runnerupdate.LoadRequest(base)
	if err != nil || req.Target != "v0.16.7" || req.Deadline.Sub(req.RequestedAt) != runnerupdate.IdleLimit {
		t.Fatalf("request: %+v %v", req, err)
	}
	var got runnerupdate.Receipt
	b, _ := os.ReadFile(receipt)
	if json.Unmarshal(b, &got) != nil || got.State != "succeeded" || len(got.Result) == 0 {
		t.Fatalf("receipt: %s", b)
	}
	b, _ = os.ReadFile(target)
	if string(b) != string(bin) {
		t.Fatal("binary was not installed")
	}
	e.run(ctx(t), "upgrade", "--version", "invalid", "--result-file", receipt)
	b, _ = os.ReadFile(receipt)
	json.Unmarshal(b, &got)
	if got.State != "failed" || got.Error == "" {
		t.Fatalf("failure lost: %s", b)
	}
	badConfig := filepath.Join(t.TempDir(), "config.yaml")
	os.WriteFile(badConfig, []byte("runner: [invalid"), 0600)
	e.env["MYCO_CONFIG"] = badConfig
	if code, _, _ := e.run(ctx(t), "upgrade", "--restart-when-idle", "--result-file", receipt); code == 0 {
		t.Fatal("invalid config accepted")
	}
	b, _ = os.ReadFile(receipt)
	json.Unmarshal(b, &got)
	if got.State != "failed" || got.Error == "" {
		t.Fatalf("failure before command handler was lost: %s", b)
	}

}

func TestConsoleUpgradeReceiptSurvivesRelaunchAndRetry(t *testing.T) {
	home := t.TempDir()
	base := filepath.Join(home, ".myco")
	starts := 0
	l := &console.Local{Home: home, LookPath: func(string) (string, error) { return "/demo/myco", nil }, Run: func(_ context.Context, _ []string, _ string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "schema" {
			return []byte(`{"name":"restart-when-idle"}`), nil
		}
		return []byte(`{"version":"v2.0.0"}`), nil
	}}
	l.StartProcess = func(command []string, logPath string, env []string) (int, error) {
		starts++
		args := strings.Join(command, " ")
		if !strings.Contains(args, "--restart-when-idle") || strings.Contains(args, "--no-restart") {
			t.Errorf("wrong policy: %v", command)
		}
		for i, v := range command {
			if v == "--result-file" {
				runnerupdate.WriteFile(command[i+1], runnerupdate.Receipt{State: "failed", At: time.Now(), Error: "download unavailable"})
			}
		}
		return 100, nil
	}
	makeApp := func() *appDevice {
		b := &console.Backend{Store: console.NewStore(filepath.Join(home, "hubs.json")), Local: l, Version: "v2.0.0"}
		srv := httptest.NewServer(b.Handler())
		t.Cleanup(srv.Close)
		return &appDevice{t: t, srv: srv}
	}
	d := makeApp()
	code, m, _ := d.do("POST", "/app/local/upgrade", map[string]any{})
	if code != 202 || starts != 1 {
		t.Fatalf("start: %d %v", code, m)
	}
	// A new backend process reads the outcome of the detached invocation.
	d = makeApp()
	_, m, _ = d.do("GET", "/app/local/upgrade", nil)
	receipt, _ := m["cli"].(map[string]any)
	if receipt["state"] != "failed" || receipt["error"] != "download unavailable" {
		t.Fatalf("lost failure: %v", m)
	}
	d.do("POST", "/app/local/upgrade", map[string]any{})
	if starts != 2 {
		t.Fatal("failed upgrade could not be retried")
	}
	runnerupdate.Write(base, "runtime", runnerupdate.Runtime{PID: 10, Instance: "running", Version: "v1.0.0", At: time.Now()})
	code, m, _ = d.do("POST", "/app/local/upgrade/restart", map[string]any{})
	if code != 202 || m["pending"] != true || starts != 2 {
		t.Fatalf("restart must only enqueue: %d %v", code, m)
	}
	req, err := runnerupdate.LoadRequest(base)
	if err != nil {
		t.Fatal(err)
	}
	if req.Deadline.Sub(req.RequestedAt) > runnerupdate.IdleLimit {
		t.Fatal("unbounded wait")
	}
	runnerupdate.Write(base, "runtime", runnerupdate.Runtime{PID: 11, Instance: "new", Version: "v2.0.0", At: time.Now(), State: "complete", RequestID: req.ID})
	_, m, _ = d.do("GET", "/app/local/upgrade", nil)
	if m["pending"] != false {
		t.Fatalf("new runtime not recognized: %v", m)
	}
	// The installed executable alone must not count as a running new version.
	runnerupdate.Write(base, "runtime", runnerupdate.Runtime{PID: 11, Version: "v2.0.0", At: time.Now().Add(-time.Hour)})
	_, m, _ = d.do("GET", "/app/local/upgrade", nil)
	if m["runner"] != nil {
		t.Fatal(fmt.Sprint("stale runtime accepted: ", m))
	}
}

func TestConsoleShowsLegacyRunnerReportedVersionWithoutRestartAuthority(t *testing.T) {
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	worker := h.client(h.register(api.RegisterRequest{Name: "worker", Binding: api.Binding{Harness: "codex", Environment: "demo-mac"}}))
	if err := worker.ReportMachine(ctx(t), api.MachineReport{Fingerprint: strings.Repeat("cd", 16), Name: "demo-mac", OS: "darwin", MycoVersion: "v0.23.9"}); err != nil {
		t.Fatal(err)
	}
	if err := worker.ReportRuntime(ctx(t), api.Runtime{CLI: "codex"}); err != nil {
		t.Fatal(err)
	}
	machines, err := h.admin().Machines(ctx(t))
	if err != nil || len(machines) != 1 {
		t.Fatalf("machines: %+v %v", machines, err)
	}
	home := t.TempDir()
	cfg := filepath.Join(home, ".myco", "config.yaml")
	if err := config.SaveMachine(cfg, &config.Machine{MachineHub: config.MachineHub{Hub: h.srv.URL, Machine: machines[0].ID}}); err != nil {
		t.Fatal(err)
	}
	l := &console.Local{Home: home, LookPath: func(string) (string, error) { return "/demo/myco", nil }, Run: func(context.Context, []string, string, ...string) ([]byte, error) {
		return []byte(`{"version":"v0.24.0"}`), nil
	}}
	b := &console.Backend{Store: console.NewStore(filepath.Join(home, "hubs.json")), Secrets: &memSecrets{m: map[string]string{}}, Local: l, Version: "v0.24.0"}
	srv := httptest.NewServer(b.Handler())
	t.Cleanup(srv.Close)
	d := &appDevice{t: t, srv: srv}
	if code, _, raw := d.do("POST", "/app/hubs", map[string]string{"url": h.srv.URL, "token": adminToken, "name": "Team"}); code/100 != 2 {
		t.Fatalf("add hub: %d %s", code, raw)
	}
	_, out, _ := d.do("GET", "/app/local/upgrade", nil)
	rt, _ := out["runner"].(map[string]any)
	if rt["version"] != "v0.23.9" || out["pending"] != true || out["unsupported"] != true {
		t.Fatalf("legacy version lost: %v", out)
	}
	if code, out, _ := d.do("POST", "/app/local/upgrade/restart", map[string]any{}); code != 409 || out["error"] != "unsupported" {
		t.Fatalf("legacy report authorized restart: %d %v", code, out)
	}
	if _, err := runnerupdate.LoadRequest(filepath.Dir(cfg)); !os.IsNotExist(err) {
		t.Fatal("legacy report wrote a restart request")
	}
	now := time.Now().UTC()
	runnerupdate.Write(filepath.Dir(cfg), "request", runnerupdate.Request{ID: "legacy", Target: "v0.24.0", RequestedAt: now, Deadline: now.Add(time.Hour)})
	if err := worker.ReportMachine(ctx(t), api.MachineReport{Fingerprint: strings.Repeat("cd", 16), Name: "demo-mac", OS: "darwin", MycoVersion: "v0.24.0"}); err != nil {
		t.Fatal(err)
	}
	_, out, _ = d.do("GET", "/app/local/upgrade", nil)
	if out["pending"] != false {
		t.Fatalf("reported current version should settle a legacy pending notice: %v", out)
	}

}
