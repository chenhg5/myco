package processwatch

import (
	"context"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

func birth(f windows.Filetime) string {
	return fmt.Sprintf("%020d", uint64(f.HighDateTime)<<32|uint64(f.LowDateTime))
}
func seconds(f windows.Filetime) float64 {
	return float64(uint64(f.HighDateTime)<<32|uint64(f.LowDateTime)) / 1e7
}
func readProcess(pid int) (Process, error) {
	h, e := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if e != nil {
		return Process{}, e
	}
	defer windows.CloseHandle(h)
	var c, x, k, u windows.Filetime
	if e = windows.GetProcessTimes(h, &c, &x, &k, &u); e != nil {
		return Process{}, e
	}
	return Process{PID: pid, Birth: birth(c), CPUTime: seconds(k) + seconds(u)}, nil
}
func readProcesses(ctx context.Context) ([]Process, error) {
	snap, e := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if e != nil {
		return nil, e
	}
	defer windows.CloseHandle(snap)
	// Token SID comparison prevents publishing another OS user's process names.
	own, e := windows.OpenCurrentProcessToken()
	if e != nil {
		return nil, e
	}
	defer own.Close()
	user, e := own.GetTokenUser()
	if e != nil {
		return nil, e
	}
	var out []Process
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for e = windows.Process32First(snap, &entry); e == nil; e = windows.Process32Next(snap, &entry) {
		if e = ctx.Err(); e != nil {
			return nil, e
		}
		p, err := readProcess(int(entry.ProcessID))
		if err != nil {
			continue
		}
		p.Parent = int(entry.ParentProcessID)
		p.Name = windows.UTF16ToString(entry.ExeFile[:])
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, entry.ProcessID)
		if err == nil {
			var token windows.Token
			if windows.OpenProcessToken(h, windows.TOKEN_QUERY, &token) == nil {
				if other, err := token.GetTokenUser(); err == nil {
					p.User = user.User.Sid.Equals(other.User.Sid)
				}
				token.Close()
			}
			windows.CloseHandle(h)
		}
		out = append(out, p)
	}
	if e != windows.ERROR_NO_MORE_FILES {
		return nil, e
	}
	return out, nil
}
