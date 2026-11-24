//go:build !windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

// The browser inherits the component's private process group. The App can
// clean that whole owned group after a component crash; a session stop signals
// only its own browser PID, never another Chrome process or Browser.close.
func guardBrowser(cmd *exec.Cmd) (func(), error) {
	return func() { _ = cmd.Process.Signal(syscall.SIGTERM) }, nil
}
func lockProfile(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}
