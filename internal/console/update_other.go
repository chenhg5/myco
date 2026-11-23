//go:build !windows

package console

import (
	"os/exec"
	"syscall"
)

// launchApp starts a program in its own session (the hand-over is used
// on Windows; this keeps the code testable elsewhere).
func launchApp(exe string) (waiter, error) {
	cmd := exec.Command(exe)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmdWaiter{cmd}, nil
}
