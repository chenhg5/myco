//go:build !windows

package cli

import (
	"os/exec"
	"syscall"
)

// detachProcess starts cmd in its own session, so it outlives the command
// that started it.
func detachProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
