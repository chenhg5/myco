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

// hideWindow is for Windows (console windows); nothing to do elsewhere.
func hideWindow(*exec.Cmd) {}

// wslHost: WSL distributions can live on this computer.
const wslHost = false
