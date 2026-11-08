package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/api"
	. "github.com/chenhg5/myco/internal/cli/cmdkit"
	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/config"
	"github.com/chenhg5/myco/internal/infra/tunnel"
	"github.com/chenhg5/myco/internal/service"
)

// hubDir is where `myco hub up` keeps a personal hub: hub.env (admin token,
// mode 600), hub.db, files/, hub.log, cloudflared.yml.
func (e *env) hubDir() string { return filepath.Join(config.Dir(e.defaultPath), "hub") }

func hubUpCommand() *Command {
	return &Command{
		Name:     "up",
		Summary:  "Run a hub on this computer (no VPS needed), published through Cloudflare or Tailscale",
		Mutating: true,
		Long: `Installs the hub as a background service on this machine (launchd on macOS, systemd --user on
Linux) and publishes it at a public HTTPS URL through a tunnel, then prints a join link. Re-running
is safe (it updates the service; the admin token and data stay in ~/.myco/hub).

  --tunnel cloudflare --hostname hub.example.com   stable URL on your domain (free Cloudflare account,
                                                   domain on Cloudflare; run "cloudflared tunnel login" once)
  --tunnel cloudflare                              temporary https://<random>.trycloudflare.com URL, no
                                                   account; it CHANGES whenever the tunnel restarts, which
                                                   breaks links and joined machines: for trying it out
  --tunnel tailscale                               stable https://<machine>.<tailnet>.ts.net via Funnel
                                                   (free Tailscale account; Funnel enabled for the device)

The computer must stay on and awake, and claude.ai cloud environments must be allowed to reach the
URL (add its host to the environment's allowed network hosts if it uses an allowlist).`,
		Examples: []string{
			"myco hub up --tunnel cloudflare --hostname hub.example.com",
			"myco hub up --tunnel tailscale",
			"myco hub up --tunnel cloudflare            # temporary URL, to try it out",
			"myco hub up --tunnel tailscale --dry-run   # show what would be installed",
		},
		Flags: []Flag{
			{Name: "tunnel", Kind: Enum, Values: append([]string{"none"}, tunnel.Kinds...), Required: true, Help: "cloudflare, tailscale, or none (reachable on this machine/LAN only)"},
			{Name: "hostname", Kind: String, Help: "cloudflare: your hostname on Cloudflare, e.g. hub.example.com (empty = temporary URL)"},
			{Name: "tunnel-name", Kind: String, Default: "myco", Help: "cloudflare: named tunnel to create or reuse"},
			{Name: "port", Kind: Int, Default: "7777", Help: "Local port (the hub only listens on 127.0.0.1)"},
			{Name: "no-service", Kind: Bool, Help: "Do not install a service; print the command to run instead"},
			{Name: "no-update-check", Kind: Bool, Help: "Do not look for new myco releases"},
		},
		Run: handler(func(e *env) error {
			if err := e.in.Require("tunnel"); err != nil {
				return err
			}
			kind, port := e.in.String("tunnel"), e.in.Int("port")
			if kind != "cloudflare" && e.in.String("hostname") != "" {
				return errorf("invalid_argument", ExitUsage, "--hostname is for --tunnel cloudflare; Tailscale picks <machine>.<tailnet>.ts.net", "--hostname needs --tunnel cloudflare")
			}
			dir := e.hubDir()
			local := "http://127.0.0.1:" + strconv.Itoa(port)
			dry := e.in.Bool("dry-run")
			bin, err := os.Executable()
			if err != nil {
				return err
			}
			if real, err := filepath.EvalSymlinks(bin); err == nil {
				bin = real
			}
			args := []string{"hub", "serve", "--addr", "127.0.0.1:" + strconv.Itoa(port), "--db", filepath.Join(dir, "hub.db"),
				"--admin-token-file", filepath.Join(dir, "hub.env"), "--files-dir", filepath.Join(dir, "files"), "--log-level", "info"}
			if e.in.Bool("no-update-check") {
				args = append(args, "--no-update-check")
			}
			if kind != "none" {
				args = append(args, "--tunnel", kind)
				if h := e.in.String("hostname"); h != "" {
					args = append(args, "--tunnel-hostname", h, "--tunnel-name", e.in.String("tunnel-name"))
				}
			}
			if dry {
				return e.dryRun(map[string]any{"action": "hub_up", "dir": dir, "service": service.HubID.Label, "command": append([]string{bin}, args...)})
			}

			// 1. Admin token (kept across re-runs) and local hub URL.
			created, err := writeHubEnv(dir, local)
			if err != nil {
				return err
			}
			// 2. Tunnel prerequisites now, in the foreground, so problems
			// (not logged in, Funnel disabled) show up here, not in a log.
			public := local
			if kind != "none" {
				t, err := tunnel.New(tunnel.Options{Kind: kind, Hostname: e.in.String("hostname"), Name: e.in.String("tunnel-name"), StateDir: dir})
				if err != nil {
					return err
				}
				fmt.Fprintf(e.in.Stderr, "==> preparing the %s tunnel\n", kind)
				p, err := t.Prepare(e.in.Ctx, local)
				var te *tunnel.Error
				if errors.As(err, &te) {
					return errorf("tunnel_not_ready", ExitUsage, te.Fix, "%s", te.Msg)
				}
				if err != nil {
					return err
				}
				public = p // "" for a quick tunnel: learned once it runs
			}
			// 3. The service (or the command to run).
			out := map[string]any{"dir": dir}
			if e.in.Bool("no-service") {
				out["command"] = append([]string{bin}, args...)
				return e.p.Print(out, func(w io.Writer) {
					fmt.Fprintf(w, "run the hub with:\n  %s\n", strings.Join(append([]string{bin}, args...), " "))
				})
			}
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			m, err := service.DetectID(home, service.ExecRunner{}, service.HubID)
			if err != nil {
				return errorf("unsupported", ExitError, "run it under your own process manager: myco hub up --no-service", "%v", err)
			}
			spec := service.Spec{Binary: bin, Args: args, Env: map[string]string{}, LogPath: filepath.Join(dir, "hub.log")}
			for _, k := range serviceEnvKeys {
				if v := e.in.Getenv(k); v != "" {
					spec.Env[k] = v
				}
			}
			plan, err := m.PlanInstall(spec)
			if err != nil {
				return err
			}
			if err := service.Apply(plan, service.ExecRunner{}); err != nil {
				return errorf("service_failed", ExitError, "see "+spec.LogPath, "%v", err)
			}
			fmt.Fprintf(e.in.Stderr, "==> hub service installed (%s); logs: %s\n", plan.Manager, spec.LogPath)

			// 4. Wait until it answers locally, then through the tunnel.
			if public, err = waitHub(e.in.Ctx, local, public, 90*time.Second); err != nil {
				return errorf("hub_not_ready", ExitUnavailable, "see the log: "+spec.LogPath, "%v", err)
			}
			reachable := kind == "none" || waitHealth(e.in.Ctx, public, 90*time.Second) == nil
			out["public_url"], out["reachable"], out["service"] = public, reachable, plan.Manager

			// 5. A join link on first setup.
			if created {
				if link, err := createLink(e.in.Ctx, local, dir); err == nil {
					out["join_url"] = link
				} else {
					out["join_link_error"] = err.Error()
				}
			}
			return e.p.Print(out, func(w io.Writer) {
				fmt.Fprintf(w, "\nHub URL: %s\n", public)
				if !reachable {
					fmt.Fprintln(w, "WARNING: not reachable through the tunnel yet (DNS can take a few minutes); check: curl "+public+"/v1/health")
				}
				if kind == "cloudflare" && e.in.String("hostname") == "" {
					fmt.Fprintln(w, "NOTE: this trycloudflare.com URL changes whenever the tunnel restarts; links and joined machines then stop working. Use --hostname for a stable URL.")
				}
				if l, ok := out["join_url"].(string); ok {
					fmt.Fprintf(w, "\nAdd a computer or server (macOS / Linux / WSL), run this on it:\n  curl -fsSL %s | bash\n", l)
					fmt.Fprintf(w, "\nConnect an AI session (Claude Code, Codex, claude.ai), tell it:\n  接入 myco：%s\n", l)
				}
				fmt.Fprintln(w, "\nNew join link any time: myco link create")
				if runtime.GOOS == "darwin" {
					fmt.Fprintln(w, "Keep this Mac awake while it serves the hub (System Settings → Energy / Battery → prevent automatic sleeping).")
				}
				for _, n := range plan.Notes {
					fmt.Fprintln(w, "note:", n)
				}
			})
		}),
	}
}

