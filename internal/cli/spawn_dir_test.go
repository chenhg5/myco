package cli

import (
	"github.com/chenhg5/myco/internal/testutil"
	"path/filepath"
	"testing"
)

// remoteDir hands the other machine "~/..." with forward slashes, whatever
// this machine's separator, and leaves other paths alone.
func TestRemoteDir(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)
	for in, want := range map[string]string{
		home:                                   "~",
		filepath.Join(home, "code", "app"):     "~/code/app",
		filepath.Join(home+"x", "code"):        filepath.Join(home+"x", "code"),
		filepath.Join(filepath.Dir(home), "o"): filepath.Join(filepath.Dir(home), "o"),
		"~/already":                            "~/already",
	} {
		if got := remoteDir(in); got != want {
			t.Errorf("remoteDir(%q) = %q, want %q", in, got, want)
		}
	}
}
