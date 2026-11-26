package quota

import (
	"context"
	"io"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

func nativeProcess(cmd *exec.Cmd) func() {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000 | 0x00000200}
	kill := func() {
		if cmd.Process == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		killer := exec.CommandContext(ctx, "taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
		killer.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
		killer.Stdout, killer.Stderr = io.Discard, io.Discard
		_ = killer.Run()
		_ = cmd.Process.Kill()
	}
	cmd.Cancel = func() error { kill(); return nil }
	return kill
}
