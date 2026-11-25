package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMemoryBlockAddRefreshRemove(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".claude"), 0o755)
	os.MkdirAll(filepath.Join(home, ".codex"), 0o755)
	getenv := func(k string) string {
		if k == "HOME" {
			return home
		}
		return ""
	}
	claude := filepath.Join(home, ".claude", "CLAUDE.md")
	codex := filepath.Join(home, ".codex", "AGENTS.md")
	os.WriteFile(claude, []byte("# My rules\nBe terse."), 0o644)
	targets := memoryTargets(getenv)
	if len(targets) != 2 || targets[0] != claude || targets[1] != codex {
		t.Fatalf("targets: %v", targets)
	}
	// Refresh adds none.
	if ch, _, _ := installMemory(targets, memoryRefresh); len(ch) != 0 {
		t.Fatalf("refresh added: %v", ch)
	}
	ch, _, err := installMemory(targets, memoryAdd)
	if err != nil || len(ch) != 2 {
		t.Fatalf("add: %v %v", ch, err)
	}
	b, _ := os.ReadFile(claude)
	if !strings.HasPrefix(string(b), "# My rules\nBe terse.\n\n"+memoryBegin) || !strings.Contains(string(b), "--attach shot.png") || !strings.Contains(string(b), "--suggest") {
		t.Fatalf("claude:\n%s", b)
	}
	if !strings.Contains(string(b), "--ui ui.json") || !strings.Contains(string(b), "myco ui schema --format text") {
		t.Fatal("joined agent memory lacks structured UI guidance")
	}
	// Again: nothing changes; an old block is replaced in place.
	if ch, _, _ := installMemory(targets, memoryAdd); len(ch) != 0 {
		t.Fatalf("second add: %v", ch)
	}
	old := strings.Replace(string(b), "myco agent", "OLD TEXT", 1) + "\nmore of mine\n"
	os.WriteFile(claude, []byte(old), 0o644)
	if ch, _, _ := installMemory(targets, memoryRefresh); len(ch) != 1 {
		t.Fatalf("refresh: %v", ch)
	}
	b, _ = os.ReadFile(claude)
	if strings.Contains(string(b), "OLD TEXT") || !strings.HasSuffix(string(b), "\nmore of mine\n") || strings.Count(string(b), memoryBegin) != 1 {
		t.Fatalf("refreshed:\n%s", b)
	}
	// Removed: stays out on join and upgrade; --add puts it back.
	if ch, _, _ := installMemory(targets, memoryRemove); len(ch) != 2 {
		t.Fatalf("remove: %v", ch)
	}
	b, _ = os.ReadFile(claude)
	if strings.Contains(string(b), memoryBegin) || !strings.Contains(string(b), memoryOff) || !strings.Contains(string(b), "Be terse.") {
		t.Fatalf("removed:\n%s", b)
	}
	if ch, off, _ := installMemory(targets, memoryAdd); len(ch) != 0 || len(off) != 2 {
		t.Fatalf("add after remove: %v %v", ch, off)
	}
	if ch, _, _ := installMemory(targets, memoryForce); len(ch) != 2 {
		t.Fatalf("force: %v", ch)
	}
	b, _ = os.ReadFile(codex)
	if strings.Contains(string(b), memoryOff) || !strings.Contains(string(b), memoryEnd) {
		t.Fatalf("back:\n%s", b)
	}
}
