package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/chenhg5/myco/internal/domain"
)

// FileRepo implements domain.FileRepository.
type FileRepo struct{ db *sql.DB }

// NewFileRepo returns a SQLite-backed file repository.
func NewFileRepo(db *sql.DB) *FileRepo { return &FileRepo{db: db} }

var _ domain.FileRepository = (*FileRepo)(nil)

const fileCols = `id, name, size, sha256, content_type, owner, created_at, expires_at`

func (r *FileRepo) Create(ctx context.Context, f *domain.File) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return internal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO files (`+fileCols+`) VALUES (?,?,?,?,?,?,?,?)`,
		f.ID, f.Name, f.Size, f.SHA256, f.ContentType, f.Owner, ms(f.CreatedAt), ms(f.ExpiresAt)); err != nil {
		return internal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO file_access (file_id, agent) VALUES (?,?)`, f.ID, f.Owner); err != nil {
		return internal(err)
	}
	return internal(tx.Commit())
}

func (r *FileRepo) Get(ctx context.Context, id string) (*domain.File, error) {
	var (
		f                domain.File
		created, expires int64
	)
	err := r.db.QueryRowContext(ctx, `SELECT `+fileCols+` FROM files WHERE id=?`, id).
		Scan(&f.ID, &f.Name, &f.Size, &f.SHA256, &f.ContentType, &f.Owner, &created, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.Errf(domain.CodeNotFound, "file %s not found (it may have expired)", id)
	}
	if err != nil {
		return nil, internal(err)
	}
	f.CreatedAt, f.ExpiresAt = fromMS(created), fromMS(expires)
	return &f, nil
}

func (r *FileRepo) Grant(ctx context.Context, id string, agent domain.AgentName) error {
	_, err := r.db.ExecContext(ctx, `INSERT OR IGNORE INTO file_access (file_id, agent) VALUES (?,?)`, id, agent)
	return internal(err)
}

func (r *FileRepo) CanAccess(ctx context.Context, id string, agent domain.AgentName) (bool, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM file_access WHERE file_id=? AND agent=?`, id, agent).Scan(&n)
	return n > 0, internal(err)
}

func (r *FileRepo) ListExpired(ctx context.Context, now time.Time, limit int) ([]*domain.File, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id FROM files WHERE expires_at > 0 AND expires_at < ? LIMIT ?`, ms(now), limit)
	if err != nil {
		return nil, internal(err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, internal(err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	var out []*domain.File
	for _, id := range ids {
		if f, err := r.Get(ctx, id); err == nil {
			out = append(out, f)
		}
	}
	return out, nil
}

func (r *FileRepo) Delete(ctx context.Context, id string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return internal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM file_access WHERE file_id=?`, id); err != nil {
		return internal(err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM files WHERE id=?`, id); err != nil {
		return internal(err)
	}
	return internal(tx.Commit())
}
