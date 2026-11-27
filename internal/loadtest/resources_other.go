//go:build !linux

package loadtest

import "time"

func readResources(pid int, dir string, prev *resourceSample) resourceSample {
	return resourceSample{At: time.Now().UTC()}
}
