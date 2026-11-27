package processwatch

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// AT_CLKTCK comes from the kernel, so CPU accounting does not assume HZ=100.
func clockTicks() float64 {
	b, e := os.ReadFile("/proc/self/auxv")
	if e != nil {
		return 0
	}
	width := strconv.IntSize / 8
	get := func(b []byte) uint64 {
		if width == 8 {
			return binary.NativeEndian.Uint64(b)
		}
		return uint64(binary.NativeEndian.Uint32(b))
	}
	for i := 0; i+2*width <= len(b); i += 2 * width {
		if get(b[i:]) == 17 {
			return float64(get(b[i+width:]))
		}
	}
	return 0
}
func readProcess(pid int) (Process, error) {
	p := Process{PID: pid}
	dir := "/proc/" + strconv.Itoa(pid)
	b, err := os.ReadFile(dir + "/stat")
	if err != nil {
		return p, err
	}
	line := string(b)
	left, right := strings.IndexByte(line, '('), strings.LastIndexByte(line, ')')
	if left < 0 || right <= left {
		return p, errors.New("invalid process stat")
	}
	p.Name = line[left+1 : right]
	f := strings.Fields(line[right+1:])
	if len(f) < 22 {
		return p, errors.New("short process stat")
	}
	n := func(i int) uint64 { v, _ := strconv.ParseUint(f[i], 10, 64); return v }
	p.Parent = int(n(1))
	p.Group = int(n(2))
	hz := clockTicks()
	if hz <= 0 {
		return p, errors.New("process clock unavailable")
	}
	boot, e := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if e != nil {
		return p, e
	}
	p.Birth = strings.TrimSpace(string(boot)) + ":" + f[19]
	p.CPUTime = float64(n(11)+n(12)) / hz
	p.Memory = n(21) * uint64(os.Getpagesize())
	if st, e := os.Stat(dir); e == nil {
		if info, ok := st.Sys().(*syscall.Stat_t); ok {
			p.User = info.Uid == uint32(os.Geteuid())
		}
	}
	return p, nil
}
func readProcesses(ctx context.Context) ([]Process, error) {
	entries, e := os.ReadDir("/proc")
	if e != nil {
		return nil, e
	}
	out := []Process{}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		pid, e := strconv.Atoi(filepath.Base(entry.Name()))
		if e != nil {
			continue
		}
		if p, e := readProcess(pid); e == nil {
			out = append(out, p)
		}
	}
	return out, nil
}
func terminate(ctx context.Context, p Process, grace time.Duration) error {
	fd, e := unix.PidfdOpen(p.PID, 0)
	if e != nil {
		return e
	}
	defer unix.Close(fd)
	cur, e := readProcess(p.PID)
	if e != nil || cur.Birth != p.Birth {
		return ErrIdentity
	}
	if e = unix.PidfdSendSignal(fd, unix.SIGTERM, nil, 0); e != nil {
		return e
	}
	timer := time.NewTimer(grace)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
	}
	e = unix.PidfdSendSignal(fd, unix.SIGKILL, nil, 0)
	if e == unix.ESRCH {
		return nil
	}
	return e
}
