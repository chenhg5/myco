//go:build !windows

package browserautomation

import (
	"os/exec"
	"sync"
	"syscall"
)

func hideComponent(c *exec.Cmd) { c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }

// Clean the component group (including browsers from older components).
// New browsers have their own recorded groups; the component recovery helper
// validates and cleans those separately after this group exits.
func guardComponent(c *exec.Cmd) (func(), error) {
	var once sync.Once
	return func() { once.Do(func() { _ = syscall.Kill(-c.Process.Pid, syscall.SIGKILL) }) }, nil
}
