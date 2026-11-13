package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/api"
	. "github.com/chenhg5/myco/internal/cli/cmdkit"
	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/config"
	"github.com/chenhg5/myco/internal/infra/tunnel"
	"github.com/chenhg5/myco/internal/osutil"
	"github.com/chenhg5/myco/internal/service"
)

// hubDir is where `myco hub up` keeps a personal hub: hub.env (admin token,
// mode 600), hub.db, files/, hub.log, cloudflared.yml.
// LocalOnlyMarker, in the hub's directory, records `hub up --local-only`.
const LocalOnlyMarker = "local-only"

func (e *env) hubDir() string { return e.hubDirFor("") }

func (e *env) hubDirFor(name string) string {
	base := config.Dir(e.defaultPath)
	if name == "" {
		return filepath.Join(base, "hub")
	}
	return filepath.Join(base, "hubs", name)
}

func hubPort(dir string) int {
	vals, err := config.ReadEnvFile(filepath.Join(dir, "hub.env"))
	if err != nil {
		return 0
	}
	for _, kv := range vals {
		if v, ok := strings.CutPrefix(kv, config.EnvHub+"="); ok {
			u, err := url.Parse(v)
			if err == nil {
				p, _ := strconv.Atoi(u.Port())
				return p
			}
		}
	}
	return 0
}

func firstHubPort(base string) (int, error) {
	used := map[int]bool{hubPort(filepath.Join(base, "hub")): true}
	if entries, err := os.ReadDir(filepath.Join(base, "hubs")); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				used[hubPort(filepath.Join(base, "hubs", entry.Name()))] = true
			}
		}
	}
	for port := 7778; port <= 65535; port++ {
		if used[port] {
			continue
		}
		l, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err == nil {
			_ = l.Close()
			return port, nil
		}
	}
	return 0, errors.New("no free local port for the hub")
}

