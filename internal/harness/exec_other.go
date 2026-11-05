//go:build !unix

package harness

import "os/exec"

func setProcessGroup(*exec.Cmd) {}
