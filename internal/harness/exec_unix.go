//go:build unix

package harness

import (
	"os/exec"
	"syscall"
	"time"
)

// interruptGrace is how long an interrupted agent CLI gets to save its
// session (after SIGINT) before the whole process group is killed.
var interruptGrace = 5 * time.Second

func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// Interrupt the whole group (agent CLIs save their session on
		// SIGINT, like Ctrl-C), then kill whatever is left.
		pgid := cmd.Process.Pid
		_ = syscall.Kill(-pgid, syscall.SIGINT)
		go func() {
			time.Sleep(interruptGrace)
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		}()
		return nil
	}
}
