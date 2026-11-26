package harness

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestAgentBinaryOverrideAndPathPrecedence(t *testing.T) {
	for _, binary := range []string{"custom-codebuddy", "/custom/codebuddy", "claude"} {
		if got := ResolveAgentBinary(binary); got != binary {
			t.Fatalf("override %q became %q", binary, got)
		}
	}
	dir := t.TempDir()
	name := "codebuddy"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("test"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	if runtime.GOOS == "windows" {
		t.Setenv("PATHEXT", ".EXE;.CMD")
	}
	if got := ResolveAgentBinary("codebuddy"); got != p {
		t.Fatalf("PATH must win: %q", got)
	}
}
