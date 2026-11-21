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
func captureArgs(ctx context.Context, bin string) ([]string, error) {
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin"}
	switch runtime.GOOS {
	case "darwin":
		// AVFoundation enumerates cameras before displays, so screen 0 is not
		// necessarily device 0. Never guess: that could publish the webcam.
		listCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		c := exec.CommandContext(listCtx, bin, "-hide_banner", "-f", "avfoundation", "-list_devices", "true", "-i", "")
		var out bytes.Buffer
		c.Stderr = &out
		_ = c.Run()
		device := screenDevice(out.String())
		if device == "" {
			return nil, errors.New("no_screen")
		}
		args = append(args, "-f", "avfoundation", "-framerate", "30", "-capture_cursor", "1", "-i", device+":none")
	case "windows":
		args = append(args, "-f", "gdigrab", "-framerate", "30", "-draw_mouse", "1", "-i", "desktop")
	default:
		return nil, errors.New("unsupported_platform")
	}
	// Keep H.264 baseline for hardware decoding in WKWebView and WebView2.
	args = append(args, "-an", "-vf", "scale=1920:1080:force_original_aspect_ratio=decrease:force_divisible_by=2", "-r", "30")
	if runtime.GOOS == "darwin" {
		args = append(args, "-c:v", "h264_videotoolbox", "-realtime", "1", "-allow_sw", "1")
	} else {
		args = append(args, "-c:v", "h264_mf")
	}
	return append(args, "-profile:v", "baseline", "-b:v", "4000k", "-g", "30", "-bf", "0", "-pix_fmt", "nv12", "-flags", "-global_header", "-bsf:v", "h264_mp4toannexb", "-payload_type", "96", "-f", "rtp"), nil
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
