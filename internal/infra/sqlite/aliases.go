package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/chenhg5/myco/internal/domain"
)

// AliasRepo implements domain.AliasRepository.
type AliasRepo struct{ db *sql.DB }

// NewAliasRepo returns a SQLite-backed alias repository.
func NewAliasRepo(db *sql.DB) *AliasRepo { return &AliasRepo{db: db} }

var _ domain.AliasRepository = (*AliasRepo)(nil)

func (r *AliasRepo) Set(ctx context.Context, a *domain.Alias) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO aliases (name, target, created_at, updated_at) VALUES (?,?,?,?)
		ON CONFLICT(name) DO UPDATE SET target=excluded.target, updated_at=excluded.updated_at`,
		a.Name, a.Target, ms(a.CreatedAt), ms(a.UpdatedAt))
	return internal(err)
}

func (r *AliasRepo) Get(ctx context.Context, name domain.AgentName) (*domain.Alias, error) {
	var (
		a                domain.Alias
		created, updated int64
	)
	err := r.db.QueryRowContext(ctx, `SELECT name, target, created_at, updated_at FROM aliases WHERE name=?`, name).
		Scan(&a.Name, &a.Target, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.Errf(domain.CodeNotFound, "alias %s not found", name)
	}
	if err != nil {
		return nil, internal(err)
	}
	a.CreatedAt, a.UpdatedAt = fromMS(created), fromMS(updated)
	return &a, nil
}

func (r *AliasRepo) Delete(ctx context.Context, name domain.AgentName) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM aliases WHERE name=?`, name)
	if err != nil {
		return internal(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.Errf(domain.CodeNotFound, "alias %s not found", name)
	}
	return nil
}

func (r *AliasRepo) DeleteForTarget(ctx context.Context, target domain.AgentName) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM aliases WHERE target=?`, target)
	return internal(err)
}

func (r *AliasRepo) List(ctx context.Context) ([]*domain.Alias, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT name, target, created_at, updated_at FROM aliases ORDER BY name`)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	var out []*domain.Alias
	for rows.Next() {
		var (
			a                domain.Alias
			created, updated int64
		)
		if err := rows.Scan(&a.Name, &a.Target, &created, &updated); err != nil {
			return nil, internal(err)
		}
		a.CreatedAt, a.UpdatedAt = fromMS(created), fromMS(updated)
		out = append(out, &a)
	}
	return out, internal(rows.Err())
}
