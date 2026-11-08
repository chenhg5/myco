package console

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/service"
)

// The local hub before v0.18.3: `myco hub up --tunnel none` made it listen on
// 127.0.0.1 only, so other computers on the network could not reach it and
// its links named 127.0.0.1. The app moves such a hub onto the LAN once, in
// the background: it re-runs `myco hub up --tunnel none` (upgrading the CLI
// first when it is too old to know the difference). A hub set up with
// --local-only on purpose, or behind a tunnel, is left alone.

// localOnlyMarker matches cli.LocalOnlyMarker (written by hub up --local-only).
const localOnlyMarker = "local-only"

// hubServiceFile returns the local hub's service definition ("" if none).
func (l *Local) hubServiceFile() string {
	if l.ServiceFile != nil {
		return l.ServiceFile()
	}
	m, err := service.DetectID(l.Home, service.ExecRunner{}, service.HubID)
	if err != nil {
		return ""
	}
	return m.PlanUninstall().File
}

// hubNeedsLAN reports whether the local hub listens on 127.0.0.1 only
// without having been asked to.
func (l *Local) hubNeedsLAN() bool {
	f := l.hubServiceFile()
	if f == "" {
		return false
	}
	b, err := os.ReadFile(f)
	if err != nil {
		return false
	}
	def := string(b)
	if !strings.Contains(def, "127.0.0.1:") || strings.Contains(def, "--tunnel") {
		return false
	}
	_, err = os.Stat(filepath.Join(l.Home, ".myco", "hub", localOnlyMarker))
	return os.IsNotExist(err)
}

// cliKnowsLAN reports whether the installed myco CLI's hub up listens on the
// LAN (it has --local-only).
func (l *Local) cliKnowsLAN(ctx context.Context) bool {
	out, err := l.run(ctx, "hub", "up", "--help", "--format", "text")
	return err == nil && strings.Contains(string(out), "local-only")
}

// moveHubToLAN does the move described above, once per app run.
func (l *Local) moveHubToLAN() {
	l.lanOnce.Do(func() {
		if !l.hubNeedsLAN() {
			return
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
			defer cancel()
			log := slog.Default().With("component", "local-hub")
			if !l.cliKnowsLAN(ctx) {
				if out, err := l.run(ctx, "upgrade"); err != nil {
					log.Warn("could not upgrade the myco CLI for the LAN hub", "err", err, "out", lastLine(out))
					return
				}
				if !l.cliKnowsLAN(ctx) {
					return
				}
			}
			if out, err := l.run(ctx, "hub", "up", "--tunnel", "none"); err != nil {
				log.Warn("could not move the local hub onto the LAN", "err", err, "out", lastLine(out))
				return
			}
			log.Info("the local hub now listens on the LAN")
		}()
	})
}
