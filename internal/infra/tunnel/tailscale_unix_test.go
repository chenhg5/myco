//go:build !windows

package tunnel

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeTailscale puts a tailscale on PATH that answers status with json and
// runs funnel as given.
func fakeTailscale(t *testing.T, status, funnel string) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\ncase \"$1\" in\nstatus) " + status + " ;;\nfunnel) " + funnel + " ;;\nesac\n"
	if err := os.WriteFile(filepath.Join(dir, "tailscale"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

const running = `echo '{"BackendState":"Running","Self":{"DNSName":"mac.tail1.ts.net."}}'`

// Funnel not enabled: the CLI prints where to enable it and waits; Prepare
// stops at once and hands the link on.
func TestTailscaleFunnelNotEnabledStopsAtOnce(t *testing.T) {
	fakeTailscale(t, running, `echo "Funnel is not enabled on your tailnet."; echo "To enable, visit:"; echo; echo "         https://login.tailscale.com/f/funnel?node=nABC"; sleep 30`)
	start := time.Now()
	_, err := (&Tailscale{}).Prepare(context.Background(), "http://127.0.0.1:8080")
	var te *Error
	if !errors.As(err, &te) || !strings.Contains(te.Fix, "https://login.tailscale.com/f/funnel?node=nABC") {
		t.Fatalf("err %v", err)
	}
	if time.Since(start) > 10*time.Second {
		t.Fatalf("waited %v for a Funnel that is not enabled", time.Since(start))
	}
}

// A status that never answers gives up after a while instead of hanging.
func TestTailscaleStatusTimesOut(t *testing.T) {
	statusTimeout = 300 * time.Millisecond
	t.Cleanup(func() { statusTimeout = 15 * time.Second })
	fakeTailscale(t, "sleep 30", "true")
	start := time.Now()
	_, err := (&Tailscale{}).Prepare(context.Background(), "http://127.0.0.1:8080")
	var te *Error
	if !errors.As(err, &te) || !strings.Contains(te.Msg, "did not answer") {
		t.Fatalf("err %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("waited %v", time.Since(start))
	}
}

func TestTailscaleFunnelOK(t *testing.T) {
	fakeTailscale(t, running, `echo "Available on the internet: https://mac.tail1.ts.net/"`)
	u, err := (&Tailscale{}).Prepare(context.Background(), "http://127.0.0.1:8080")
	if err != nil || u != "https://mac.tail1.ts.net" {
		t.Fatalf("%q %v", u, err)
	}
}
