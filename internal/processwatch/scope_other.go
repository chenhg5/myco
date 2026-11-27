//go:build !unix && !windows

package processwatch

import (
	"errors"
	"os/exec"
	"time"
)

type nativeScope struct{}

func (*nativeScope) prepare(*exec.Cmd) error                         { return errors.New("process scopes unsupported") }
func (*nativeScope) started(*exec.Cmd) error                         { return nil }
func (*nativeScope) close()                                          {}
func (*nativeScope) contains(Process, Process, map[int]Process) bool { return false }
func orphan(Process, map[int]Process, Process) bool                  { return false }
func (*nativeScope) stop(int, time.Duration) error                   { return errors.New("process scopes unsupported") }
func (*nativeScope) stopVerified(Process, []Process, time.Duration) error {
	return errors.New("process scopes unsupported")
}
func (*nativeScope) empty(Process, map[int]Process) bool { return false }

func bornWithin(Process, Process) bool { return false }
