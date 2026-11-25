//go:build !windows

package browserautomation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Recovery is implemented by the same component that owns the record format
// and process identity checks. Never kill a group from a bare numeric PID.
func cleanupRecordedBrowserGroups(binary, dataDir string, ownerPID int) error {
	found, err := hasRecordedBrowserGroups(dataDir)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	cfg, err := json.Marshal(map[string]any{"data_dir": dataDir, "owner_pid": ownerPID})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "--cleanup-browser-groups")
	for _, key := range []string{"HOME", "PATH", "TMPDIR", "TMP", "TEMP", "LANG", "LC_ALL"} {
		if value := os.Getenv(key); value != "" {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
	}
	cmd.Stdin = bytes.NewReader(cfg)
	if err = cmd.Run(); err != nil {
		return errors.New("browser_group_cleanup_failed")
	}
	return nil
}

func hasRecordedBrowserGroups(dataDir string) (bool, error) {
	entries, err := os.ReadDir(filepath.Join(dataDir, "profiles"))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "p_") {
			continue
		}
		if _, e := os.Lstat(filepath.Join(dataDir, "profiles", entry.Name(), ".myco-process-group.json")); e == nil {
			return true, nil
		}
	}
	return false, nil
}

func componentStopGrace() time.Duration { return 15 * time.Second }
