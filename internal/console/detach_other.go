//go:build !windows

package console

import (
	"os/exec"
	"syscall"
)

// detach starts cmd in its own session, so it keeps running after the app quits.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
