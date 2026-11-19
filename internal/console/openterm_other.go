//go:build !windows

package console

import (
	"errors"
	"os/exec"
	"runtime"
	"strings"
)

// openTerminal opens a terminal window running command and leaves it open
// afterwards: Terminal on macOS, the desktop's terminal on Linux. The
// command is one of the app's own (agents.go), never user input.
func openTerminal(command string, env []string) error {
	if runtime.GOOS == "darwin" {
		// Terminal runs it in a login shell, with the person's own PATH.
		script := `tell application "Terminal" to do script "` + strings.ReplaceAll(command, `"`, `\"`) + `"`
		return exec.Command("osascript", "-e", script, "-e", `tell application "Terminal" to activate`).Run()
	}
	shell := command + "; exec bash"
	for _, t := range [][]string{
		{"x-terminal-emulator", "-e", "bash", "-lc", shell},
		{"gnome-terminal", "--", "bash", "-lc", shell},
		{"konsole", "-e", "bash", "-lc", shell},
		{"xterm", "-e", "bash", "-lc", shell},
	} {
		if p, err := exec.LookPath(t[0]); err == nil {
			cmd := exec.Command(p, t[1:]...)
			cmd.Env = env
			return cmd.Start()
		}
	}
	return errors.New("no terminal program found")
}
