package quota

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os/exec"
	"path/filepath"
	"time"
)

// The keychain account is scoped to CODEX_HOME; a service-only lookup could
// silently select another login. Nothing from security's stderr is surfaced.
func codexKeychain(dir string) []byte {
	if p, e := filepath.EvalSymlinks(dir); e == nil {
		dir = p
	}
	sum := sha256.Sum256([]byte(dir))
	account := "cli|" + hex.EncodeToString(sum[:])[:16]
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/security", "find-generic-password", "-s", "Codex Auth", "-a", account, "-w")
	cmd.Stderr = io.Discard
	b, e := cmd.Output()
	if e != nil || len(b) > 1<<20 {
		return nil
	}
	return b
}
