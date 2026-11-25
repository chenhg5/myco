//go:build linux

package main

import (
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func groupProcesses(pgid int) ([]groupProcess, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return nil, err
	}
	var out []groupProcess
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		dir := filepath.Join("/proc", entry.Name())
		info, err := os.Stat(dir)
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ESRCH) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if int(info.Sys().(*syscall.Stat_t).Uid) != os.Getuid() {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, "stat"))
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ESRCH) {
			continue
		}
		if err != nil {
			return nil, err
		}
		// comm can contain spaces and parentheses; fields after its final ')' start at state.
		end := strings.LastIndexByte(string(raw), ')')
		if end < 0 {
			return nil, errors.New("invalid process stat")
		}
		fields := strings.Fields(string(raw)[end+1:])
		if len(fields) < 20 {
			return nil, errors.New("short process stat")
		}
		group, err := strconv.Atoi(fields[2])
		if err != nil {
			return nil, err
		}
		if group != pgid && pgid != 0 {
			continue
		}
		out = append(out, groupProcess{PID: pid, PGID: group, Start: strings.TrimSpace(string(boot)) + ":" + fields[19], Zombie: fields[0] == "Z"})
	}
	return out, nil
}
func processArguments(pid int) ([]string, error) {
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return nil, err
	}
	if len(raw) > 256<<10 {
		return nil, errors.New("process arguments too large")
	}
	return strings.Split(string(raw), "\x00"), nil
}
func adoptBrowserChildren() error { return unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0) }
func reapBrowserChild(pid int) {
	var status syscall.WaitStatus
	_, _ = syscall.Wait4(pid, &status, syscall.WNOHANG, nil)
}
