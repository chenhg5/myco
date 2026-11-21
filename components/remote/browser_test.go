package main

import (
	"context"
	"flag"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/pion/webrtc/v4"
)

var browserSmoke = flag.Bool("browser-smoke", false, "run the opt-in browser/FFmpeg decoding check (needs Node, Playwright and Chromium)")

func TestBrowserDecodesVideo(t *testing.T) {
	if !*browserSmoke {
		t.Skip("opt in with -args -browser-smoke; ordinary unit tests need no browser or encoder")
	}
	bin, e := exec.LookPath("ffmpeg")
	if e != nil {
		t.Fatal(e)
	}
	engine := &engine{cfg: settings{Secret: strings.Repeat("s", 64), PairingCode: "abcdef12"}, peers: map[string]*peer{}, start: func(ctx context.Context, tr *webrtc.TrackLocalStaticRTP) error {
		return captureCommand(ctx, tr, bin, []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-re", "-f", "lavfi", "-i", "testsrc=size=640x360:rate=30", "-an", "-c:v", "libx264", "-profile:v", "baseline", "-pix_fmt", "yuv420p", "-tune", "zerolatency", "-g", "30", "-bf", "0", "-flags", "-global_header", "-payload_type", "96", "-f", "rtp"})
	}}
	defer engine.shutdown()
	server := httptest.NewServer(engine.handler())
	defer server.Close()
	cmd := exec.Command("node", "scripts/browser-smoke.mjs")
	cmd.Env = append(os.Environ(), "REMOTE_TEST_RPC_URL="+server.URL, "REMOTE_TEST_RPC_TOKEN="+engine.cfg.Secret)
	out, e := cmd.CombinedOutput()
	if e != nil {
		t.Fatalf("browser decoding failed: %v\n%s", e, out)
	}
	t.Log(string(out))
}
