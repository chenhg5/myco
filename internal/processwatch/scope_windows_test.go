package processwatch

import (
	"context"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestJobHelperChild(t *testing.T) {
	if len(os.Args) < 2 || os.Args[len(os.Args)-1] != "processwatch-child" {
		t.Skip("subprocess helper")
	}
	time.Sleep(12 * time.Second)
	os.Exit(0)
}
func TestJobHelperRoot(t *testing.T) {
	if len(os.Args) < 3 || os.Args[len(os.Args)-2] != "processwatch-root" {
		t.Skip("subprocess helper")
	}
	self, _ := os.Executable()
	c := exec.Command(self, "-test.run=^TestJobHelperChild$", "--", "processwatch-child")
	c.Env = []string{"SystemRoot=" + os.Getenv("SystemRoot")}
	if c.Start() != nil {
		os.Exit(2)
	}
	if os.WriteFile(os.Args[len(os.Args)-1], []byte(strconv.Itoa(c.Process.Pid)), 0600) != nil {
		os.Exit(3)
	}
	os.Exit(0)
}
func TestJobRetainsDescendantAfterLeaderExits(t *testing.T) {
	self, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(t.TempDir(), "pid")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, self, "-test.run=^TestJobHelperRoot$", "--", "processwatch-root", path)
	c.Env = []string{"SystemRoot=" + os.Getenv("SystemRoot")}
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
	p, e := readProcess(pid)
	if e != nil || !windowsRunning(pid) {
		t.Fatal("normal turn killed child", e)
	}
	wrong := p
	wrong.Birth = "wrong"
	if e = terminate(ctx, wrong, time.Millisecond); e != ErrIdentity || !windowsRunning(pid) {
		t.Fatal("identity mismatch did not refuse termination", e)
	}
	m.StopAgent("", "fixture")
	deadline := time.Now().Add(time.Second)
	for windowsRunning(pid) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if windowsRunning(pid) {
		t.Fatal("Job descendant survived explicit stop")
	}
}
func windowsRunning(pid int) bool {
	h, e := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if e != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	return windows.GetExitCodeProcess(h, &code) == nil && code == 259
}
