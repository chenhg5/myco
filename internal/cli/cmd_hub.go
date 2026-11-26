package cli

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/app"
	. "github.com/myco-ai/myco/internal/cli/cmdkit"
	"github.com/myco-ai/myco/internal/config"
	"github.com/myco-ai/myco/internal/hubserver"
	"github.com/myco-ai/myco/internal/infra/tunnel"
	"github.com/myco-ai/myco/internal/release"
	"github.com/myco-ai/myco/internal/templates"
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
			{Name: "slow-request", Kind: Duration, Default: "1s", Help: "Log requests that take this long or longer as \"slow request\" (0s: never; long polls are left out)"},
			{Name: "public-url", Kind: String, Help: "Hub URL as agents see it (included in wake messages)"},
			{Name: "wake-cooldown", Kind: Duration, Default: "2m", Help: "Minimum time between wakes of the same push agent"},
			{Name: "github-webhook-secret-file", Kind: String, Help: "Enable POST /v1/ingress/github with this webhook secret"},
			{Name: "github-reply-from", Kind: List, Help: "Only accept GitHub replies from these logins"},
			{Name: "files-dir", Kind: String, Help: "Where attachment contents are stored (default: <db dir>/files)"},
			{Name: "page-files-max", Kind: Int, Default: "1000", Env: "MYCO_PAGE_FILES_MAX", Help: "Page-owned attachments per page (1..10000)"},
			{Name: "page-bytes-max", Kind: String, Default: "2147483648", Env: "MYCO_PAGE_BYTES_MAX", Help: "Logical attachment bytes per page (up to 21474836480)"},
			{Name: "project-files-max", Kind: Int, Default: "1000", Env: "MYCO_PROJECT_FILES_MAX", Help: "Default non-context files per project (1..10000)"},
			{Name: "project-bytes-max", Kind: String, Default: "2147483648", Env: "MYCO_PROJECT_BYTES_MAX", Help: "Default total logical bytes per project (up to 21474836480)"},
			{Name: "disk-min-free-bytes", Kind: String, Default: "2147483648", Env: "MYCO_DISK_MIN_FREE_BYTES", Help: "Disk space reserved after project uploads, in bytes"},
			{Name: "disk-min-free-percent", Kind: Int, Default: "5", Env: "MYCO_DISK_MIN_FREE_PERCENT", Help: "Disk percentage reserved after project uploads (0..99)"},
			{Name: "max-file-size", Kind: Int, Default: "50", Help: "Max attachment size in MB"},
			{Name: "file-ttl", Kind: Duration, Default: "168h", Help: "Delete attachments after this long (0 = keep)"},
			{Name: "tunnel", Kind: Enum, Values: tunnel.Kinds, Help: "Publish the hub through cloudflare (cloudflared) or tailscale (Funnel); its URL becomes --public-url"},
			{Name: "tunnel-hostname", Kind: String, Help: "cloudflare: your hostname, e.g. hub.example.com (empty = temporary trycloudflare.com URL)"},
			{Name: "tunnel-name", Kind: String, Default: "myco", Help: "cloudflare: named tunnel to create or reuse"},
			{Name: "backups", Kind: Int, Default: "7", Help: "Backups of its data to keep in <db dir>/backups, one made a day (0 = none)"},
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
			// The official apps: the binary's, and newer ones as myco
			// publishes them (fetched daily unless update checks are off).
			apps := &templates.Catalog{Download: templates.DownloadURL(in.Getenv), StateDir: filepath.Dir(in.String("db")), HubVersion: Version,
				Offline: in.Bool("no-update-check") || release.Disabled(in.Getenv), Log: newLogger(in.String("log-level"))}
			if err := apps.Load(); err != nil {
				return errorf("internal", ExitError, "", "official apps: %v", err)
			}
			// Upgrading from the App ends with this process stopping (the
			// service manager starts the new binary): as on SIGTERM.
			ctx, stop := context.WithCancel(in.Ctx)
			defer stop()
			storage, err := projectStoragePolicy(in)
			if err != nil {
				return err
			}
			pagePolicy, err := pageAttachmentPolicy(in)
			if err != nil {
				return err
			}
			return hubserver.Run(ctx, hubserver.Options{
				PageAttachments: &pagePolicy,
				ProjectStorage:  &storage,
				Upgrade:         hubUpgrader(in.Getenv, in.String("db"), stop),
				Tunnel:          tun, Version: Version, UpdateCheck: check, Apps: apps,
				Addr: in.String("addr"), DBPath: in.String("db"), AdminToken: tok, Policy: p,
				OnlineWindow: in.Duration("online-window"), Log: newLogger(in.String("log-level")),
				SlowRequest:         slowRequest(in.Duration("slow-request")),
				Dispatch:            app.DispatchPolicy{Cooldown: in.Duration("wake-cooldown"), PublicURL: in.String("public-url")},
				GitHubWebhookSecret: ghSecret, GitHubReplyFrom: in.List("github-reply-from"),
				FilesDir: in.String("files-dir"), Backups: backupsKeep(in.Int("backups")),
				Files: app.FilePolicy{MaxSize: int64(in.Int("max-file-size")) << 20, TTL: in.Duration("file-ttl")},
			})
		},
	}
	return (&Command{Name: "hub", Summary: "Operate a hub server"}).Add(serve, hubUpCommand(), hubDownCommand(), hubListCommand(), hubSettingsCommand(), hubBackupCommand(), hubRestoreCommand())
}

