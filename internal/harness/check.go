package harness

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// Check is the result of probing a harness installation (no quota spent).
type Check struct {
	Binary  string `json:"binary"`
	Path    string `json:"path,omitempty"`
	Version string `json:"version,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Checker is implemented by harnesses that can verify their CLI is usable.
type Checker interface {
	Check(ctx context.Context) Check
}

// CheckBinary looks the binary up on PATH and runs it with versionArgs.
func CheckBinary(ctx context.Context, binary string, versionArgs ...string) Check {
	c := Check{Binary: binary}
	path, err := exec.LookPath(binary)
	if err != nil {
		c.Error = "not found on PATH"
		return c
	}
	c.Path = path
	if len(versionArgs) == 0 {
		versionArgs = []string{"--version"}
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, versionArgs...).CombinedOutput()
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	c.Version = strings.TrimSpace(lines[len(lines)-1])
	if err != nil {
		c.Error = "version check failed: " + err.Error()
	}
	return c
}
