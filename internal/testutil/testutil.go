// Package testutil has test helpers for the differences between Unix and
// Windows.
package testutil

import (
	"runtime"
	"testing"
)

// SetHome points the home directory at dir for the test: HOME on Unix, and
// USERPROFILE too on Windows, where os.UserHomeDir ignores HOME.
func SetHome(t testing.TB, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", dir)
	}
}

// NeedsPOSIXShell skips tests whose fixtures are sh scripts or sh commands
// (fake agent CLIs, `sleep`, `>&2`).
func NeedsPOSIXShell(t testing.TB) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses sh scripts as fixtures")
	}
}

// NeedsUnixPermissions skips assertions on file mode bits, which Windows
// does not have (access is by ACL; Go reports 0666/0444).
func NeedsUnixPermissions(t testing.TB) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("file mode bits do not exist on Windows")
	}
}
