package cli

import (
	"log/slog"
	"os"
	"strings"

	"github.com/chenhg5/myco/internal/app"
	. "github.com/chenhg5/myco/internal/cli/cmdkit"
	"github.com/chenhg5/myco/internal/config"
	"github.com/chenhg5/myco/internal/hubserver"
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
		},
		Run: func(in *Invocation) error {
			tok := in.Getenv(config.EnvAdminToken)
			if f := in.String("admin-token-file"); f != "" {
				b, err := os.ReadFile(f)
				if err != nil {
					return errorf("invalid_argument", ExitUsage, "", "cannot read --admin-token-file: %v", err)
				}
				tok = strings.TrimSpace(string(b))
			}
			if len(tok) < 16 {
				return errorf("not_configured", ExitUsage, "generate one with: openssl rand -hex 32",
					"admin token missing or shorter than 16 chars (set MYCO_ADMIN_TOKEN)")
			}
			p := app.DefaultMessagePolicy()
			p.Lease = in.Duration("lease")
			p.MaxAttempts = in.Int("max-attempts")
			return hubserver.Run(in.Ctx, hubserver.Options{
				Addr: in.String("addr"), DBPath: in.String("db"), AdminToken: tok, Policy: p,
				OnlineWindow: in.Duration("online-window"), Log: newLogger(in.String("log-level")),
			})
		},
	}
	return (&Command{Name: "hub", Summary: "Operate a hub server"}).Add(serve)
}

func newLogger(level string) *slog.Logger {
	var l slog.Level
	_ = l.UnmarshalText([]byte(level))
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: l}))
}
