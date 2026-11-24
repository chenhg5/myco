//go:build darwin || linux

package oscap

import (
	"bufio"
	"bytes"
	"context"
	"io/fs"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func crossDevice(error) bool { return false } // (EXDEV, checked by the caller)

func statOf(fi fs.FileInfo) *syscall.Stat_t {
	st, _ := fi.Sys().(*syscall.Stat_t)
	return st
}

func devOf(fi fs.FileInfo) (uint64, bool) {
	if st := statOf(fi); st != nil {
		return uint64(st.Dev), true
	}
	return 0, false
}

func diskOf(_ string, fi fs.FileInfo) int64 {
	if st := statOf(fi); st != nil {
		return int64(st.Blocks) * 512
	}
	return fi.Size()
}

func inodeOf(fi fs.FileInfo) ([2]uint64, bool) {
	if st := statOf(fi); st != nil {
		return [2]uint64{uint64(st.Dev), uint64(st.Ino)}, true
	}
	return [2]uint64{}, false
}

func linksOf(fi fs.FileInfo) uint64 {
	if st := statOf(fi); st != nil {
		return uint64(st.Nlink)
	}
	return 1
}

func shellArgv(_ *Env, script string) []string { return []string{"/bin/sh", "-c", script} }

// ownGroup puts a command in its own process group, so that stopping it
// stops what it started too.
func ownGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killTree(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		return cmd.Process.Kill()
	}
	return nil
}

func hideWindow(*exec.Cmd) {}

func kill(pid int, force bool) error {
	sig := syscall.SIGTERM
	if force {
		sig = syscall.SIGKILL
	}
	if err := syscall.Kill(pid, sig); err != nil {
		if err == syscall.ESRCH {
			return errf(CodeNotFound, "no process %d", pid)
		}
		if err == syscall.EPERM {
			return errf(CodePermission, "process %d is not this person's", pid)
		}
		return err
	}
	return nil
}

// processes asks ps (the same columns on macOS and Linux).
func processes(ctx context.Context, env *Env) ([]Process, error) {
	out, err := runOut(ctx, env, nil, nil, "ps", "-axww", "-o", "pid=,ppid=,pcpu=,rss=,user=,comm=")
	if err != nil {
		return nil, err
	}
	var ps []Process
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 6 {
			continue
		}
		pid, err := strconv.Atoi(f[0])
		if err != nil {
			continue
		}
		ppid, _ := strconv.Atoi(f[1])
		cpu, _ := strconv.ParseFloat(strings.ReplaceAll(f[2], ",", "."), 64)
		rss, _ := strconv.ParseUint(f[3], 10, 64)
		// (the command is the rest of the line: a path may have spaces)
		comm := strings.Join(f[5:], " ")
		p := Process{PID: pid, PPID: ppid, CPU: cpu, Memory: rss * 1024, User: f[4], Name: filepath.Base(comm)}
		if strings.HasPrefix(comm, "/") {
			p.Path = comm
		}
		ps = append(ps, p)
	}
	return ps, nil
}

func exeNames(p string) []string { return []string{p} }
