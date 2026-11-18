package runner

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSessionLocks(t *testing.T) {
	dir := t.TempDir()
	a := NewSessionLocks(dir)
	unlock, ok := a.TryLock("claude:s1")
	if !ok {
		t.Fatal("a free session should lock")
	}
	if _, ok := a.TryLock("claude:s1"); ok {
		t.Fatal("the same session twice in one process")
	}
	if u, ok := a.TryLock("claude:s2"); !ok {
		t.Fatal("another session is free")
	} else {
		u()
	}
	// Another runner process (another lock set on the same directory).
	b := NewSessionLocks(dir)
	if _, ok := b.TryLock("claude:s1"); ok {
		t.Fatal("the same session from another process")
	}
	unlock()
	u, ok := b.TryLock("claude:s1")
	if !ok {
		t.Fatal("released: the other process takes it")
	}
	u()

	// A lock file left by a runner that died mid-turn is taken over once stale.
	c := &SessionLocks{Dir: dir, StaleAfter: time.Minute}
	u, ok = c.TryLock("codex:s3")
	if !ok {
		t.Fatal("lock")
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.lock"))
	if len(files) != 1 {
		t.Fatalf("lock files: %v", files)
	}
	old := time.Now().Add(-2 * time.Minute)
	os.Chtimes(files[0], old, old)
	d := &SessionLocks{Dir: dir, StaleAfter: time.Minute}
	if u2, ok := d.TryLock("codex:s3"); !ok {
		t.Fatal("a stale lock should be taken over")
	} else {
		u2()
	}
	u()

	// No locks configured: everything passes.
	var none *SessionLocks
	if u, ok := none.TryLock("claude:s1"); !ok {
		t.Fatal("nil locks")
	} else {
		u()
	}
}
