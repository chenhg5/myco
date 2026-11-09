package service

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeRunner struct {
	calls  [][]string
	output map[string]string
	fail   map[string]bool
}

func (f *fakeRunner) Run(name string, args ...string) (string, error) {
	call := append([]string{name}, args...)
	f.calls = append(f.calls, call)
	key := strings.Join(call, " ")
	if f.fail[key] {
		return "boom", errors.New("failed")
	}
	return f.output[key], nil
}

var spec = Spec{Binary: "/Users/me/.local/bin/myco", Args: []string{"runner", "start", "--all"},
	Env: map[string]string{"PATH": "/Users/me/.nvm/v24/bin:/usr/bin", "CODEX_HOME": "/Users/me/.codex"}, LogPath: "/Users/me/.myco/runner.log"}

func TestLaunchdInstallAndStatus(t *testing.T) {
	home := t.TempDir()
	r := &fakeRunner{fail: map[string]bool{"launchctl bootout gui/501/ai.myco.runner": true}, output: map[string]string{
		"launchctl print gui/501/ai.myco.runner": "ai.myco.runner = {\n\tstate = running\n\tpid = 4242\n}",
	}}
	l := &Launchd{Home: home, UID: 501, R: r}
	p, _ := l.PlanInstall(spec)
	for _, want := range []string{"<string>/Users/me/.local/bin/myco</string>", "<string>--all</string>", "<key>PATH</key><string>/Users/me/.nvm/v24/bin:/usr/bin</string>", "<key>KeepAlive</key><true/>"} {
		if !strings.Contains(p.Content, want) {
			t.Fatalf("plist missing %q:\n%s", want, p.Content)
		}
	}
	// bootout failing (not loaded yet) must not abort the install.
	if err := Apply(p, r); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, "Library/LaunchAgents/ai.myco.runner.plist")); err != nil {
		t.Fatal("plist not written")
	}
	if last := strings.Join(r.calls[len(r.calls)-1], " "); !strings.HasPrefix(last, "launchctl bootstrap gui/501 ") {
		t.Fatalf("last call: %s", last)
	}
	st, _ := l.Status()
	if !st.Installed || !st.Running || st.PID != 4242 {
		t.Fatalf("status: %+v", st)
	}
	if err := RemoveFile(l.PlanUninstall()); err != nil || exists(l.file()) {
		t.Fatal("uninstall should remove the plist")
	}
}

func TestSystemdUnit(t *testing.T) {
	home := t.TempDir()
	r := &fakeRunner{output: map[string]string{
		"systemctl --user show myco-runner.service --property=ActiveState,SubState,MainPID": "ActiveState=active\nSubState=running\nMainPID=99\n",
	}}
	s := &Systemd{Home: home, R: r}
	sp := spec
	sp.Env = map[string]string{"PATH": "/a b:/usr/bin"}
	p, _ := s.PlanInstall(sp)
	for _, want := range []string{"ExecStart=/Users/me/.local/bin/myco runner start --all", `Environment="PATH=/a b:/usr/bin"`, "Restart=always", "StandardOutput=append:/Users/me/.myco/runner.log"} {
		if !strings.Contains(p.Content, want) {
			t.Fatalf("unit missing %q:\n%s", want, p.Content)
		}
	}
	if err := Apply(p, r); err != nil {
		t.Fatal(err)
	}
	st, _ := s.Status()
	if !st.Running || st.PID != 99 || st.Detail != "active/running" {
		t.Fatalf("status: %+v", st)
	}
}

// flakyRunner fails a command the first n times.
type flakyRunner struct {
	fakeRunner
	failFirst map[string]int
}

func (f *flakyRunner) Run(name string, args ...string) (string, error) {
	key := strings.Join(append([]string{name}, args...), " ")
	f.calls = append(f.calls, append([]string{name}, args...))
	if f.failFirst[key] > 0 {
		f.failFirst[key]--
		return "Bootstrap failed: 5: Input/output error", errors.New("exit status 5")
	}
	return "", nil
}

// A bootstrap right after a bootout can fail while launchd is still tearing
// the old service down; Apply retries it instead of leaving it unloaded.
func TestLaunchdBootstrapRetries(t *testing.T) {
	bootstrapPause = 0
	t.Cleanup(func() { bootstrapPause = 500 * time.Millisecond })
	home := t.TempDir()
	l := &Launchd{Home: home, UID: 501}
	p, _ := l.PlanInstall(spec)
	key := "launchctl bootstrap gui/501 " + l.file()
	r := &flakyRunner{failFirst: map[string]int{key: 2}}
	if err := Apply(p, r); err != nil {
		t.Fatalf("apply: %v", err)
	}
	n := 0
	for _, c := range r.calls {
		if strings.Join(c, " ") == key {
			n++
		}
	}
	if n != 3 {
		t.Fatalf("bootstrap tried %d times, want 3", n)
	}
	r = &flakyRunner{failFirst: map[string]int{key: 100}}
	if err := Apply(p, r); err == nil {
		t.Fatal("a bootstrap that keeps failing must be reported")
	}
}
