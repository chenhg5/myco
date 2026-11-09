//go:build windows

package console

import (
	"os/exec"
	"syscall"
)

const (
	createNewProcessGroup = 0x00000200
	createNewConsole      = 0x00000010
)

// detach starts cmd in its own process group with its own hidden console,
// so it keeps running after the app quits and shows no window; programs it
// starts in turn share that hidden console instead of opening their own.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewProcessGroup | createNewConsole, HideWindow: true}
}

// hideWindow keeps a console program the app runs (the myco CLI) from
// flashing a console window: it gets a hidden one, shared with whatever it
// runs.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewConsole, HideWindow: true}
}