func hubDownCommand() *Command {
	return &Command{
		Name:     "down",
		Summary:  "Stop the hub started by `myco hub up` and remove its service (data is kept)",
		Mutating: true,
		Examples: []string{"myco hub down"},
		Run: handler(func(e *env) error {
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			m, err := service.DetectID(home, service.ExecRunner{}, service.HubID)
			if err != nil {
				return errorf("unsupported", ExitError, "", "%v", err)
			}
			plan := m.PlanUninstall()
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "hub_down", "plan": plan})
			}
			if err := service.Apply(plan, service.ExecRunner{}); err != nil {
				return err
			}
			_ = os.Remove(plan.File)
			return e.p.Print(map[string]any{"stopped": true, "data": e.hubDir()}, func(w io.Writer) {
				fmt.Fprintf(w, "hub stopped; its data stays in %s (myco hub up starts it again)\n", e.hubDir())
			})
		}),
	}
}

// writeHubEnv creates <dir>/hub.env with a new admin token, or updates the
// local hub URL in an existing one. It reports whether it was created.
func writeHubEnv(dir, local string) (bool, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return false, err
	}
	path := filepath.Join(dir, "hub.env")
	b, err := os.ReadFile(path)
	created := errors.Is(err, os.ErrNotExist)
	if err != nil && !created {
		return false, err
	}
	tok := tokenFromFile(string(b))
	if created || len(tok) < 16 {
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			return false, err
		}
		tok, created = hex.EncodeToString(raw), true
	}
	content := config.EnvAdminToken + "=" + tok + "\n" + config.EnvHub + "=" + local + "\n"
	return created, os.WriteFile(path, []byte(content), 0o600)
}

