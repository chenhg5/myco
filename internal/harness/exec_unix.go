//go:build unix

package harness

import (
	"os/exec"
	"syscall"
)

func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// Kill the whole group so agent sub-processes die too.
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
