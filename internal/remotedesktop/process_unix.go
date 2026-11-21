//go:build !windows

package remotedesktop

import (
	"os/exec"
	"syscall"
)

func hideComponent(c *exec.Cmd) { c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }

// Clean the whole process group, including an encoder left by a component crash.
func guardComponent(c *exec.Cmd) (func(), error) {
	return func() { _ = syscall.Kill(-c.Process.Pid, syscall.SIGKILL) }, nil
}
