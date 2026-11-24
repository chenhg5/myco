//go:build !windows

package browserautomation

import (
	"os/exec"
	"sync"
	"syscall"
)

func hideComponent(c *exec.Cmd) { c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }

// Clean the whole process group, including an browser left by a component crash.
func guardComponent(c *exec.Cmd) (func(), error) {
	var once sync.Once
	return func() { once.Do(func() { _ = syscall.Kill(-c.Process.Pid, syscall.SIGKILL) }) }, nil
}
