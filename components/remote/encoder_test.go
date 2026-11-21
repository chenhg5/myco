package main

import (
	"bytes"
	"context"
	"flag"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/pion/rtp"
)

var encoderSmoke = flag.String("encoder-smoke", "", "test the packaged native FFmpeg encoder using synthetic NV12 input")

func TestPackagedEncoderSendsParameterSets(t *testing.T) {
	if *encoderSmoke == "" {
		t.Skip("opt in with -args -encoder-smoke <binary>")
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("native platform encoder")
	}
	bin, e := filepath.Abs(*encoderSmoke)
	if e != nil {
		t.Fatal(e)
	}
	// Test pixels only, never a screenshot or camera feed.
	frame := append(bytes.Repeat([]byte{128}, 640*360), bytes.Repeat([]byte{64, 192}, 640*360/4)...)
	input := filepath.Join(t.TempDir(), "test.nv12")
	if e = os.WriteFile(input, bytes.Repeat(frame, 3), 0600); e != nil {
		t.Fatal(e)
	}
	conn, e := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if e != nil {
		t.Fatal(e)
	}
	defer conn.Close()
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-f", "rawvideo", "-pixel_format", "nv12", "-video_size", "640x360", "-framerate", "30", "-i", input, "-an", "-vf", "scale=640:360", "-frames:v", "3"}
	if runtime.GOOS == "darwin" {
		args = append(args, "-c:v", "h264_videotoolbox", "-allow_sw", "1", "-realtime", "1")
	} else {
		args = append(args, "-c:v", "h264_mf")
	}
	args = append(args, "-profile:v", "baseline", "-b:v", "1000k", "-bf", "0", "-g", "30", "-flags", "-global_header", "-bsf:v", "h264_mp4toannexb", "-payload_type", "96", "-f", "rtp", "rtp://127.0.0.1:"+strconv.Itoa(conn.LocalAddr().(*net.UDPAddr).Port)+"?pkt_size=1200")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if out, e := exec.CommandContext(ctx, bin, args...).CombinedOutput(); e != nil {
		t.Fatalf("packaged platform encoder failed: %v\n%s", e, out)
	}
	seen := map[byte]bool{}
	observe := func(data []byte) {
		if len(data) > 0 {
			seen[data[0]&31] = true
		}
	}
	for {
		_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		buf := make([]byte, 2048)
		n, _, e := conn.ReadFromUDP(buf)
		if e != nil {
			break
		}
		var p rtp.Packet
		if p.Unmarshal(buf[:n]) != nil || len(p.Payload) == 0 {
			continue
		}
		switch p.Payload[0] & 31 {
		case 24:
			data := p.Payload[1:]
			for len(data) >= 2 {
				size := int(data[0])<<8 | int(data[1])
				data = data[2:]
				if size > len(data) {
					break
				}
				observe(data[:size])
				data = data[size:]
			}
		case 28:
			if len(p.Payload) > 1 {
				seen[p.Payload[1]&31] = true
			}
		default:
			observe(p.Payload)
		}
	}
	if !seen[7] || !seen[8] || !seen[5] {
		t.Fatalf("a browser needs in-band SPS, PPS and IDR, got NAL types %v", seen)
	}
}
