//go:build !windows

package harness

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSilentStartStopsAProcessThatPrintsNothing(t *testing.T) {
	_, err := Exec(context.Background(), Process{Binary: "sh", Args: []string{"-c", "echo 'stream error: retrying 1/5' >&2; sleep 5"},
		SilentStart: 300 * time.Millisecond})
	if !errors.Is(err, ErrSilentStart) || !strings.Contains(err.Error(), "retrying 1/5") {
		t.Fatalf("silent: %v", err)
	}
	// Something on stdout in time: it runs to the end.
	out, err := Exec(context.Background(), Process{Binary: "sh", Args: []string{"-c", "echo hi; sleep 0.5; echo done"},
		SilentStart: 300 * time.Millisecond})
	if err != nil || !strings.Contains(out.Stdout, "done") {
		t.Fatalf("talking: %v %q", err, out.Stdout)
	}
	// The caller's own stop is not called silence.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := Exec(ctx, Process{Binary: "sh", Args: []string{"-c", "sleep 5"}, SilentStart: time.Second}); errors.Is(err, ErrSilentStart) {
		t.Fatalf("parent stop: %v", err)
	}
}
