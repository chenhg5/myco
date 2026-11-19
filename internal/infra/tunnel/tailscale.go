package tunnel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Tailscale publishes the hub with Tailscale Funnel at
// https://<machine>.<tailnet>.ts.net (stable). Funnel is configured in the
// background (`tailscale funnel --bg`), so tailscaled keeps it up; Run only
// re-applies it periodically in case it was turned off.
type Tailscale struct {
	o   Options
	bin string
}

func (t *Tailscale) Name() string { return "tailscale" }

func (t *Tailscale) Prepare(ctx context.Context, local string) (string, error) {
	bin, err := lookPath("tailscale", "/Applications/Tailscale.app/Contents/MacOS/Tailscale")
	if err != nil {
		return "", &Error{Msg: "the tailscale CLI is not installed",
			Fix: "install Tailscale (https://tailscale.com/download) and log in"}
	}
	t.bin = bin
	// The Mac app's own binary, used as a command line, can hang when asked
	// from a background process: never wait long for it.
	appCLI := strings.Contains(bin, "Tailscale.app/")
	cliFix := "open the Tailscale app and connect"
	if appCLI {
		cliFix = "install Tailscale's command line (Tailscale menu > Settings > Install CLI), open the Tailscale app and connect, then try again"
	}
	sctx, cancel := context.WithTimeout(ctx, statusTimeout)
	defer cancel()
	// stdout only: the CLI may warn on stderr (e.g. a client/daemon version
	// mismatch), which must not spoil the JSON.
	out, errOut, _ := runSplit(sctx, bin, "status", "--json")
	if sctx.Err() != nil && ctx.Err() == nil {
		return "", &Error{Msg: "Tailscale did not answer within " + statusTimeout.String() + " (" + bin + " status)", Fix: cliFix}
	}
	state, dns := tailscaleStatus(out)
	if state == "" && strings.Contains(errOut+out, "GUI failed to start") {
		// The Mac app runs for one macOS user at a time.
		return "", &Error{Msg: "the Tailscale app is not running for this macOS user",
			Fix: "open the Tailscale app as this user and log in (on a Mac, only the user running the Tailscale app can use it)"}
	}
	if state == "" {
		msg := strings.TrimSpace(errOut + "\n" + out)
		if len(msg) > 400 {
			msg = msg[:400] + "…"
		}
		return "", &Error{Msg: "cannot read Tailscale's status (" + bin + "): " + msg, Fix: cliFix}
	}
	if state != "Running" || dns == "" {
		// The app may show connected while its binary, asked from here,
		// says otherwise.
		return "", &Error{Msg: "Tailscale is not connected here (state " + state + ")", Fix: cliFix}
	}
	if err := t.funnel(ctx, local); err != nil {
		return "", err
	}
	return "https://" + dns, nil
}

func (t *Tailscale) funnel(ctx context.Context, local string) error {
	u, err := url.Parse(local)
	if err != nil || u.Port() == "" {
		return fmt.Errorf("cannot publish %q: need a local URL with a port", local)
	}
	ctx, cancel := context.WithTimeout(ctx, funnelTimeout)
	defer cancel()
	// When Funnel (or HTTPS) is not enabled on the tailnet, the CLI prints a
	// link to enable it and waits: stop at once and hand the link on.
	var enable string
	out, err := runUntil(ctx, t.bin, []string{"funnel", "--bg", u.Port()}, func(line string) bool {
		if i := strings.Index(line, "https://login.tailscale.com/f/"); i >= 0 {
			enable = strings.Fields(line[i:])[0]
			return true
		}
		return false
	})
	if enable != "" {
		return &Error{Msg: "Funnel is not enabled on this tailnet yet",
			Fix: "open " + enable + " to enable it (and HTTPS certificates if asked), then try again"}
	}
	if err == nil {
		return nil
	}
	if ctx.Err() == context.DeadlineExceeded {
		return &Error{Msg: "Tailscale Funnel did not answer within " + funnelTimeout.String() + ": " + strings.TrimSpace(lastOf(out, 300)),
			Fix: "check that Funnel is allowed for this device in the Tailscale admin console, then try again"}
	}
	low := strings.ToLower(out)
	switch {
	case strings.Contains(low, "access denied") || strings.Contains(low, "operator"):
		return &Error{Msg: "Tailscale refused to change Funnel settings for this user: " + strings.TrimSpace(out),
			Fix: "allow your user once: sudo tailscale set --operator=$USER"}
	case strings.Contains(low, "funnel") && (strings.Contains(low, "not enabled") || strings.Contains(low, "https://login.tailscale.com")):
		return &Error{Msg: "Funnel is not enabled for this device: " + strings.TrimSpace(out),
			Fix: "open the link above to enable Funnel (and HTTPS certificates) in the Tailscale admin console, then run this again"}
	}
	return err
}

func (t *Tailscale) Run(ctx context.Context, local string, _ func(string)) error {
	tick := time.NewTicker(10 * time.Minute)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
			if err := t.funnel(ctx, local); err != nil && ctx.Err() == nil {
				t.o.Log.Warn("re-applying Tailscale Funnel failed", "err", err)
			}
		}
	}
}

// How long to wait for the Tailscale CLI (tests shorten them).
var (
	statusTimeout = 15 * time.Second
	funnelTimeout = 90 * time.Second
)

func lastOf(s string, n int) string {
	if len(s) > n {
		return "…" + s[len(s)-n:]
	}
	return s
}

// tailscaleStatus extracts the backend state and this device's DNS name
// (without the trailing dot) from `tailscale status --json`.
func tailscaleStatus(out string) (state, dns string) {
	var st struct {
		BackendState string
		Self         struct{ DNSName string }
	}
	// Tolerate text around the JSON object.
	if i, j := strings.Index(out, "{"), strings.LastIndex(out, "}"); i >= 0 && j > i {
		out = out[i : j+1]
	}
	if json.Unmarshal([]byte(out), &st) != nil {
		return "", ""
	}
	return st.BackendState, strings.TrimSuffix(st.Self.DNSName, ".")
}
