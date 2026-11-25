//go:build !windows

package browserautomation

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestRecordedBrowserGroupsRecoveredBeforeStartAndAfterCrash(t *testing.T) {
	t.Setenv("BROWSER_FIXTURE_PRIVATE", "must-not-reach-cleanup")
	if runtime.GOOS == "linux" {
		t.Setenv("DISPLAY", ":188")
		t.Setenv("WAYLAND_DISPLAY", "wayland-fixture")
		t.Setenv("XAUTHORITY", "/tmp/fixture-xauth")
		t.Setenv("XDG_RUNTIME_DIR", "/tmp/fixture-runtime")
	}
	c := &Component{Home: t.TempDir()}
	if err := os.MkdirAll(c.dir(), 0700); err != nil {
		t.Fatal(err)
	}
	source, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	src, err := os.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	target, err := os.OpenFile(filepath.Join(c.dir(), program("myco-browser")), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0700)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = io.Copy(target, src); err != nil {
		t.Fatal(err)
	}
	if err = target.Close(); err != nil {
		t.Fatal(err)
	}
	dataDir := filepath.Join(c.Home, ".myco", "browser")
	profileDir := filepath.Join(dataDir, "profiles", "p_fixture")
	if err = os.MkdirAll(profileDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(profileDir, ".myco-process-group.json"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer c.Stop()
	if runtime.GOOS == "linux" {
		var env map[string]any
		if err = c.Call(context.Background(), "fixture", "GET", "/status", nil, &env); err != nil {
			t.Fatal(err)
		}
		if env["display"] != ":188" || env["wayland"] != "wayland-fixture" || env["xauthority"] != "/tmp/fixture-xauth" || env["runtime_dir"] != "/tmp/fixture-runtime" || env["environment_clean"] != true {
			t.Fatal("graphical environment was not safely inherited", env)
		}
	}
	if _, err = os.Stat(filepath.Join(dataDir, "cleanup-0.json")); err != nil {
		t.Fatal("startup did not invoke record recovery", err)
	}
	c.mu.Lock()
	pid := c.process.Process.Pid
	process, done := c.process, c.done
	c.mu.Unlock()
	if err = process.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("App crash cleanup did not finish")
	}
	raw, err := os.ReadFile(filepath.Join(dataDir, fmt.Sprintf("cleanup-%d.json", pid)))
	if err != nil {
		t.Fatal("dead component records not recovered", err)
	}
	var got struct {
		Dir string `json:"data_dir"`
		PID int    `json:"owner_pid"`
	}
	if json.Unmarshal(raw, &got) != nil || got.Dir != dataDir || got.PID != pid {
		t.Fatal("cleanup ownership changed")
	}
}
func TestRecordedBrowserGroupsWithoutRecordDoNotRunHelper(t *testing.T) {
	if err := cleanupRecordedBrowserGroups("/does/not/exist", t.TempDir(), 123); err != nil {
		t.Fatal(err)
	}
}

func TestRecordedBrowserGroupsRejectLegacyComponent(t *testing.T) {
	c := &Component{Home: t.TempDir()}
	os.MkdirAll(c.dir(), 0700)
	source, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	src, err := os.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	target, err := os.OpenFile(filepath.Join(c.dir(), program("myco-browser")), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0700)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = io.Copy(target, src); err != nil {
		t.Fatal(err)
	}
	target.Close()
	dataDir := filepath.Join(c.Home, ".myco", "browser")
	profileDir := filepath.Join(dataDir, "profiles", "p_fixture")
	os.MkdirAll(profileDir, 0700)
	os.WriteFile(filepath.Join(profileDir, ".myco-process-group.json"), []byte("fixture"), 0600)
	os.WriteFile(filepath.Join(dataDir, "legacy-group-protocol"), []byte("fixture"), 0600)
	if _, err = c.Start(context.Background()); err == nil || err.Error() != "incompatible_component" {
		c.Stop()
		t.Fatal("legacy component ignored retained process records", err)
	}
	if c.Status().Running {
		t.Fatal("legacy component still running")
	}
}
