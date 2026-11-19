package console

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The inventory is answered from memory once read, and read again after a
// session joined (forgotten); a refresh never makes the answer wait.
func TestMachineInventoryCached(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	write := func(id, text string) {
		dir := filepath.Join(home, ".claude", "projects", "-code-app")
		os.MkdirAll(dir, 0o755)
		line, _ := json.Marshal(map[string]any{"type": "user", "cwd": home, "sessionId": id, "timestamp": "2026-11-18T10:00:00Z",
			"message": map[string]any{"role": "user", "content": text}})
		os.WriteFile(filepath.Join(dir, id+".jsonl"), append(line, '\n'), 0o644)
	}
	write("11111111-1111-4111-8111-111111111111", "first conversation")
	_, s1 := MachineInventoryCached(context.Background(), home, "http://hub")
	if len(s1) != 1 {
		t.Fatalf("first read: %d sessions", len(s1))
	}
	write("22222222-2222-4222-8222-222222222222", "second conversation")
	start := time.Now()
	_, s2 := MachineInventoryCached(context.Background(), home, "http://hub")
	if len(s2) != 1 || time.Since(start) > time.Second {
		t.Fatalf("a fresh inventory is answered from memory: %d sessions in %v", len(s2), time.Since(start))
	}
	ForgetMachineInventory(home, "http://hub")
	_, s3 := MachineInventoryCached(context.Background(), home, "http://hub")
	if len(s3) != 2 {
		t.Fatalf("after forgetting it is read again: %d sessions", len(s3))
	}
}
