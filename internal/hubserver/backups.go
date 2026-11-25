package hubserver

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/backup"
	"github.com/myco-ai/myco/internal/domain"
)

// DefaultBackups is how many backups a hub keeps of its own data.
const DefaultBackups = 7

// backups makes and keeps the backups of one hub (the server's own, or a
// workspace's): its database and files, into <its dir>/backups.
type backups struct {
	dir     string
	source  backup.Source
	scope   string
	version string
	keep    int
	log     *slog.Logger
	mu      sync.Mutex // one backup at a time
}

func newBackups(dbPath, filesDir, scope, version string, keep int, log *slog.Logger) *backups {
	if dbPath == "" || dbPath == ":memory:" || keep <= 0 {
		return nil
	}
	if filesDir == "" {
		filesDir = filepath.Join(filepath.Dir(dbPath), "files")
	}
	return &backups{dir: filepath.Join(filepath.Dir(dbPath), "backups"), source: backup.Source{DBPath: dbPath, FilesDir: filesDir},
		scope: scope, version: version, keep: keep, log: log}
}

func (b *backups) Info() (api.Backups, error) {
	list, err := backup.List(b.dir)
	if err != nil {
		return api.Backups{}, err
	}
	out := api.Backups{Backups: make([]api.Backup, 0, len(list)), Keep: b.keep, Daily: true,
		Restore: "on the server, with the hub stopped: myco hub restore --from <backup> --db " + b.source.DBPath}
	for _, x := range list {
		out.Backups = append(out.Backups, api.Backup{Name: x.Name, Size: x.Size, CreatedAt: x.CreatedAt})
	}
	return out, nil
}

func (b *backups) Create(ctx context.Context) (api.Backup, error) {
	if !b.mu.TryLock() {
		return api.Backup{}, domain.Errf(domain.CodeConflict, "a backup is being made").WithSuggestion("wait a minute and list the backups again")
	}
	defer b.mu.Unlock()
	info, err := backup.Write(ctx, b.dir, []backup.Source{b.source}, b.scope, b.version, time.Now())
	if err != nil {
		return api.Backup{}, domain.Errf(domain.CodeInternal, "making the backup: %v", err)
	}
	if err := backup.Prune(b.dir, b.keep); err != nil && b.log != nil {
		b.log.Warn("removing old backups failed", "err", err)
	}
	return api.Backup{Name: info.Name, Size: info.Size, CreatedAt: info.CreatedAt}, nil
}

func (b *backups) Open(name string) (*os.File, error) {
	if !backup.ValidName(name) {
		return nil, domain.Errf(domain.CodeNotFound, "no backup %s", name)
	}
	f, err := os.Open(filepath.Join(b.dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return nil, domain.Errf(domain.CodeNotFound, "no backup %s", name).WithSuggestion("list them: GET /v1/admin/backups")
	}
	return f, err
}

// daily makes a backup when the newest is a day old (checked every hour,
// the first time a few minutes after the hub starts), until ctx ends.
func (b *backups) daily(ctx context.Context, first time.Duration) {
	t := time.NewTimer(first)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		list, err := backup.List(b.dir)
		if err == nil && (len(list) == 0 || time.Since(list[0].CreatedAt) > 23*time.Hour) {
			start := time.Now()
			if info, err := b.Create(ctx); err != nil {
				if b.log != nil {
					b.log.Error("daily backup failed", "err", err)
				}
			} else if b.log != nil {
				b.log.Info("daily backup made", "name", info.Name, "bytes", info.Size, "took", time.Since(start).Round(time.Millisecond))
			}
		}
		t.Reset(time.Hour)
	}
}
