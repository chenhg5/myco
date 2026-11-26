package harness

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestCodeBuddyCommandPreservesArguments(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "codebuddy.cmd")
	script := filepath.Join(dir, "codebuddy")
	if err := os.WriteFile(script, []byte("#!/usr/bin/env node\n"), 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"--acp", "--model", `model&echo unexpected`, "a'b %TEMP%"}
	binary, got := CodeBuddyCommand(bin, args)
	if binary != "node" || !slices.Equal(got, append([]string{script}, args...)) {
		t.Fatalf("command %q %#v", binary, got)
	}
	if other, got := CodeBuddyCommand("other.cmd", args); other != "other.cmd" || !slices.Equal(got, args) {
		t.Fatal("changed another CLI")
	}
}
