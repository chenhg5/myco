package cli

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/chenhg5/myco/internal/app"
	. "github.com/chenhg5/myco/internal/cli/cmdkit"
	"github.com/chenhg5/myco/internal/config"
	"github.com/chenhg5/myco/internal/hubserver"
	"github.com/chenhg5/myco/internal/infra/tunnel"
	"github.com/chenhg5/myco/internal/release"
)

func hubCommands() *Command {
	serve := &Command{
		Name:    "serve",
		Summary: "Run the hub server (registry, inboxes, delivery)",
		Long:    "Requires MYCO_ADMIN_TOKEN (or --admin-token-file). Put it behind TLS (e.g. Caddy) in production.",
		Examples: []string{
			"MYCO_ADMIN_TOKEN=$(openssl rand -hex 32) myco hub serve --addr :7777 --db /var/lib/myco/hub.db",
		},
		Flags: []Flag{
			{Name: "addr", Kind: String, Default: "127.0.0.1:7777", Help: "Listen address"},
			{Name: "db", Kind: String, Default: "myco.db", Help: "SQLite database path"},
			{Name: "admin-token-file", Kind: String, Help: "File containing the admin token (instead of MYCO_ADMIN_TOKEN)"},
			{Name: "lease", Kind: Duration, Default: "5m", Help: "How long a pulled, unacked message stays invisible"},
			{Name: "max-attempts", Kind: Int, Default: "5", Help: "Deliveries before an unacked message fails"},
			{Name: "online-window", Kind: Duration, Default: "90s", Help: "Agents seen within this window count as online"},
			{Name: "log-level", Kind: Enum, Values: []string{"debug", "info", "warn", "error"}, Default: "info", Help: "Log verbosity (stderr)"},
			{Name: "public-url", Kind: String, Help: "Hub URL as agents see it (included in wake messages)"},
			{Name: "wake-cooldown", Kind: Duration, Default: "2m", Help: "Minimum time between wakes of the same push agent"},
			{Name: "github-webhook-secret-file", Kind: String, Help: "Enable POST /v1/ingress/github with this webhook secret"},
			{Name: "github-reply-from", Kind: List, Help: "Only accept GitHub replies from these logins"},
			{Name: "files-dir", Kind: String, Help: "Where attachment contents are stored (default: <db dir>/files)"},
			{Name: "max-file-size", Kind: Int, Default: "50", Help: "Max attachment size in MB"},
			{Name: "file-ttl", Kind: Duration, Default: "168h", Help: "Delete attachments after this long (0 = keep)"},
			{Name: "tunnel", Kind: Enum, Values: tunnel.Kinds, Help: "Publish the hub through cloudflare (cloudflared) or tailscale (Funnel); its URL becomes --public-url"},
			{Name: "tunnel-hostname", Kind: String, Help: "cloudflare: your hostname, e.g. hub.example.com (empty = temporary trycloudflare.com URL)"},
			{Name: "tunnel-name", Kind: String, Default: "myco", Help: "cloudflare: named tunnel to create or reuse"},
			{Name: "no-update-check", Kind: Bool, Help: "Do not look for new myco releases (default: once a day in the background; also MYCO_NO_UPDATE_CHECK=1)"},
		},
		Run: func(in *Invocation) error {
			tok := in.Getenv(config.EnvAdminToken)
			if f := in.String("admin-token-file"); f != "" {
				b, err := os.ReadFile(f)
				if err != nil {
					return errorf("invalid_argument", ExitUsage, "", "cannot read --admin-token-file: %v", err)
				}
				tok = tokenFromFile(string(b))
			}
			if len(tok) < 16 {
				return errorf("not_configured", ExitUsage, "generate one with: openssl rand -hex 32",
					"admin token missing or shorter than 16 chars (set MYCO_ADMIN_TOKEN)")
			}
			p := app.DefaultMessagePolicy()
			p.Lease = in.Duration("lease")
			p.MaxAttempts = in.Int("max-attempts")
			ghSecret := ""
			if f := in.String("github-webhook-secret-file"); f != "" {
				b, err := os.ReadFile(f)
				if err != nil {
					return errorf("invalid_argument", ExitUsage, "", "cannot read --github-webhook-secret-file: %v", err)
				}
				ghSecret = strings.TrimSpace(string(b))
			}
			var tun tunnel.Tunnel
			if k := in.String("tunnel"); k != "" {
				var err error
				if tun, err = tunnel.New(tunnel.Options{Kind: k, Hostname: in.String("tunnel-hostname"), Name: in.String("tunnel-name"),
					StateDir: filepath.Dir(in.String("db")), Log: newLogger(in.String("log-level"))}); err != nil {
					return errorf("invalid_argument", ExitUsage, "", "%v", err)
				}
			}
			var check *release.Checker
			if !in.Bool("no-update-check") && !release.Disabled(in.Getenv) {
				check = &release.Checker{API: release.APIURL(in.Getenv), Log: newLogger(in.String("log-level"))}
			}
			return hubserver.Run(in.Ctx, hubserver.Options{
				Tunnel: tun, Version: Version, UpdateCheck: check,
				Addr: in.String("addr"), DBPath: in.String("db"), AdminToken: tok, Policy: p,
				OnlineWindow: in.Duration("online-window"), Log: newLogger(in.String("log-level")),
				Dispatch:            app.DispatchPolicy{Cooldown: in.Duration("wake-cooldown"), PublicURL: in.String("public-url")},
				GitHubWebhookSecret: ghSecret, GitHubReplyFrom: in.List("github-reply-from"),
				FilesDir: in.String("files-dir"),
				Files:    app.FilePolicy{MaxSize: int64(in.Int("max-file-size")) << 20, TTL: in.Duration("file-ttl")},
			})
		},
	}
	return (&Command{Name: "hub", Summary: "Operate a hub server"}).Add(serve, hubUpCommand(), hubDownCommand(), hubSettingsCommand())
}

// tokenFromFile reads an admin token file: either just the token, or
// KEY=VALUE lines with MYCO_ADMIN_TOKEN (a hub.env file).
func tokenFromFile(content string) string {
	for _, line := range strings.Split(content, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), config.EnvAdminToken+"="); ok {
			return strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return strings.TrimSpace(content)
}

func newLogger(level string) *slog.Logger { return newLoggerTo(os.Stderr, level) }

func newLoggerTo(w io.Writer, level string) *slog.Logger {
	var l slog.Level
	_ = l.UnmarshalText([]byte(level))
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: l}))
}
