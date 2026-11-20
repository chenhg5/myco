package oscap

import (
	"context"
	"encoding/json"
	"os"
	"os/user"
	"runtime"
	"sort"
	"strings"
)

func init() {
	register(
		&Cap{Name: "system.info", About: "系统、机型、芯片、内存、开机时长", Fn: systemInfo},
		&Cap{Name: "system.disks", About: "各个磁盘：容量、剩余、格式、是否外接", Fn: systemDisks},
		&Cap{Name: "system.processes", About: "正在运行的程序：CPU、内存", Fn: systemProcesses},
		&Cap{Name: "process.kill", About: "结束一个程序", Fn: processKill},
	)
}

// SystemInfo is system.info.
type SystemInfo struct {
	// OS: darwin, windows or linux; Name: "macOS", "Windows 11", "Ubuntu 24.04.1 LTS".
	OS       string `json:"os"`
	Name     string `json:"name"`
	Version  string `json:"version"`
	Arch     string `json:"arch"`
	Model    string `json:"model,omitempty"`
	Hostname string `json:"hostname"`
	User     string `json:"user"`
	Home     string `json:"home"`
	CPUs     int    `json:"cpus"`
	CPUModel string `json:"cpu_model,omitempty"`
	// Memory: bytes of RAM; Uptime: seconds since it started.
	Memory uint64 `json:"memory"`
	Uptime int64  `json:"uptime"`
}

func systemInfo(_ context.Context, env *Env, _ json.RawMessage, _ Emit) (any, error) {
	i := SystemInfo{OS: runtime.GOOS, Arch: runtime.GOARCH, Home: env.Home, CPUs: runtime.NumCPU()}
	i.Hostname, _ = os.Hostname()
	if u, err := user.Current(); err == nil {
		i.User = u.Username
		if j := strings.LastIndexByte(i.User, '\\'); j >= 0 { // (Windows: DOMAIN\name)
			i.User = i.User[j+1:]
		}
	}
	fillInfo(&i)
	if i.Name == "" {
		i.Name = osName()
	}
	return i, nil
}

// Disk is a mounted disk (a volume).
type Disk struct {
	Mount string `json:"mount"`
	// Device: /dev/disk3s1, C:, …; FS: apfs, ntfs, ext4, …
	Device string `json:"device,omitempty"`
	FS     string `json:"fs,omitempty"`
	Label  string `json:"label,omitempty"`
	// Total, Free and Available bytes (Available: what this person can use).
	Total     uint64 `json:"total"`
	Free      uint64 `json:"free"`
	Available uint64 `json:"available"`
	// ScanPath: where to scan to see what is on it, when not Mount (macOS:
	// the startup disk's files are on its Data volume).
	ScanPath  string `json:"scan_path,omitempty"`
	Removable bool   `json:"removable,omitempty"`
	Network   bool   `json:"network,omitempty"`
	ReadOnly  bool   `json:"read_only,omitempty"`
}

func systemDisks(_ context.Context, _ *Env, _ json.RawMessage, _ Emit) (any, error) {
	ds, err := disks()
	if err != nil {
		return nil, err
	}
	if ds == nil {
		ds = []Disk{}
	}
	return ds, nil
}

// Process is a program running.
type Process struct {
	PID  int    `json:"pid"`
	PPID int    `json:"ppid"`
	Name string `json:"name"`
	// Path: the program's file, when known.
	Path string `json:"path,omitempty"`
	User string `json:"user,omitempty"`
	// CPU: percent of one core; Memory: bytes in RAM.
	CPU    float64 `json:"cpu"`
	Memory uint64  `json:"memory"`
}

func systemProcesses(ctx context.Context, env *Env, _ json.RawMessage, _ Emit) (any, error) {
	ps, err := processes(ctx, env)
	if err != nil {
		return nil, err
	}
	sort.Slice(ps, func(a, b int) bool { return ps[a].PID < ps[b].PID })
	if ps == nil {
		ps = []Process{}
	}
	return ps, nil
}

func processKill(_ context.Context, _ *Env, args json.RawMessage, _ Emit) (any, error) {
	var in struct {
		PID int `json:"pid"`
		// Force: kill at once (otherwise asked to quit: SIGTERM; Windows always kills).
		Force bool `json:"force"`
	}
	if err := decode(args, &in); err != nil {
		return nil, err
	}
	if in.PID <= 0 {
		return nil, errf(CodeInvalid, "a pid is needed")
	}
	if in.PID == os.Getpid() {
		return nil, errf(CodeInvalid, "that is the myco App itself")
	}
	return true, kill(in.PID, in.Force)
}
