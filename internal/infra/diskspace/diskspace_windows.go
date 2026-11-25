package diskspace

import "golang.org/x/sys/windows"

func Read(path string) (Space, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return Space{}, err
	}
	var s Space
	var free uint64
	err = windows.GetDiskFreeSpaceEx(p, &s.Available, &s.Total, &free)
	return s, err
}