// waitHub waits for the local hub to answer and, when the public URL is
// not known yet (quick tunnel), for it to report one.
func waitHub(ctx context.Context, local, public string, d time.Duration) (string, error) {
	deadline := time.Now().Add(d)
	var last error
	for time.Now().Before(deadline) {
		h, err := health(ctx, local)
		switch {
		case err != nil:
			last = err
		case public != "":
			return public, nil
		case h.PublicURL != "":
			return h.PublicURL, nil
		default:
			last = errors.New("the tunnel has not reported a public URL yet")
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return "", fmt.Errorf("hub not ready after %s: %v", d, last)
}

func waitHealth(ctx context.Context, base string, d time.Duration) error {
	deadline := time.Now().Add(d)
	for {
		_, err := health(ctx, base)
		if err == nil || time.Now().After(deadline) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
}

func health(ctx context.Context, base string) (*api.Health, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/v1/health", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var h api.Health
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&h) != nil || !h.OK {
		return nil, fmt.Errorf("%s/v1/health: HTTP %d", base, resp.StatusCode)
	}
	return &h, nil
}

func createLink(ctx context.Context, local, dir string) (string, error) {
	b, err := os.ReadFile(filepath.Join(dir, "hub.env"))
	if err != nil {
		return "", err
	}
	c, err := client.New(local, tokenFromFile(string(b)))
	if err != nil {
		return "", err
	}
	res, err := c.CreateEnrollment(ctx, api.CreateEnrollmentRequest{Kind: "link", MaxUses: 10, TTLSeconds: int64((7 * 24 * time.Hour).Seconds()),
		Note: "created by myco hub up"}, false)
	if err != nil {
		return "", err
	}
	return res.JoinURL, nil
}
