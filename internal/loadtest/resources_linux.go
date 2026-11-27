package loadtest

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var tickOnce sync.Once
var tickHz = 100.0

func clockTicks() float64 {
	tickOnce.Do(func() {
		if b, e := exec.Command("getconf", "CLK_TCK").Output(); e == nil {
			if n, e := strconv.ParseFloat(strings.TrimSpace(string(b)), 64); e == nil && n > 0 {
				tickHz = n
			}
		}
	})
	return tickHz
}
func num(s string) uint64 { v, _ := strconv.ParseUint(s, 10, 64); return v }
func proc(pid int) processResource {
	var p processResource
	base := "/proc/" + strconv.Itoa(pid)
	b, e := os.ReadFile(base + "/stat")
	if e != nil {
		return p
	}
	k := strings.LastIndexByte(string(b), ')')
	if k < 0 {
		return p
	}
	f := strings.Fields(string(b)[k+1:])
	if len(f) > 21 {
		p.CPUTicks = num(f[11]) + num(f[12])
		p.RSSBytes = int64(num(f[21])) * int64(os.Getpagesize())
		p.Threads = int(num(f[17]))
	}
	if a, e := os.ReadDir(base + "/fd"); e == nil {
		p.FDs = len(a)
	}
	if b, e = os.ReadFile(base + "/io"); e == nil {
		p.IOAvailable = true
		for _, line := range strings.Split(string(b), "\n") {
			f := strings.Fields(line)
			if len(f) != 2 {
				continue
			}
			switch f[0] {
			case "rchar:":
				p.LogicalRead = num(f[1])
			case "read_bytes:":
				p.DiskRead = num(f[1])
			case "write_bytes:":
				p.DiskWrite = num(f[1])
			}
		}
	}
	return p
}
func diskFor(dir string) *diskResource {
	b, e := os.ReadFile("/proc/self/mountinfo")
	if e != nil {
		return nil
	}
	dev, mount := "", ""
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 6 {
			continue
		}
		point := strings.ReplaceAll(f[4], `\040`, " ")
		if (dir == point || strings.HasPrefix(dir, strings.TrimRight(point, "/")+"/")) && len(point) > len(mount) {
			mount = point
			dev = f[2]
		}
	}
	a, bdev, ok := strings.Cut(dev, ":")
	if !ok {
		return nil
	}
	f, e := os.Open("/proc/diskstats")
	if e != nil {
		return nil
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		x := strings.Fields(s.Text())
		if len(x) < 14 || x[0] != a || x[1] != bdev {
			continue
		}
		d := &diskResource{Device: x[2], Writes: num(x[7]), WriteMS: num(x[10]), BusyMS: num(x[12]), QueueMS: num(x[13])}
		if len(x) >= 20 {
			d.Flushes = num(x[18])
			d.FlushMS = num(x[19])
		}
		return d
	}
	return nil
}
func readResources(pid int, dir string, prev *resourceSample) resourceSample {
	s := resourceSample{At: time.Now().UTC(), Supported: true, Hub: proc(pid), Generator: proc(os.Getpid()), Disk: diskFor(filepath.Clean(dir))}
	var st syscall.Statfs_t
	if syscall.Statfs(dir, &st) == nil {
		free, total := st.Bavail*uint64(st.Bsize), st.Blocks*uint64(st.Bsize)
		s.DiskFree = &free
		s.DiskTotal = &total
	}
	b, _ := os.ReadFile("/proc/stat")
	lines := strings.SplitN(string(b), "\n", 2)
	f := strings.Fields(lines[0])
	if len(f) >= 9 {
		for _, x := range f[1:9] {
			s.CPU = append(s.CPU, num(x))
		}
	}
	if prev == nil {
		return s
	}
	secs := s.At.Sub(prev.At).Seconds()
	if secs <= 0 {
		return s
	}
	if s.Hub.CPUTicks >= prev.Hub.CPUTicks {
		v := float64(s.Hub.CPUTicks-prev.Hub.CPUTicks) / clockTicks() / secs * 100
		s.Hub.CPUPercent = &v
	}
	if s.Generator.CPUTicks >= prev.Generator.CPUTicks {
		v := float64(s.Generator.CPUTicks-prev.Generator.CPUTicks) / clockTicks() / secs * 100
		s.Generator.CPUPercent = &v
	}
	s.StealPercent, s.IOWaitPercent = systemCPUPercent(s.CPU, prev.CPU)
	if s.Disk != nil && prev.Disk != nil && s.Disk.Device == prev.Disk.Device {
		a, b := s.Disk, prev.Disk
		if a.Writes > b.Writes && a.WriteMS >= b.WriteMS {
			v := float64(a.WriteMS-b.WriteMS) / float64(a.Writes-b.Writes)
			a.WriteAwaitMS = &v
		}
		if a.Flushes > b.Flushes && a.FlushMS >= b.FlushMS {
			v := float64(a.FlushMS-b.FlushMS) / float64(a.Flushes-b.Flushes)
			a.FlushAwaitMS = &v
		}
		if a.BusyMS >= b.BusyMS {
			v := float64(a.BusyMS-b.BusyMS) / 1000 / secs * 100
			a.UtilPercent = &v
		}
	}
	return s
}

// Keep independent values: two pointers to the same reused local variable
// would turn steal into iowait in the encoded sample. Counter resets are unknown.
func systemCPUPercent(cur, prev []uint64) (steal, wait *float64) {
	if len(cur) != 8 || len(prev) != 8 {
		return nil, nil
	}
	var total uint64
	for i, n := range cur {
		if n < prev[i] {
			return nil, nil
		}
		total += n - prev[i]
	}
	if total == 0 {
		return nil, nil
	}
	sv := float64(cur[7]-prev[7]) / float64(total) * 100
	wv := float64(cur[4]-prev[4]) / float64(total) * 100
	return &sv, &wv
}
