package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunnerStopBoundsPreserveHubPolicy(t *testing.T) {
	for _, tc := range []struct {
		id      ID
		seconds string
	}{{RunnerID, "30"}, {HubID, "600"}} {
		s := &Systemd{ID: tc.id}
		if !strings.Contains(s.Unit(spec), "TimeoutStopSec="+tc.seconds+"\n") {
			t.Fatal(s.Unit(spec))
		}
		l := &Launchd{ID: tc.id}
		if !strings.Contains(l.Plist(spec), "<key>ExitTimeOut</key><integer>"+tc.seconds+"</integer>") {
			t.Fatal(l.Plist(spec))
		}
	}
}
func TestSystemdRestartMigratesInstalledRunnerBeforeStopping(t *testing.T) {
	f := &fakeRunner{}
	s := &Systemd{Home: t.TempDir(), R: f}
	os.MkdirAll(filepath.Dir(s.file()), 0755)
	old := "[Service]\nExecStart=/example/myco runner start --all --max-concurrent 7\nEnvironment=CODEX_HOME=/example/codex\nTimeoutStopSec=600\nKillMode=mixed\n"
	os.WriteFile(s.file(), []byte(old), 0644)
	if err := s.Restart(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(s.file())
	want := strings.Replace(old, "TimeoutStopSec=600", "TimeoutStopSec=30", 1)
	if string(b) != want {
		t.Fatalf("changed unrelated unit options: %s", b)
	}
	if len(f.calls) != 2 || strings.Join(f.calls[0], " ") != "systemctl --user daemon-reload" || strings.Join(f.calls[1], " ") != "systemctl --user restart myco-runner.service" {
		t.Fatalf("restart order: %v", f.calls)
	}
}
func TestWindowsRestartHasBoundedGracefulStopAndForceFallback(t *testing.T) {
	f := &fakeRunner{}
	w := &WindowsLogon{Home: t.TempDir(), R: f}
	if err := w.Restart(); err != nil {
		t.Fatal(err)
	}
	script := strings.Join(f.calls[0], " ")
	for _, want := range []string{"runner-stop-", "AddSeconds(25)", "Stop-Process", "-Force"} {
		if !strings.Contains(script, want) {
			t.Fatalf("missing %s: %s", want, script)
		}
	}
	if len(f.calls) != 2 || f.calls[1][len(f.calls[1])-1] != w.file() {
		t.Fatalf("did not start replacement: %v", f.calls)
	}
}

func TestLaunchdInstallDoesNotDrainLegacyRunnerForTenMinutes(t *testing.T) {
	l := &Launchd{Home: t.TempDir(), UID: 501}
	os.MkdirAll(filepath.Dir(l.file()), 0755)
	os.WriteFile(l.file(), []byte(`<key>ExitTimeOut</key><integer>600</integer>`), 0644)
	p, err := l.PlanInstall(spec)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(p.Commands[0], " ") != "-launchctl kill SIGKILL gui/501/ai.myco.runner" {
		t.Fatalf("legacy generation was not terminated first: %v", p.Commands)
	}
	os.WriteFile(l.file(), []byte(l.Plist(spec)), 0644)
	p, _ = l.PlanInstall(spec)
	if p.Commands[0][1] != "bootout" {
		t.Fatal("new runner should be allowed its bounded cleanup")
	}
}
