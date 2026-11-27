package processwatch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

func readProcess(pid int) (Process, error) {
	k, e := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if e != nil {
		return Process{}, e
	}
	if k.Proc.P_pid != int32(pid) {
		return Process{}, ErrIdentity
	}
	// ps -o comm never includes argv. Kernel start timeval supplies identity;
	// ps lstart would lose sub-second precision and is not sufficient.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, "/bin/ps", "-p", strconv.Itoa(pid), "-o", "pgid=,time=,rss=")
	c.Env = append(os.Environ(), "LC_ALL=C")
	b, e := c.Output()
	if e != nil {
		return Process{}, e
	}
	f := strings.Fields(string(b))
	if len(f) != 3 {
		return Process{}, errors.New("incomplete process accounting")
	}
	group, e := strconv.Atoi(f[0])
	if e != nil {
		return Process{}, e
	}
	rss, e := strconv.ParseUint(f[2], 10, 64)
	if e != nil {
		return Process{}, e
	}
	name := strings.TrimRight(string(k.Proc.P_comm[:]), "\x00")
	p := Process{PID: pid, Parent: int(k.Eproc.Ppid), Group: group, Name: name, Birth: fmt.Sprintf("%d:%d", k.Proc.P_starttime.Sec, k.Proc.P_starttime.Usec), CPUTime: parseCPUTime(f[1]), Memory: rss * 1024, User: k.Eproc.Ucred.Uid == uint32(os.Geteuid())}
	// Fail closed if this PID changed while ps read its accounting.
	again, e := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if e != nil || again.Proc.P_starttime != k.Proc.P_starttime {
		return Process{}, ErrIdentity
	}
	return p, nil
}
func parseCPUTime(s string) float64 {
	days := 0.0
	if a, b, ok := strings.Cut(s, "-"); ok {
		days, _ = strconv.ParseFloat(a, 64)
		s = b
	}
	n := days * 86400
	parts := strings.Split(s, ":")
	v := 0.0
	for _, p := range parts {
		x, _ := strconv.ParseFloat(p, 64)
		v = v*60 + x
	}
	return n + v
}
func readProcesses(ctx context.Context) ([]Process, error) {
	// One bounded ps invocation per sample, not one subprocess per process.
	scanCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	c := exec.CommandContext(scanCtx, "/bin/ps", "-axo", "pid=,time=,rss=")
	c.Env = append(os.Environ(), "LC_ALL=C")
	data, e := c.Output()
	if e != nil {
		return nil, e
	}
	ks, e := unix.SysctlKinfoProcSlice("kern.proc.all")
	if e != nil {
		return nil, e
	}
	identities := map[int]unix.KinfoProc{}
	for _, k := range ks {
		if k.Eproc.Ucred.Uid == uint32(os.Geteuid()) {
			identities[int(k.Proc.P_pid)] = k
		}
	}
	out := []Process{}
	for _, line := range strings.Split(string(data), "\n") {
		if e = ctx.Err(); e != nil {
			return nil, e
		}
		f := strings.Fields(line)
		if len(f) != 3 {
			continue
		}
		pid, e := strconv.Atoi(f[0])
		if e != nil {
			continue
		}
		k, ok := identities[pid]
		if !ok {
			continue
		}
		rss, e := strconv.ParseUint(f[2], 10, 64)
		if e != nil {
			continue
		}
		out = append(out, Process{PID: pid, Parent: int(k.Eproc.Ppid), Group: int(k.Eproc.Pgid), Name: strings.TrimRight(string(k.Proc.P_comm[:]), "\x00"), Birth: fmt.Sprintf("%d:%d", k.Proc.P_starttime.Sec, k.Proc.P_starttime.Usec), CPUTime: parseCPUTime(f[1]), Memory: rss * 1024, User: true})
	}
	return out, nil
}
func terminate(ctx context.Context, p Process, grace time.Duration) error {
	cur, e := readProcess(p.PID)
	if e != nil || cur.Birth != p.Birth {
		return ErrIdentity
	}
	if e = unix.Kill(p.PID, unix.SIGTERM); e != nil {
		return e
	}
	t := time.NewTimer(grace)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
	}
	cur, e = readProcess(p.PID)
	if e != nil {
		return nil
	}
	if cur.Birth != p.Birth {
		return ErrIdentity
	}
	return unix.Kill(p.PID, unix.SIGKILL)
}