// backupsKeep maps --backups to hubserver.Options.Backups (0 there means
// the default; here it means none).
func backupsKeep(n int) int {
	if n <= 0 {
		return -1
	}
	return n
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

func projectStoragePolicy(in *Invocation) (app.ProjectStoragePolicy, error) {
	storage := app.DefaultProjectStoragePolicy()
	var parseErr error
	storage.FilesMax, parseErr = strconv.Atoi(in.String("project-files-max"))
	if parseErr != nil {
		return storage, errorf("invalid_argument", ExitUsage, "", "--project-files-max must be an integer")
	}
	storage.DiskMinFreePercent, parseErr = strconv.Atoi(in.String("disk-min-free-percent"))
	if parseErr != nil {
		return storage, errorf("invalid_argument", ExitUsage, "", "--disk-min-free-percent must be an integer")
	}
	storage.BytesMax, parseErr = strconv.ParseInt(in.String("project-bytes-max"), 10, 64)
	if parseErr != nil {
		return storage, errorf("invalid_argument", ExitUsage, "", "--project-bytes-max must be an integer byte count")
	}
	storage.DiskMinFreeBytes, parseErr = strconv.ParseInt(in.String("disk-min-free-bytes"), 10, 64)
	if parseErr != nil {
		return storage, errorf("invalid_argument", ExitUsage, "", "--disk-min-free-bytes must be an integer byte count")
	}
	if err := storage.Validate(); err != nil {
		return storage, err
	}

	return storage, nil
}

func pageAttachmentPolicy(in *Invocation) (app.PageAttachmentPolicy, error) {
	p := app.DefaultPageAttachmentPolicy()
	var err error
	p.FilesMax, err = strconv.Atoi(in.String("page-files-max"))
	if err != nil {
		return p, errorf("invalid_argument", ExitUsage, "", "--page-files-max must be an integer")
	}
	p.BytesMax, err = strconv.ParseInt(in.String("page-bytes-max"), 10, 64)
	if err != nil {
		return p, errorf("invalid_argument", ExitUsage, "", "--page-bytes-max must be an integer byte count")
	}
	return p, p.Validate()
}

// slowRequest: the flag's duration, 0 meaning never (the hub's own 0 means
// its default).
func slowRequest(d time.Duration) time.Duration {
	if d <= 0 {
		return -1
	}
	return d
}
