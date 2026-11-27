//go:build !linux && !darwin && !windows

package processwatch

import (
	"context"
	"errors"
	"time"
)

func readProcess(int) (Process, error) { return Process{}, errors.New("process identity unsupported") }
func readProcesses(context.Context) ([]Process, error) {
	return nil, errors.New("process observation unsupported")
}
func terminate(context.Context, Process, time.Duration) error {
	return errors.New("process termination unsupported")
}
