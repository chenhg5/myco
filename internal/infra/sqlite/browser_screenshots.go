package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/myco-ai/myco/internal/domain"
)

type BrowserScreenshotRepo struct{ db *sql.DB }

// OpenBrowserScreenshots keeps private, expiring content outside workspace
// backups. A separate database also allows secure deletion of expired images.
func OpenBrowserScreenshots(path string) (*BrowserScreenshotRepo, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`
PRAGMA busy_timeout = 5000;
PRAGMA secure_delete = ON;
CREATE TABLE IF NOT EXISTS browser_screenshots (
	id TEXT PRIMARY KEY, agent_id TEXT NOT NULL, owner_id TEXT NOT NULL,
	media_type TEXT NOT NULL, expires_at INTEGER NOT NULL, data BLOB NOT NULL
);
CREATE INDEX IF NOT EXISTS browser_screenshot_expiry ON browser_screenshots(expires_at);
CREATE INDEX IF NOT EXISTS browser_screenshot_owner ON browser_screenshots(owner_id, expires_at DESC);
`)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return &BrowserScreenshotRepo{db}, nil
}

func (r *BrowserScreenshotRepo) Close() error { return r.db.Close() }

func (r *BrowserScreenshotRepo) Save(ctx context.Context, a domain.BrowserScreenshot) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO browser_screenshots(id,agent_id,owner_id,media_type,expires_at,data) VALUES(?,?,?,?,?,?)`,
		a.ID, a.AgentID, a.OwnerID, a.MediaType, a.ExpiresAt.UnixMilli(), a.Data)
	return err
}

func (r *BrowserScreenshotRepo) Get(ctx context.Context, id, agent, owner string, now time.Time) (*domain.BrowserScreenshot, error) {
	var a domain.BrowserScreenshot
	var expires int64
	err := r.db.QueryRowContext(ctx, `SELECT id,media_type,expires_at,data FROM browser_screenshots
		WHERE id=? AND expires_at>? AND ((?<>'' AND agent_id=?) OR (?<>'' AND owner_id=?))`,
		id, now.UnixMilli(), agent, agent, owner, owner).Scan(&a.ID, &a.MediaType, &expires, &a.Data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.Errf(domain.CodeNotFound, "browser screenshot not found (or no longer kept)")
	}
	if err != nil {
		return nil, err
	}
	a.ExpiresAt, a.Bytes = time.UnixMilli(expires).UTC(), len(a.Data)
	return &a, nil
}

func (r *BrowserScreenshotRepo) List(ctx context.Context, owner string, now time.Time) ([]domain.BrowserScreenshot, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,media_type,expires_at,length(data) FROM browser_screenshots
		WHERE owner_id=? AND expires_at>? ORDER BY expires_at DESC,id DESC LIMIT 100`, owner, now.UnixMilli())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.BrowserScreenshot{}
	for rows.Next() {
		var a domain.BrowserScreenshot
		var expires int64
		if err := rows.Scan(&a.ID, &a.MediaType, &expires, &a.Bytes); err != nil {
			return nil, err
		}
		a.ExpiresAt = time.UnixMilli(expires).UTC()
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *BrowserScreenshotRepo) Prune(ctx context.Context, now time.Time) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM browser_screenshots WHERE expires_at<=?`, now.UnixMilli())
	return err
}