func hubUpCommand() *Command {
	return &Command{
		Name:     "up",
		Summary:  "Run a hub on this computer (no VPS needed), published through Cloudflare or Tailscale",
		Mutating: true,
		Long: `Installs the hub as a background service on this machine (launchd on macOS, systemd --user on
Linux) and publishes it at a public HTTPS URL through a tunnel, then prints a join link. Re-running
is safe (it updates the service and keeps its admin token and data). Without --name the original
hub stays in ~/.myco/hub on port 7777. With --name it lives in ~/.myco/hubs/<name> and gets its
own service; if --port is omitted, the first available port from 7778 is used.

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
			{Name: "name", Kind: String, Help: "Named hub (1–32 lowercase letters, digits or hyphens); omitted for the original default hub"},
			{Name: "tunnel", Kind: Enum, Values: append([]string{"none"}, tunnel.Kinds...), Required: true, Help: "cloudflare, tailscale, or none (reachable on this machine and its local network only)"},
			{Name: "hostname", Kind: String, Help: "cloudflare: your hostname on Cloudflare, e.g. hub.example.com (empty = temporary URL)"},
			{Name: "tunnel-name", Kind: String, Default: "myco", Help: "cloudflare: named tunnel to create or reuse"},
			{Name: "port", Kind: Int, Default: "7777", Help: "Port (default hub: 7777; new named hub: first free from 7778). With a tunnel listen on 127.0.0.1; with none on every interface"},
			{Name: "local-only", Kind: Bool, Help: "--tunnel none: listen on 127.0.0.1 only (this machine; no LAN access)"},
			{Name: "no-service", Kind: Bool, Help: "Do not install a service; print the command to run instead"},
			{Name: "no-update-check", Kind: Bool, Help: "Do not look for new myco releases"},
		},
		Run: handler(func(e *env) error {
			if err := e.in.Require("tunnel"); err != nil {
				return err
			}
			name := e.in.String("name")
			if !service.ValidHubName(name) || (e.in.IsSet("name") && name == "") {
				return errorf("invalid_argument", ExitUsage, "use 1–32 lowercase letters, digits or hyphens", "invalid hub name %q", name)
			}
			kind, port := e.in.String("tunnel"), e.in.Int("port")
			if kind != "cloudflare" && e.in.String("hostname") != "" {
				return errorf("invalid_argument", ExitUsage, "--hostname is for --tunnel cloudflare; Tailscale picks <machine>.<tailnet>.ts.net", "--hostname needs --tunnel cloudflare")
			}
			dir := e.hubDirFor(name)
			if name != "" && !e.in.IsSet("port") {
				port = hubPort(dir)
				if port == 0 {
					var err error
					port, err = firstHubPort(config.Dir(e.defaultPath))
					if err != nil {
						return err
					}
				}
			}
			if port < 1 || port > 65535 {
				return errorf("invalid_argument", ExitUsage, "choose a port from 1 to 65535", "invalid port %d", port)
			}
			tunnelName := e.in.String("tunnel-name")
			if name != "" && !e.in.IsSet("tunnel-name") {
				tunnelName = "myco-" + name
			}
			id := service.HubIDFor(name)
			local := "http://127.0.0.1:" + strconv.Itoa(port)
			dry := e.in.Bool("dry-run")
			bin, err := os.Executable()
			if err != nil {
				return err
			}
			if real, err := filepath.EvalSymlinks(bin); err == nil {
				bin = real
			}
			// Without a tunnel, other computers on the network reach the hub
			// directly; with one, only the tunnel (and this machine) does.
			listen := "127.0.0.1:" + strconv.Itoa(port)
			lan := kind == "none" && !e.in.Bool("local-only")
			if lan {
				listen = "0.0.0.0:" + strconv.Itoa(port)
			}
			args := []string{"hub", "serve", "--addr", listen, "--db", filepath.Join(dir, "hub.db"),
				"--admin-token-file", filepath.Join(dir, "hub.env"), "--files-dir", filepath.Join(dir, "files"), "--log-level", "info"}
			if e.in.Bool("no-update-check") {
				args = append(args, "--no-update-check")
			}
			if kind != "none" {
				args = append(args, "--tunnel", kind)
				if h := e.in.String("hostname"); h != "" {
					args = append(args, "--tunnel-hostname", h, "--tunnel-name", tunnelName)
				}
			}
			if dry {
				return e.dryRun(map[string]any{"action": "hub_up", "name": name, "dir": dir, "service": id.Label, "command": append([]string{bin}, args...)})
			}

			// 1. Admin token (kept across re-runs) and local hub URL.
			created, err := writeHubEnv(dir, local)
			if err != nil {
				return err
			}
			// Remember a deliberate --local-only, so the app does not "fix"
			// a hub that listens on 127.0.0.1 on purpose.
			if marker := filepath.Join(dir, LocalOnlyMarker); kind == "none" && e.in.Bool("local-only") {
				_ = os.WriteFile(marker, []byte("myco hub up --local-only\n"), 0o600)
			} else {
				_ = os.Remove(marker)
			}
			// 2. Tunnel prerequisites now, in the foreground, so problems
			// (not logged in, Funnel disabled) show up here, not in a log.
			public := local
			if kind != "none" {
				t, err := tunnel.New(tunnel.Options{Kind: kind, Hostname: e.in.String("hostname"), Name: tunnelName, StateDir: dir})
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
			m, err := service.DetectID(home, service.ExecRunner{}, id)
			if err != nil {
				return errorf("unsupported", ExitError, "run it under your own process manager: myco hub up --no-service", "%v", err)
			}
			spec := service.Spec{Binary: bin, Args: args, LogPath: filepath.Join(dir, "hub.log")}
			spec.Env, _ = serviceEnv(e.in.Getenv) // the hub reaches GitHub for update checks
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
			_ = writeHubPublic(dir, public)
			if lan {
				if ip := osutil.LANAddr(); ip != "" {
					out["lan_url"] = "http://" + ip + ":" + strconv.Itoa(port)
				}
			}

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
				if l, ok := out["lan_url"].(string); ok {
					fmt.Fprintf(w, "On your local network: %s (links made on this machine use it; other computers need to be on the same network)\n", l)
				}
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
				if name == "" {
					fmt.Fprintln(w, "\nNew join link any time: myco link create")
				} else {
					fmt.Fprintf(w, "\nNew join link any time: MYCO_HUB_NAME=%s myco link create\n", name)
				}
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
		Examples: []string{"myco hub down", "myco hub down --name work"},
		Flags:    []Flag{{Name: "name", Kind: String, Help: "Named hub to stop; omitted for the default hub"}},
		Run: handler(func(e *env) error {
			name := e.in.String("name")
			if !service.ValidHubName(name) || (e.in.IsSet("name") && name == "") {
				return errorf("invalid_argument", ExitUsage, "use 1–32 lowercase letters, digits or hyphens", "invalid hub name %q", name)
			}
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			m, err := service.DetectID(home, service.ExecRunner{}, service.HubIDFor(name))
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
			return e.p.Print(map[string]any{"stopped": true, "data": e.hubDirFor(name)}, func(w io.Writer) {
				fmt.Fprintf(w, "hub stopped; its data stays in %s (myco hub up starts it again)\n", e.hubDirFor(name))
			})
		}),
	}
}

type localHubItem struct {
	Name      string `json:"name"`
	Dir       string `json:"dir"`
	LocalURL  string `json:"local_url"`
	PublicURL string `json:"public_url"`
	Running   bool   `json:"running"`
	Service   string `json:"service"`
}

func writeHubPublic(dir, public string) error {
	return os.WriteFile(filepath.Join(dir, "public-url"), []byte(public+"\n"), 0o600)
}

func localHubDirs(base string) []localHubItem {
	var out []localHubItem
	add := func(name, dir string) {
		port := hubPort(dir)
		if port == 0 {
			return
		}
		public, _ := os.ReadFile(filepath.Join(dir, "public-url"))
		id := service.HubIDFor(name)
		serviceName := id.Unit
		if runtime.GOOS == "darwin" {
			serviceName = id.Label
		}
		if runtime.GOOS == "windows" {
			serviceName = "app-hub.json"
		}
		out = append(out, localHubItem{Name: name, Dir: dir, LocalURL: "http://127.0.0.1:" + strconv.Itoa(port), PublicURL: strings.TrimSpace(string(public)), Service: serviceName})
	}
	add("", filepath.Join(base, "hub"))
	if entries, err := os.ReadDir(filepath.Join(base, "hubs")); err == nil {
		for _, entry := range entries {
			if entry.IsDir() && service.ValidHubName(entry.Name()) {
				add(entry.Name(), filepath.Join(base, "hubs", entry.Name()))
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func hubListCommand() *Command {
	return &Command{Name: "list", Summary: "List hubs started with `myco hub up` on this computer", Run: handler(func(e *env) error {
		items := localHubDirs(config.Dir(e.defaultPath))
		home, _ := os.UserHomeDir()
		for i := range items {
			if m, err := service.DetectID(home, service.ExecRunner{}, service.HubIDFor(items[i].Name)); err == nil {
				if st, err := m.Status(); err == nil {
					items[i].Running = st.Running
				}
			}
			ctx, cancel := context.WithTimeout(e.in.Ctx, 500*time.Millisecond)
			if h, err := health(ctx, items[i].LocalURL); err == nil {
				items[i].Running = true
				if h.PublicURL != "" {
					items[i].PublicURL = h.PublicURL
				}
			}
			cancel()
		}
		return e.p.Print(map[string]any{"hubs": items}, func(w io.Writer) {
			for _, h := range items {
				fmt.Fprintf(w, "%s\t%s\t%s\t%v\t%s\n", orStr(h.Name, "(default)"), h.LocalURL, h.PublicURL, h.Running, h.Service)
			}
		})
	})}
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
