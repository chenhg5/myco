// Package tunnel exposes a hub running on a personal machine (no VPS) at a
// public HTTPS URL through Cloudflare Tunnel or Tailscale Funnel. The hub
// listens on localhost; the tunnel's own client (cloudflared, tailscaled)
// carries the traffic and terminates TLS.
package tunnel

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"time"
)

// Tunnel publishes a local HTTP address.
type Tunnel interface {
	Name() string
	// Prepare checks prerequisites and does idempotent one-time setup. It
	// returns the public URL when it is known in advance ("" when it is
	// only learned once the tunnel runs, e.g. a Cloudflare quick tunnel).
	Prepare(ctx context.Context, local string) (publicURL string, err error)
	// Run keeps the tunnel up until ctx ends, calling onURL whenever the
	// public URL is (re)learned.
	Run(ctx context.Context, local string, onURL func(string)) error
}

// Options selects and configures a tunnel.
type Options struct {
	// Kind is "cloudflare" or "tailscale".
	Kind string
	// Hostname (Cloudflare): a hostname in a zone on your Cloudflare
	// account, e.g. hub.example.com. Empty = a quick tunnel with a random
	// trycloudflare.com URL that changes on every restart.
	Hostname string
	// Name (Cloudflare named tunnels) defaults to "myco".
	Name string
	// StateDir holds generated config (cloudflared.yml).
	StateDir string
	Log      *slog.Logger
}

// Kinds lists the supported tunnels.
var Kinds = []string{"cloudflare", "tailscale"}

// New builds the tunnel named by o.Kind.
func New(o Options) (Tunnel, error) {
	if o.Log == nil {
		o.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	switch o.Kind {
	case "cloudflare":
		if o.Name == "" {
			o.Name = "myco"
		}
		return &Cloudflare{o: o}, nil
	case "tailscale":
		return &Tailscale{o: o}, nil
	}
	return nil, fmt.Errorf("unknown tunnel %q (use %s)", o.Kind, strings.Join(Kinds, " or "))
}

// Error is a prerequisite problem with a suggested fix for the user.
type Error struct {
	Msg, Fix string
}

func (e *Error) Error() string { return e.Msg }

// run executes a command and returns its combined output.
func run(ctx context.Context, name string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// runSplit runs a command and returns its stdout and stderr apart.
func runSplit(ctx context.Context, name string, args ...string) (stdout, stderr string, err error) {
	var o, e strings.Builder
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout, cmd.Stderr = &o, &e
	err = cmd.Run()
	return o.String(), e.String(), err
}

// supervise runs a long-lived child until ctx ends, restarting it with
// backoff when it exits. Each output line is passed to onLine.
func supervise(ctx context.Context, log *slog.Logger, name string, args []string, onLine func(string)) error {
	backoff := time.Second
	for ctx.Err() == nil {
		start := time.Now()
		cmd := exec.CommandContext(ctx, name, args...)
		pr, pw := io.Pipe()
		cmd.Stdout, cmd.Stderr = pw, pw
		go func() {
			sc := bufio.NewScanner(pr)
			for sc.Scan() {
				if onLine != nil {
					onLine(sc.Text())
				}
				log.Debug(name, "line", sc.Text())
			}
		}()
		err := cmd.Run()
		pw.Close()
		if ctx.Err() != nil {
			return nil
		}
		if time.Since(start) > time.Minute {
			backoff = time.Second
		}
		log.Warn("tunnel client exited; restarting", "client", name, "err", errString(err), "retry_in", backoff)
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return nil
		}
		backoff = min(backoff*2, time.Minute)
	}
	return nil
}

func errString(err error) string {
	if err == nil {
		return "exited"
	}
	return err.Error()
}

// lookPath finds a CLI, also trying well-known install locations that a
// service's minimal PATH may miss.
func lookPath(name string, extra ...string) (string, error) {
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	for _, p := range extra {
		if _, err := exec.LookPath(p); err == nil {
			return p, nil
		}
	}
	return "", errors.New(name + " not found")
}
