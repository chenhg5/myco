package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestExecNodeName(t *testing.T) {
	home := filepath.FromSlash("/home/alice")
	for _, tc := range []struct{ dir, want string }{
		{home, "box-shell"}, // as before: re-joining recovers it
		{filepath.Join(home, "code", "acme"), "box-acme-shell"},
		{filepath.Join(home, "code", "My App!"), "box-my-app-shell"},
		{filepath.Join(home, "code", "项目"), "box-shell"}, // nothing left of the name
		{filepath.Join(home, "x", strings.Repeat("a", 80)), "box-" + strings.Repeat("a", 63-len("box-")-len("-shell")) + "-shell"},
	} {
		got := execNodeName("box-", tc.dir, home)
		if got != tc.want || len(got) > 63 {
			t.Errorf("execNodeName(%q) = %q, want %q", tc.dir, got, tc.want)
		}
	}
}
