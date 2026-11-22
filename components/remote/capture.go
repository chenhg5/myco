package main

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/pion/webrtc/v4"
)

func ffmpegPath() string {
	exe, _ := os.Executable()
	name := "ffmpeg"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(filepath.Dir(exe), name)
}

// screenInput is FFmpeg's input for the display remote control maps to (the
// main display on a Mac, the virtual desktop on Windows) at fps frames a second.
func screenInput(ctx context.Context, bin, fps string) ([]string, error) {
	switch runtime.GOOS {
	case "darwin":
		// AVFoundation enumerates cameras before displays, so screen 0 is not
		// necessarily device 0. Never guess: that could publish the webcam.
		listCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		c := exec.CommandContext(listCtx, bin, "-hide_banner", "-f", "avfoundation", "-list_devices", "true", "-i", "")
		hideProcess(c)
		var out bytes.Buffer
		c.Stderr = &out
		_ = c.Run()
		device := screenDevice(out.String())
		if device == "" {
			return nil, errors.New("no_screen")
		}
		return []string{"-f", "avfoundation", "-framerate", fps, "-capture_cursor", "1", "-i", device + ":none"}, nil
	case "windows":
		return []string{"-f", "gdigrab", "-framerate", fps, "-draw_mouse", "1", "-i", "desktop"}, nil
	}
	return nil, errors.New("unsupported_platform")
}
func captureArgs(ctx context.Context, bin string) ([]string, error) {
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin"}
	in, err := screenInput(ctx, bin, "30")
	if err != nil {
		return nil, err
	}
	args = append(args, in...)
	// Stay within the negotiated H.264 baseline level 3.1 (720p30), including
	// on older WKWebView and WebView2 hardware decoders.
	args = append(args, "-an", "-vf", "scale=1280:720:force_original_aspect_ratio=decrease:force_divisible_by=2", "-r", "30")
	args = append(args, h264EncoderArgs()...)
	return append(args, "-b:v", "4000k", "-g", "30", "-bf", "0", "-pix_fmt", "nv12", "-flags", "-global_header", "-bsf:v", "h264_mp4toannexb", "-payload_type", "96", "-f", "rtp"), nil
}

// Media Foundation does not register the named profile constants used by
// VideoToolbox. 66 is AV_PROFILE_H264_BASELINE in the pinned FFmpeg source.
func h264EncoderArgs() []string {
	if runtime.GOOS == "windows" {
		return []string{"-c:v", "h264_mf", "-profile:v", "66"}
	}
	return []string{"-c:v", "h264_videotoolbox", "-realtime", "1", "-allow_sw", "1", "-profile:v", "baseline"}
}

func capture(ctx context.Context, track *webrtc.TrackLocalStaticRTP) error {
	bin := ffmpegPath()
	if _, err := os.Stat(bin); err != nil {
		return errors.New("encoder_missing")
	}
	args, err := captureArgs(ctx, bin)
	if err != nil {
		return err
	}
	return captureCommand(ctx, track, bin, args)
}
func captureCommand(ctx context.Context, track *webrtc.TrackLocalStaticRTP, bin string, args []string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		return err
	}
	defer conn.Close()
	port := conn.LocalAddr().(*net.UDPAddr).Port
	args = append(args, "rtp://127.0.0.1:"+strconv.Itoa(port)+"?pkt_size=1200")
	cmd := exec.CommandContext(ctx, bin, args...)
	hideProcess(cmd)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err = cmd.Start(); err != nil {
		return errors.New("encoder_start_failed")
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait(); cancel(); _ = conn.Close() }()
	buf := make([]byte, 2048)
	for {
		_ = conn.SetReadDeadline(time.Now().Add(12 * time.Second))
		n, _, e := conn.ReadFromUDP(buf)
		if e != nil {
			cancel()
			<-done
			if ctx.Err() != nil && strings.Contains(e.Error(), "closed") {
				return errors.New("screen_capture_failed")
			}
			return e
		}
		if _, e = track.Write(buf[:n]); e != nil {
			cancel()
			<-done
			return e
		}
	}
}

func screenDevice(output string) string {
	parts := strings.SplitN(output, "AVFoundation video devices:", 2)
	if len(parts) != 2 {
		return ""
	}
	video := strings.SplitN(parts[1], "AVFoundation audio devices:", 2)[0]
	matches := regexp.MustCompile(`\[(\d+)\] Capture screen 0`).FindAllStringSubmatch(video, -1)
	if len(matches) == 0 {
		return ""
	}
	return matches[len(matches)-1][1]
}
