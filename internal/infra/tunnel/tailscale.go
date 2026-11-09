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
	// stdout only: the CLI may warn on stderr (e.g. a client/daemon version
	// mismatch), which must not spoil the JSON.
	out, errOut, _ := runSplit(ctx, bin, "status", "--json")
	state, dns := tailscaleStatus(out)
	if state == "" {
		msg := strings.TrimSpace(errOut + "\n" + out)
		if len(msg) > 400 {
			msg = msg[:400] + "…"
		}
		return "", &Error{Msg: "cannot read Tailscale's status (" + bin + "): " + msg, Fix: "start Tailscale and log in: tailscale up (or open the Tailscale app)"}
	}
	if state != "Running" || dns == "" {
		return "", &Error{Msg: "Tailscale is not connected (state " + state + ")", Fix: "log in: tailscale up (or open the Tailscale app)"}
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
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	out, err := run(ctx, t.bin, "funnel", "--bg", u.Port())
	if err == nil {
		return nil
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
