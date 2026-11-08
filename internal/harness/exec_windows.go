//go:build windows

package harness

import (
	"os/exec"
	"strconv"
	"syscall"
	"time"
	"unsafe"
)

// interruptGrace is how long an interrupted agent CLI gets to save its
// session (after Ctrl-Break) before its whole process tree is killed.
var interruptGrace = 5 * time.Second

const (
	createNewProcessGroup = 0x00000200
	createNoWindow        = 0x08000000
	ctrlBreakEvent        = 1
)

var (
	kernel32                  = syscall.NewLazyDLL("kernel32.dll")
	procGenerateConsoleCtrl   = kernel32.NewProc("GenerateConsoleCtrlEvent")
	procGetConsoleWindow      = kernel32.NewProc("GetConsoleWindow")
	procGetConsoleProcessList = kernel32.NewProc("GetConsoleProcessList")
)

// hasConsole reports whether this process has a console that children can
// share (a terminal). A background runner has none.
func hasConsole() bool {
	if procGetConsoleWindow.Find() != nil {
		return false
	}
	h, _, _ := procGetConsoleWindow.Call()
	if h != 0 {
		return true
	}
	if procGetConsoleProcessList.Find() != nil {
		return false
	}
	var ids [1]uint32
	n, _, _ := procGetConsoleProcessList.Call(uintptr(unsafe.Pointer(&ids[0])), 1)
	return n > 0
}

// setProcessGroup puts the child in its own process group. With a console,
// cancelling sends it Ctrl-Break (agent CLIs save their session, like on
// Ctrl-C); without one (a background runner) the child gets no window, so
// no console pops up. Either way, whatever is left after the grace period
// is killed with its whole process tree.
func setProcessGroup(cmd *exec.Cmd) {
	console := hasConsole()
	flags := uint32(createNewProcessGroup)
	if !console {
		flags |= createNoWindow
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: flags}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		pid := cmd.Process.Pid
		killTree := func() {
			_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()
			_ = cmd.Process.Kill()
		}
		if console {
			if r, _, _ := procGenerateConsoleCtrl.Call(ctrlBreakEvent, uintptr(pid)); r != 0 {
				go func() {
					time.Sleep(interruptGrace)
					killTree()
				}()
				return nil
			}
		}
		killTree()
		return nil
	}
}
