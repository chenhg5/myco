package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func groupProcesses(pgid int) ([]groupProcess, error) {
	all, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil, err
	}
	var out []groupProcess
	for _, p := range all {
		if pgid != 0 && int(p.Eproc.Pgid) != pgid {
			continue
		}
		if int(p.Eproc.Ucred.Uid) != os.Getuid() {
			if pgid != 0 {
				return nil, errors.New("foreign process group")
			}
			continue
		}
		out = append(out, groupProcess{PID: int(p.Proc.P_pid), PGID: int(p.Eproc.Pgid), Start: fmt.Sprintf("%d:%d", p.Proc.P_starttime.Sec, p.Proc.P_starttime.Usec), Zombie: p.Proc.P_stat == 5})
	}
	return out, nil
}
func processArguments(pid int) ([]string, error) {
	raw, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil {
		return nil, err
	}
	if len(raw) < 4 || len(raw) > 256<<10 {
		return nil, errors.New("invalid process arguments")
	}
	argc := int(binary.LittleEndian.Uint32(raw[:4]))
	if argc < 0 || argc > 16384 {
		return nil, errors.New("invalid argument count")
	}
	raw = raw[4:]
	end := bytes.IndexByte(raw, 0)
	if end < 0 {
		return nil, errors.New("invalid executable path")
	}
	raw = bytes.TrimLeft(raw[end+1:], "\x00")
	var out []string
	for len(out) < argc {
		end = bytes.IndexByte(raw, 0)
		if end < 0 {
			return nil, errors.New("short process arguments")
		}
		out = append(out, string(raw[:end]))
		raw = raw[end+1:]
	}
	return out, nil
}
func adoptBrowserChildren() error { return nil }
func reapBrowserChild(int)        {}
