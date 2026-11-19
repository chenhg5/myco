//go:build !windows

package console

import (
	"os"
	"os/exec"
	"syscall"
	"time"
)

// detach starts cmd in its own session, so it keeps running after the app quits.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

// hideWindow is for Windows (console windows); nothing to do elsewhere.
func hideWindow(*exec.Cmd) {}

// wslHost: WSL distributions can live on this computer.
const wslHost = false

// softCancel makes cancelling cmd interrupt it first, so a myco run stops
// what it started too (its own commands end with it), and kill it only if
// it is still there a few seconds later.
func softCancel(cmd *exec.Cmd) {
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = 5 * time.Second
}
