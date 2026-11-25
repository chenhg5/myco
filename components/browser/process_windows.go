package main

import (
	"golang.org/x/sys/windows"
	"io"
	"os"
	"os/exec"
	"sync"
	"unsafe"
)

func guardBrowser(cmd *exec.Cmd, _ string) (func() error, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		return nil, err
	}
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		windows.CloseHandle(job)
		return nil, err
	}
	defer windows.CloseHandle(process)
	if err = windows.AssignProcessToJobObject(job, process); err != nil {
		windows.CloseHandle(job)
		return nil, err
	}
	var once sync.Once
	return func() error { once.Do(func() { _ = windows.CloseHandle(job) }); return nil }, nil
}
func lockProfile(path string) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(h), path), nil
}

func newBrowserCommand(binary string, args []string, _ string, _ *os.File) (*exec.Cmd, error) {
	return exec.Command(binary, args...), nil
}
func recoverProfileProcesses(string, any) error             { return nil }
func adoptBrowserChildren() error                           { return nil }
func browserInternalMode([]string, io.Reader) (bool, error) { return false, nil }

type profileProcessError struct{ Reason string }

func (e *profileProcessError) Error() string { return "profile_locked: " + e.Reason }
func groupError(reason string) error         { return &profileProcessError{Reason: reason} }

func recoverAllBrowserGroups(string) {}

func browserGroupProtocol() int { return 0 }

func browserCommandArguments(cmd *exec.Cmd) ([]string, error) { return cmd.Args, nil }

func recoverFailedBrowserCommand(cmd *exec.Cmd, _ string) error { _ = cmd.Process.Kill(); return nil }
