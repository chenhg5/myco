//go:build linux || darwin

package processwatch

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// A bounded sleeping fixture, never an unbounded CPU loop. Cleanup is registered
// before launch and also runs when an assertion or identity read fails.
func TestSuccessfulTurnKeepsChildUntilExplicitStop(t *testing.T) {
	if _, e := readProcess(os.Getpid()); e != nil {
		t.Skipf("OS process identity unavailable: %v", e)
	}
	path := filepath.Join(t.TempDir(), "child.pid")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, "/bin/sh", "-c", `sleep 12 >/dev/null 2>&1 & echo $! > "$1"`, "helper", path)
	c.Env = []string{"PATH=/usr/bin:/bin"}
	m := New("fixture")
	t.Cleanup(m.Close)
	scope, e := m.Prepare(c, Owner{Agent: "fixture"})
	if e != nil {
		t.Fatal(e)
	}
	if e = scope.Start(); e != nil {
		t.Fatal(e)
	}
	if e = c.Wait(); e != nil {
		t.Fatal(e)
	}
	data, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	pid, e := strconv.Atoi(strings.TrimSpace(string(data)))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = readProcess(pid); e != nil {
		t.Fatal("normal turn killed background child", e)
	}
	m.StopAgent("", "fixture")
	// A zombie may await init's reaping; it cannot execute. Signal zero alone
	// cannot distinguish it, so use the platform's state in this assertion.
	if running(pid) {
		t.Fatal("child still running after explicit stop")
	}
}

// If identity acquisition fails while the context is cancelled, Cmd.Wait must
// still join the Cancel watcher without deadlocking on the manager mutex.
func TestFailedStartDoesNotWaitUnderManagerLock(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", "sleep 15")
	manager := New("failed-start")
	scope, err := manager.PrepareContext(ctx, cmd, Owner{Agent: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	cmd.Cancel = scope.Cancel
	manager.lookup = func(int) (Process, error) { cancel(); return Process{}, ErrIdentity }
	done := make(chan error, 1)
	go func() { done <- scope.Start() }()
	select {
	case err = <-done:
		if err == nil {
			t.Fatal("unverified start succeeded")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("failed start deadlocked with its cancellation watcher")
	}
}
