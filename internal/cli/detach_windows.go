//go:build windows

package cli

import (
	"os/exec"
	"syscall"
)

// detachProcess starts cmd in its own process group with a hidden console,
// so it outlives the command that started it and shows no window.
func detachProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000200 | 0x00000010, HideWindow: true}
}
