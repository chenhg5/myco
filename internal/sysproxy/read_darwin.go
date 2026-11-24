//go:build darwin

package sysproxy

import (
	"context"
	"os/exec"
	"time"
)

func readSystem() Setting {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "scutil", "--proxy").Output()
	if err != nil {
		return Setting{}
	}
	return parseScutil(string(out))
}
