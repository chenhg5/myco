package oscap

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func screenRecordMissing(*Env) string { return "" }
func screenRecordCommand(a screenRecordArgs, path string) []string {
	args := []string{"-V", strconv.Itoa(a.MaxSeconds)}
	// -i selects interactive *still image* capture and is rejected with -v.
	// -Jvideo selects the system's interactive video capture instead.
	if a.Interactive {
		args = append(args, "-Jvideo")
	} else if a.Region != nil {
		r := a.Region
		args = append(args, "-v", "-R", fmt.Sprintf("%d,%d,%d,%d", r.X, r.Y, r.W, r.H))
	} else {
		args = append(args, "-v", "-m")
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
	if err := requestScreenRecordingPermission(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	cmd := exec.Command("/usr/sbin/screencapture", screenRecordCommand(a, path)...)
	// Keep bounded diagnostic text, never screen contents.
	diagnostic := &capped{n: 4096}
	cmd.Stderr = diagnostic
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
					return screenCaptureFailure(err, diagnostic.buf.String(), path)
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
