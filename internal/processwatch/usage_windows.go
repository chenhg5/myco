package processwatch

import (
	"context"
	"errors"
	"golang.org/x/sys/windows"
	"time"
	"unsafe"
)

var healthKernel = windows.NewLazySystemDLL("kernel32.dll")
var healthTimes = healthKernel.NewProc("GetSystemTimes")
var healthMemory = healthKernel.NewProc("GlobalMemoryStatusEx")

func filetimeValue(f windows.Filetime) uint64 {
	return uint64(f.HighDateTime)<<32 | uint64(f.LowDateTime)
}
func readUsage(ctx context.Context) (usageSnapshot, error) {
	counters := func() (uint64, uint64, error) {
		var idle, kernel, user windows.Filetime
		r, _, e := healthTimes.Call(uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user)))
		if r == 0 {
			return 0, 0, e
		}
		return filetimeValue(idle), filetimeValue(kernel) + filetimeValue(user), nil
	}
	i1, t1, e := counters()
	if e != nil {
		return usageSnapshot{}, e
	}
	timer := time.NewTimer(300 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return usageSnapshot{}, ctx.Err()
	case <-timer.C:
	}
	i2, t2, e := counters()
	if e != nil {
		return usageSnapshot{}, e
	}
	if t2 <= t1 || i2 < i1 || i2-i1 > t2-t1 {
		return usageSnapshot{}, errors.New("CPU counters unavailable")
	}
	var memory struct {
		Length, Load                                                                                         uint32
		TotalPhys, AvailPhys, TotalPageFile, AvailPageFile, TotalVirtual, AvailVirtual, AvailExtendedVirtual uint64
	}
	memory.Length = uint32(unsafe.Sizeof(memory))
	r, _, e := healthMemory.Call(uintptr(unsafe.Pointer(&memory)))
	if r == 0 {
		return usageSnapshot{}, e
	}
	return usageSnapshot{CPU: 100 * (1 - float64(i2-i1)/float64(t2-t1)), Total: memory.TotalPhys, Available: memory.AvailPhys}, nil
}
