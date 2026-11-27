package processwatch

import "golang.org/x/sys/unix"

func running(pid int) bool {
	k, e := unix.SysctlKinfoProc("kern.proc.pid", pid)
	return e == nil && k.Proc.P_pid == int32(pid) && k.Proc.P_stat != 5
}
