package cli

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

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/service"
)

// restartAfter gives the machine node's answer time to reach the hub before
// the runner (which sends it) is restarted.
var restartAfter = 4 * time.Second

// upgradeThisMachine is the machine node's answer to an upgrade request
// from the console: `myco upgrade --no-restart` with this binary, then the
// runner service restarted a few seconds later (detached: the restart
// stops this process), so everything here runs the new release.
func (e *env) upgradeThisMachine(ctx context.Context, version string) (api.MachineUpgrade, error) {
	exe, err := os.Executable()
	if err != nil {
		return api.MachineUpgrade{}, err
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	args := []string{"upgrade", "--no-restart", "--format", "json"}
	if version != "" {
		args = append(args, "--version", version)
	}
	cctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(cctx, exe, args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	runErr := cmd.Run()
	var res struct {
		Current string `json:"current"`
		Target  string `json:"target"`
		Message string `json:"message"`
		Suggest string `json:"suggestion"`
	}
	_ = json.Unmarshal(out.Bytes(), &res)
	if runErr != nil {
		msg := strings.TrimSpace(res.Message)
		if msg == "" {
			msg = strings.TrimSpace(lastLineOf(errb.String()))
		}
		if res.Suggest != "" {
			msg += " (" + res.Suggest + ")"
		}
		return api.MachineUpgrade{From: Version}, errors.New(orDefault(msg, runErr.Error()))
	}
	up := api.MachineUpgrade{From: Version, To: res.Target}
	if res.Target == "" || res.Target == Version {
		up.To = "" // already running it
		return up, nil
	}
	// The runner restarts onto the new binary once this answer is out.
	home, _ := os.UserHomeDir()
	m, err := service.DetectID(home, service.ExecRunner{}, service.RunnerID)
	if err == nil {
		if st, err := m.Status(); err == nil && st.Installed {
			restart := exec.Command(exe, "runner", "service", "restart")
			detachProcess(restart)
			time.AfterFunc(restartAfter, func() {
				if err := restart.Start(); err == nil {
					go func() { _ = restart.Wait() }()
				}
			})
			up.Restarting = true
			return up, nil
		}
	}
	up.Note = "myco was upgraded, but its runner does not run as a service here: restart it to use the new version"
	return up, nil
}

func lastLineOf(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}
