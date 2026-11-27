//go:build unix

package processwatch

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type nativeScope struct{}

func (*nativeScope) prepare(c *exec.Cmd) error {
	if c.SysProcAttr == nil {
		c.SysProcAttr = &syscall.SysProcAttr{}
	}
	c.SysProcAttr.Setpgid = true
	return nil
}
func (*nativeScope) started(*exec.Cmd) error { return nil }
func (*nativeScope) close()                  {}
func (*nativeScope) contains(root, p Process, all map[int]Process) bool {
	if p.Group != root.PID || !bornWithin(root, p) {
		return false
	}
	if leader, ok := all[root.PID]; ok && leader.Birth != root.Birth {
		return false
	}
	return true
}
func orphan(p Process, all map[int]Process, root Process) bool {
	if p.PID == root.PID {
		return false
	}
	parent, ok := all[p.Parent]
	return p.Parent <= 1 || !ok || parent.Name == "systemd" || parent.Name == "init" || parent.Name == "launchd"
}
func (*nativeScope) stop(pid int, grace time.Duration) error {
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	time.Sleep(grace)
	err := syscall.Kill(-pid, syscall.SIGKILL)
	if err == syscall.ESRCH {
		return nil
	}
	return err
}
func (s *nativeScope) stopVerified(root Process, members []Process, grace time.Duration) error {
	// Include all currently verifiable members of the original group even when
	// its leader finished before the minute sampler. A reused group leader denies.
	ps, err := readProcesses(context.Background())
	if err != nil {
		return err
	}
	all := map[int]Process{}
	for _, p := range ps {
		all[p.PID] = p
	}
	if leader, ok := all[root.PID]; ok && leader.Birth != root.Birth {
		return ErrIdentity
	}
	for _, p := range ps {
		if s.contains(root, p, all) {
			members = append(members, p)
		}
	}
	verified := func(groupOnly bool) bool {
		for _, p := range members {
			if groupOnly && p.Group != root.PID {
				continue
			}
			cur, e := readProcess(p.PID)
			if e == nil && cur.Birth == p.Birth {
				return true
			}
		}
		return false
	}
	if !verified(false) {
		return nil
	}
	if verified(true) {
		_ = syscall.Kill(-root.PID, syscall.SIGTERM)
	}
	for _, p := range members {
		if p.Group != root.PID {
			if cur, e := readProcess(p.PID); e == nil && cur.Birth == p.Birth {
				_ = syscall.Kill(p.PID, syscall.SIGTERM)
			}
		}
	}
	time.Sleep(grace)
	if verified(true) {
		_ = syscall.Kill(-root.PID, syscall.SIGKILL)
	}
	for _, p := range members {
		if p.Group != root.PID {
			if cur, e := readProcess(p.PID); e == nil && cur.Birth == p.Birth {
				_ = syscall.Kill(p.PID, syscall.SIGKILL)
			}
		}
	}
	return nil
}

// A failed process-table read is not evidence of an empty group. Signal 0
// checks group existence without signalling anything; EPERM means still present.
func (*nativeScope) empty(root Process, members map[int]Process) bool {
	if syscall.Kill(-root.PID, 0) != syscall.ESRCH {
		return false
	}
	for _, p := range members {
		cur, e := readProcess(p.PID)
		if e == nil {
			if cur.Birth == p.Birth {
				return false
			}
			continue
		}
		if syscall.Kill(p.PID, 0) != syscall.ESRCH {
			return false
		}
	}
	return true
}

// A re-used parent PID/group cannot claim a process older than that parent.
// Linux identity is boot-id:start-ticks; Darwin is start-seconds:microseconds.
func bornWithin(parent, child Process) bool {
	pp, pn, pok := strings.Cut(parent.Birth, ":")
	cp, cn, cok := strings.Cut(child.Birth, ":")
	if !pok || !cok {
		return false
	}
	a, ae := strconv.ParseUint(pn, 10, 64)
	b, be := strconv.ParseUint(cn, 10, 64)
	if ae != nil || be != nil {
		return false
	}
	if pp == cp {
		return b >= a
	}
	ps, pe := strconv.ParseUint(pp, 10, 64)
	cs, ce := strconv.ParseUint(cp, 10, 64)
	return pe == nil && ce == nil && cs > ps
}
