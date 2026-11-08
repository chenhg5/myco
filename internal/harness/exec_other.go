//go:build !unix && !windows

package harness

import (
	"os/exec"
	"time"
)

func setProcessGroup(*exec.Cmd) {}

// interruptGrace matches the unix value (the process is killed directly here).
var interruptGrace = 5 * time.Second
