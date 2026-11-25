//go:build linux || darwin

package diskspace

import "golang.org/x/sys/unix"

func Read(path string) (Space, error) {
	var s unix.Statfs_t
	err := unix.Statfs(path, &s)
	return Space{Available: uint64(s.Bavail) * uint64(s.Bsize), Total: uint64(s.Blocks) * uint64(s.Bsize)}, err
}
