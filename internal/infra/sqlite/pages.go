package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/chenhg5/myco/internal/domain"
)

// PageRepo implements domain.PageRepository.
type PageRepo struct{ db *sql.DB }

// NewPageRepo returns a SQLite-backed page repository.
func NewPageRepo(db *sql.DB) *PageRepo { return &PageRepo{db: db} }

var _ domain.PageRepository = (*PageRepo)(nil)

const pageCols = `id, name, project, publisher, owner, latest, created_at, updated_at`

func (r *PageRepo) Create(ctx context.Context, p *domain.Page) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO pages (`+pageCols+`) VALUES (?,?,?,?,?,?,?,?)`,
		p.ID, p.Name, p.Project, p.Publisher, p.Owner, p.Latest, ms(p.CreatedAt), ms(p.UpdatedAt))
	if isUniqueViolation(err) {
		return domain.Errf(domain.CodeConflict, "page %s exists", p.ID)
	}
	return internal(err)
}

func (r *PageRepo) Save(ctx context.Context, p *domain.Page) error {
	res, err := r.db.ExecContext(ctx, `UPDATE pages SET name=?, project=?, latest=?, updated_at=? WHERE id=?`,
		p.Name, p.Project, p.Latest, ms(p.UpdatedAt), p.ID)
	if err != nil {
		return internal(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.Errf(domain.CodeNotFound, "page %s not found", p.ID)
	}
	return nil
}

func scanPage(s scanner) (*domain.Page, error) {
	var (
		p                domain.Page
		created, updated int64
	)
	if err := s.Scan(&p.ID, &p.Name, &p.Project, &p.Publisher, &p.Owner, &p.Latest, &created, &updated); err != nil {
		return nil, err
	}
	p.CreatedAt, p.UpdatedAt = fromMS(created), fromMS(updated)
	return &p, nil
}

func (r *PageRepo) Get(ctx context.Context, id string) (*domain.Page, error) {
	p, err := scanPage(r.db.QueryRowContext(ctx, `SELECT `+pageCols+` FROM pages WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.Errf(domain.CodeNotFound, "page %s not found", id).WithSuggestion("list pages with: myco page list")
	}
	if err != nil {
		return nil, internal(err)
	}
	return p, nil
}

func (r *PageRepo) List(ctx context.Context) ([]*domain.Page, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+pageCols+` FROM pages ORDER BY updated_at DESC`)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	var out []*domain.Page
	for rows.Next() {
		p, err := scanPage(rows)
		if err != nil {
			return nil, internal(err)
		}
		out = append(out, p)
	}
	return out, internal(rows.Err())
}

func (r *PageRepo) Delete(ctx context.Context, id string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return internal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM page_versions WHERE page=?`, id); err != nil {
		return internal(err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM pages WHERE id=?`, id); err != nil {
		return internal(err)
	}
	return internal(tx.Commit())
}

const versionCols = `page, n, blob, sha256, size, entry, files, by_name, at`

func (r *PageRepo) AddVersion(ctx context.Context, v *domain.PageVersion) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO page_versions (`+versionCols+`) VALUES (?,?,?,?,?,?,?,?,?)`,
		v.Page, v.N, v.Blob, v.SHA256, v.Size, v.Entry, v.Files, v.By, ms(v.At))
	if isUniqueViolation(err) {
		return domain.Errf(domain.CodeConflict, "version %d of page %s exists", v.N, v.Page).WithSuggestion("someone published at the same time; publish again")
	}
	return internal(err)
}

func (r *PageRepo) Versions(ctx context.Context, page string) ([]*domain.PageVersion, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+versionCols+` FROM page_versions WHERE page=? ORDER BY n DESC`, page)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	var out []*domain.PageVersion
	for rows.Next() {
		var (
			v  domain.PageVersion
			at int64
		)
		if err := rows.Scan(&v.Page, &v.N, &v.Blob, &v.SHA256, &v.Size, &v.Entry, &v.Files, &v.By, &at); err != nil {
			return nil, internal(err)
		}
		v.At = fromMS(at)
		out = append(out, &v)
	}
	return out, internal(rows.Err())
}

func (r *PageRepo) DeleteVersion(ctx context.Context, page string, n int) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM page_versions WHERE page=? AND n=?`, page, n)
	return internal(err)
}
