//go:build !darwin && !windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The launcher quotes the program's path as the desktop entry spec asks.
func TestDesktopEntryQuotes(t *testing.T) {
	e := desktopEntry(`/home/a b/my"app$%`)
	if !strings.Contains(e, "Exec=\"/home/a b/my\\\\\"app\\\\$%%\"\n") {
		t.Fatalf("exec line: %s", e)
	}
	if !strings.Contains(e, "Icon=myco\n") || !strings.HasPrefix(e, "[Desktop Entry]\n") {
		t.Fatalf("entry: %s", e)
	}
}

// The window's port and key outlive a restart, in a private file.
func TestWindowStateKept(t *testing.T) {
	path := filepath.Join(t.TempDir(), "myco", "app-window.json")
	if st := loadWindowState(path); st.Port != 0 || st.Key != "" {
		t.Fatalf("nothing saved yet: %+v", st)
	}
	if err := (windowState{Port: 41234, Key: "abc"}).save(path); err != nil {
		t.Fatal(err)
	}
	st := loadWindowState(path)
	if st.Port != 41234 || st.Key != "abc" || st.url() != "http://127.0.0.1:41234/?key=abc" {
		t.Fatalf("loaded: %+v", st)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode().Perm())
	}
	if (windowState{Port: 1, Key: "x"}).alive() {
		t.Fatal("nothing listens there")
	}
}
