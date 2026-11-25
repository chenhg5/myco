//go:build !linux && !darwin && !windows

package diskspace

import "fmt"

func Read(path string) (Space, error) {
	return Space{}, fmt.Errorf("disk space checks are unsupported on this platform")
}
