package runner

import (
	"context"
	"runtime"
	"strings"
	"testing"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/harness"
)

func TestCommandWatchExecWithoutModel(t *testing.T) {
	for _, tc := range []struct {
		cmd, match string
		exit       int
		met        bool
	}{
		{"echo ready", "^ready$", 0, true},
		{"echo waiting", "ready", 0, false},
		{"exit 7", "", 7, false},
		{"echo ready", "", 0, true},
	} {
		result := executeWatchCommand(context.Background(), api.Watch{Command: tc.cmd, Match: tc.match, EverySec: 30}, t.TempDir())
		if result.ExitCode != tc.exit || result.Matched != tc.met {
			t.Fatalf("%s: %+v", tc.cmd, result)
		}
	}
}

func TestCommandWatchTimeoutAndOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fixture; permission and normal shell checks run natively on Windows")
	}
	w := api.Watch{Command: "echo before; sleep 20", EverySec: 1}
	result := executeWatchCommand(context.Background(), w, t.TempDir())
	if result.ExitCode != 124 || result.Matched || result.Result != "command timed out" {
		t.Fatalf("timeout: %+v", result)
	}
	w.Command = "printf 'old\\n'; printf 'ready token=private-value\\n'"
	result = executeWatchCommand(context.Background(), w, t.TempDir())
	if strings.Contains(result.Result, "private-value") || !strings.Contains(result.Result, "[redacted]") {
		t.Fatalf("redaction: %+v", result)
	}
	w.Command = "yes abc | head -n 30000; echo last"
	w.EverySec = 30
	result = executeWatchCommand(context.Background(), w, t.TempDir())
	if !strings.HasPrefix(result.Result, "last") || !strings.Contains(result.Result, "truncated") {
		t.Fatalf("bounded latest result: %+v", result)
	}
}

func TestCommandWatchPermissionIsLocal(t *testing.T) {
	r := New(newFakeHub(), fakeHarness{}, NewMemorySessions(), nil, Options{Self: "watcher", Access: harness.AccessFull, Workdir: "/demo", CommandEnvironment: "demo", CommandRunnerID: "run", CommandWatchAllowed: func() bool { return true }}, quiet())
	a := &api.Agent{Name: "watcher", MachineID: "m", Binding: api.Binding{Environment: "demo", Workdir: "/demo"}, Runtime: &api.Runtime{Access: "full", MaxAccess: "full", Features: []string{"command_watch"}, RunnerInstance: "run"}}
	w := &api.Watch{Agent: "watcher", MachineID: "m", Environment: "demo", Workdir: "/demo"}
	if !r.commandPermission(a, w) {
		t.Fatal("permitted local full-access watch rejected")
	}
	w.MachineID = "another"
	if r.commandPermission(a, w) {
		t.Fatal("foreign computer allowed")
	}
	w.MachineID = "m"
	a.RunSettings = &api.RunSettings{Access: "workspace"}
	if r.commandPermission(a, w) {
		t.Fatal("workspace sandbox bypassed")
	}
	a.RunSettings = nil
	r.opts.CommandWatchAllowed = func() bool { return false }
	if r.commandPermission(a, w) {
		t.Fatal("local policy bypassed")
	}
}
