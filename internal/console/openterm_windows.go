//go:build windows

package console

import "os/exec"

// openTerminal opens a PowerShell window running command and leaves it
// open afterwards. The command is one of the app's own (agents.go), never
// user input.
func openTerminal(command string, env []string) error {
	cmd := exec.Command("cmd", "/c", "start", "", "powershell", "-NoExit", "-Command", command)
	cmd.Env = env
	return cmd.Start()
}
