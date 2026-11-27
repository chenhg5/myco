package processwatch

import (
	"context"
	"fmt"
	"os/exec"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type nativeScope struct{ job windows.Handle }

func (s *nativeScope) prepare(c *exec.Cmd) error {
	h, e := windows.CreateJobObject(nil, nil)
	if e != nil {
		return e
	}
	s.job = h
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, e = windows.SetInformationJobObject(h, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); e != nil {
		s.close()
		return e
	}
	if c.SysProcAttr == nil {
		c.SysProcAttr = &syscall.SysProcAttr{}
	}
	c.SysProcAttr.CreationFlags |= windows.CREATE_SUSPENDED | windows.CREATE_NEW_PROCESS_GROUP
	return nil
}
func (s *nativeScope) started(c *exec.Cmd) error {
	h, e := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(c.Process.Pid))
	if e != nil {
		return e
	}
	defer windows.CloseHandle(h)
	if e = windows.AssignProcessToJobObject(s.job, h); e != nil {
		return fmt.Errorf("cannot contain harness in Job: %w", e)
	}
	// Go closes the initial thread handle. With CREATE_SUSPENDED the child
	// has not executed, so locate that thread, then resume only after Job assignment.
	snap, e := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if e != nil {
		return e
	}
	defer windows.CloseHandle(snap)
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	for e = windows.Thread32First(snap, &entry); e == nil; e = windows.Thread32Next(snap, &entry) {
		if entry.OwnerProcessID == uint32(c.Process.Pid) {
			th, e := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
			if e != nil {
				return e
			}
			defer windows.CloseHandle(th)
			_, e = windows.ResumeThread(th)
			return e
		}
	}
	return fmt.Errorf("cannot find suspended harness thread")
}
func (s *nativeScope) close() {
	if s.job != 0 {
		_ = windows.CloseHandle(s.job)
		s.job = 0
	}
}
func (s *nativeScope) contains(root, p Process, _ map[int]Process) bool {
	if s.job == 0 {
		return false
	}
	h, e := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(p.PID))
	if e != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var in int32
	r, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob").Call(uintptr(h), uintptr(s.job), uintptr(unsafe.Pointer(&in)))
	return r != 0 && in != 0
}
func orphan(p Process, all map[int]Process, root Process) bool {
	if p.PID == root.PID {
		return false
	}
	parent, ok := all[p.Parent]
	if !ok {
		return true
	}
	// Creation FILETIME is zero-padded decimal, so a newer reused parent is not ancestry.
	return parent.Birth > p.Birth
}
func (s *nativeScope) stop(pid int, grace time.Duration) error {
	return s.stopVerified(Process{PID: pid}, nil, grace)
}
func (s *nativeScope) stopVerified(root Process, _ []Process, grace time.Duration) error {
	if s.job == 0 {
		return nil
	}
	// Ctrl-Break is best effort; background runners do not have a console.
	if cur, e := readProcess(root.PID); e == nil && cur.Birth == root.Birth {
		r, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GenerateConsoleCtrlEvent").Call(1, uintptr(root.PID))
		if r != 0 {
			time.Sleep(grace)
		}
	}
	return windows.TerminateJobObject(s.job, 1)
}
func terminate(ctx context.Context, p Process, _ time.Duration) error {
	h, e := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE, false, uint32(p.PID))
	if e != nil {
		return e
	}
	defer windows.CloseHandle(h)
	var c, x, k, u windows.Filetime
	if e = windows.GetProcessTimes(h, &c, &x, &k, &u); e != nil {
		return e
	}
	if birth(c) != p.Birth {
		return ErrIdentity
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	return windows.TerminateProcess(h, 1)
}

func (s *nativeScope) empty(_ Process, _ map[int]Process) bool {
	if s.job == 0 {
		return true
	}
	var accounting struct {
		User, Kernel, PeriodUser, PeriodKernel int64
		Faults, Total, Active, Terminated      uint32
	}
	e := windows.QueryInformationJobObject(s.job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&accounting)), uint32(unsafe.Sizeof(accounting)), nil)
	return e == nil && accounting.Active == 0
}

func bornWithin(parent, child Process) bool {
	return len(parent.Birth) == 20 && len(child.Birth) == 20 && child.Birth >= parent.Birth
}
