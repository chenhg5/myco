//go:build windows

package console

import (
	"os/exec"
	"syscall"
)

// launchApp starts the app's new version in its own process group,
// detached from this one, which is about to quit. Not hidden: it is a
// window app.
func launchApp(exe string) (waiter, error) {
	cmd := exec.Command(exe)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000200 | 0x00000008} // CREATE_NEW_PROCESS_GROUP | DETACHED_PROCESS
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmdWaiter{cmd}, nil
}
