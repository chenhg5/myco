package oscap

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func screenRecordMissing(*Env) string { return "" }
func screenRecordCommand(a screenRecordArgs, path string) []string {
	args := []string{"-v", "-V", strconv.Itoa(a.MaxSeconds)}
	if a.Interactive {
		args = append(args, "-i", "-Jvideo")
	} else {
		args = append(args, "-m")
	}
	if a.Microphone {
		args = append(args, "-g")
	}
	if a.Clicks {
		args = append(args, "-k")
	}
	return append(args, path)
}
func recordScreen(ctx context.Context, _ *Env, a screenRecordArgs, path string, emit Emit) error {
	cmd := exec.Command("/usr/sbin/screencapture", screenRecordCommand(a, path)...)
	// Do not collect screen contents or command output in logs.
	if err := cmd.Start(); err != nil {
		return errf(CodeFailed, "could not open macOS screen recording: %v", err)
	}
	emit("started", map[string]any{"path": path, "selecting": a.Interactive})
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	stop := func() error {
		_ = cmd.Process.Signal(os.Interrupt)
		select {
		case <-done:
			return nil
		case <-time.After(8 * time.Second):
			_ = cmd.Process.Kill()
			<-done
			return errf(CodeFailed, "screen recording did not finish; check the recording folder")
		}
	}
	for {
		select {
		case err := <-done:
			if err != nil {
				if fi, e := os.Stat(path); e != nil || fi.Size() < 32 {
					return errf(CodeCancelled, "recording was cancelled; allow Screen Recording in System Settings if needed")
				}
			}
			return nil
		case <-ctx.Done():
			return stop()
		case b, ok := <-inputOf(ctx):
			if !ok || strings.TrimSpace(string(b)) == "stop" {
				return stop()
			}
		}
	}
}
