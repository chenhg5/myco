package service

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

type launchdRestartRunner struct {
	calls       [][]string
	term        bool
	ignoreTerm  bool
	replacement bool
	goneOnTerm  bool
}

func (r *launchdRestartRunner) Run(name string, args ...string) (string, error) {
	r.calls = append(r.calls, append([]string{name}, args...))
	if name == "kill" && len(args) > 0 && args[0] == "-TERM" {
		r.term = true
		if r.goneOnTerm {
			return "", errors.New("process exited")
		}
	}
	if len(args) > 0 && args[0] == "print" {
		if r.term && r.replacement {
			return "state = running\npid = 43\n", nil
		}
		if r.term && !r.ignoreTerm {
			return "state = waiting\n", nil
		}
		return "state = running\npid = 42\n", nil
	}
	return "", nil
}
func TestLaunchdRestartCancelsThenStartsWithoutKillingReplacement(t *testing.T) {
	r := &launchdRestartRunner{}
	l := &Launchd{Home: t.TempDir(), UID: 501, R: r}
	if err := l.Restart(); err != nil {
		t.Fatal(err)
	}
	joined := ""
	for _, c := range r.calls {
		joined += strings.Join(c, " ") + "\n"
	}
	if !strings.Contains(joined, "kill -TERM 42") || strings.Contains(joined, "kill -KILL") || strings.Contains(joined, "kickstart -k") {
		t.Fatalf("graceful restart: %s", joined)
	}
	if got := strings.Join(r.calls[len(r.calls)-1], " "); got != "launchctl kickstart gui/501/ai.myco.runner" {
		t.Fatal(got)
	}
}
func TestLaunchdRestartBoundsLegacyUnresponsiveProcess(t *testing.T) {
	oldGrace, oldPoll := launchdRestartGrace, launchdRestartPoll
	launchdRestartGrace, launchdRestartPoll = 5*time.Millisecond, time.Millisecond
	defer func() { launchdRestartGrace, launchdRestartPoll = oldGrace, oldPoll }()
	r := &launchdRestartRunner{ignoreTerm: true}
	l := &Launchd{Home: t.TempDir(), UID: 501, R: r}
	if err := l.Restart(); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range r.calls {
		if strings.Join(c, " ") == "kill -KILL 42" {
			found = true
		}
	}
	if !found {
		t.Fatal("legacy runner was allowed to wait for 600 seconds")
	}
}

func TestLaunchdRestartLeavesNewPIDAlone(t *testing.T) {
	r := &launchdRestartRunner{replacement: true}
	l := &Launchd{Home: t.TempDir(), UID: 501, R: r}
	if err := l.Restart(); err != nil {
		t.Fatal(err)
	}
	for _, c := range r.calls {
		if c[0] == "kill" && c[1] != "-TERM" {
			t.Fatalf("signalled replacement: %v", r.calls)
		}
		if c[0] == "launchctl" && c[1] == "kickstart" {
			t.Fatalf("restarted replacement: %v", r.calls)
		}
	}
}
func TestLaunchdRestartAcceptsOldPIDExitingBeforeSignal(t *testing.T) {
	r := &launchdRestartRunner{goneOnTerm: true}
	l := &Launchd{Home: t.TempDir(), UID: 501, R: r}
	if err := l.Restart(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(r.calls[len(r.calls)-1], " "); got != "launchctl kickstart gui/501/ai.myco.runner" {
		t.Fatal(got)
	}
}
