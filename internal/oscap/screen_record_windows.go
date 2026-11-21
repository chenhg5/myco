package oscap

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Use a separately installed encoder. Do not bundle it with the App or start
// remote desktop just to obtain a screen recording.
func screenRecordMissing(env *Env) string {
	if env == nil {
		return "screen recording needs FFmpeg on this computer"
	}
	if _, err := env.LookPath("ffmpeg"); err != nil {
		return "screen recording needs FFmpeg on this computer, or a browser screen picker"
	}
	return ""
}
func recordScreen(ctx context.Context, env *Env, a screenRecordArgs, path string, emit Emit) error {
	if a.Microphone || a.Interactive || a.Region != nil {
		return errf(CodeUnsupported, "Windows records the desktop only; microphone and region selection are not supported yet")
	}
	exe, err := env.LookPath("ffmpeg")
	if err != nil {
		return errf(CodeUnsupported, "install FFmpeg on this computer before recording")
	}
	cmd := exec.Command(exe, "-hide_banner", "-loglevel", "error", "-nostats", "-f", "gdigrab", "-framerate", "30", "-draw_mouse", "1", "-i", "desktop", "-t", strconv.Itoa(a.MaxSeconds), "-c:v", "h264_mf", "-pix_fmt", "yuv420p", "-b:v", "6M", "-movflags", "+faststart", path)
	hideWindow(cmd)
	cmd.Env = env.Environ(nil)
	// FFmpeg accepts q on stdin and flushes the movie before exiting. Sending
	// an OS interrupt is not portable to a Windows GUI process.
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err = cmd.Start(); err != nil {
		return errf(CodeFailed, "could not start screen recording: %v", err)
	}
	defer stdin.Close()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	emit("started", map[string]any{"path": path})
	stop := func() error {
		_, _ = stdin.Write([]byte("q\n"))
		select {
		case err := <-done:
			if err != nil {
				return errf(CodeFailed, "FFmpeg could not finalize the movie; verify H.264 Media Foundation support")
			}
			return nil
		case <-time.After(8 * time.Second):
			_ = cmd.Process.Kill()
			<-done
			return errf(CodeFailed, "recording did not stop; check the recording folder")
		}
	}
	for {
		select {
		case err := <-done:
			if err != nil {
				return errf(CodeFailed, "FFmpeg could not record the desktop; verify gdigrab and H.264 Media Foundation support")
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
