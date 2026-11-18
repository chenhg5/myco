package console

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// What the App runs (myco join --fork, the runner service it installs)
// finds claude and codex where the person's terminal does: the login
// shell's PATH and the usual install places come after the App's own.
func TestEnvPathHasLoginShellAndAgentDirs(t *testing.T) {
	home := t.TempDir()
	nvm := filepath.Join(home, ".nvm", "versions", "node")
	for _, v := range []string{"v18.20.0", "v22.11.0"} {
		os.MkdirAll(filepath.Join(nvm, v, "bin"), 0o755)
	}
	l := &Local{Home: home, Dirs: []string{"/app/bin"}, LookPath: func(string) (string, error) { return "", os.ErrNotExist },
		ShellPath: func() string { return "/shell/one" + string(os.PathListSeparator) + "/app/bin" }}
	var path string
	for _, kv := range l.env() {
		if v, ok := strings.CutPrefix(kv, "PATH="); ok {
			path = v
		}
	}
	dirs := filepath.SplitList(path)
	idx := map[string]int{}
	for i, d := range dirs {
		if _, dup := idx[d]; dup {
			t.Fatalf("%s twice in PATH %s", d, path)
		}
		idx[d] = i
	}
	for _, want := range []string{"/app/bin", "/shell/one", filepath.Join(home, ".claude", "local"), filepath.Join(nvm, "v22.11.0", "bin")} {
		if _, ok := idx[want]; !ok {
			t.Fatalf("%s missing from PATH %s", want, path)
		}
	}
	if _, ok := idx[filepath.Join(nvm, "v18.20.0", "bin")]; ok {
		t.Fatalf("only the newest nvm node: %s", path)
	}
	if idx["/app/bin"] > idx["/shell/one"] {
		t.Fatalf("the App's own directories come first: %s", path)
	}
}

func TestLoginShellPathSkipsWhatRCFilesPrint(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no login shell on Windows")
	}
	sh := filepath.Join(t.TempDir(), "fakesh")
	os.WriteFile(sh, []byte("#!/bin/sh\necho 'welcome from .zshrc'\nprintf '\\n__MYCO_PATH__/from/shell:/usr/bin\\n'\necho 'bye'\n"), 0o755)
	t.Setenv("SHELL", sh)
	if got := LoginShellPath(); got != "/from/shell:/usr/bin" {
		t.Fatalf("LoginShellPath = %q", got)
	}
}
