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
