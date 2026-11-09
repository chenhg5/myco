//go:build windows

package console

import (
	"os/exec"
	"syscall"
)

// detach starts cmd in its own process group with no console window, so it
// keeps running after the app quits.
func detach(cmd *exec.Cmd) {
	const createNewProcessGroup, detachedProcess, createNoWindow = 0x00000200, 0x00000008, 0x08000000
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewProcessGroup | detachedProcess | createNoWindow, HideWindow: true}
}
