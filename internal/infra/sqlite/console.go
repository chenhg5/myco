package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/chenhg5/myco/internal/domain"
)

// ConsoleTokenRepo implements domain.ConsoleTokenRepository.
type ConsoleTokenRepo struct{ db *sql.DB }

// NewConsoleTokenRepo returns a SQLite-backed console token repository.
func NewConsoleTokenRepo(db *sql.DB) *ConsoleTokenRepo { return &ConsoleTokenRepo{db: db} }

var _ domain.ConsoleTokenRepository = (*ConsoleTokenRepo)(nil)

const consoleCols = `id, name, token_hash, created_at, last_used_at, revoked_at, member_id`

func (r *ConsoleTokenRepo) Create(ctx context.Context, t *domain.ConsoleToken) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO console_tokens (`+consoleCols+`) VALUES (?,?,?,?,?,?,?)`,
		t.ID, t.Name, t.TokenHash, ms(t.CreatedAt), ms(t.LastUsedAt), ms(t.RevokedAt), t.MemberID)
	if isUniqueViolation(err) {
		return domain.Errf(domain.CodeConflict, "console token already exists")
	}
	return internal(err)
}

func (r *ConsoleTokenRepo) Save(ctx context.Context, t *domain.ConsoleToken) error {
	res, err := r.db.ExecContext(ctx, `UPDATE console_tokens SET last_used_at=?, revoked_at=?, member_id=? WHERE id=?`,
		ms(t.LastUsedAt), ms(t.RevokedAt), t.MemberID, t.ID)
	if err != nil {
		return internal(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.Errf(domain.CodeNotFound, "console token %s not found", t.ID)
	}
	return nil
}

func (r *ConsoleTokenRepo) Get(ctx context.Context, id string) (*domain.ConsoleToken, error) {
	return r.one(ctx, `WHERE id=?`, id)
}

func (r *ConsoleTokenRepo) GetByTokenHash(ctx context.Context, hash string) (*domain.ConsoleToken, error) {
	return r.one(ctx, `WHERE token_hash=?`, hash)
}

func (r *ConsoleTokenRepo) List(ctx context.Context) ([]*domain.ConsoleToken, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+consoleCols+` FROM console_tokens ORDER BY created_at`)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	var out []*domain.ConsoleToken
	for rows.Next() {
		t, err := scanConsoleToken(rows)
		if err != nil {
			return nil, internal(err)
		}
		out = append(out, t)
	}
	return out, internal(rows.Err())
}

func (r *ConsoleTokenRepo) one(ctx context.Context, where string, args ...any) (*domain.ConsoleToken, error) {
	t, err := scanConsoleToken(r.db.QueryRowContext(ctx, `SELECT `+consoleCols+` FROM console_tokens `+where, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.Errf(domain.CodeNotFound, "console token not found").WithSuggestion("list them with: myco console token list")
	}
	return t, internal(err)
}

func scanConsoleToken(s scanner) (*domain.ConsoleToken, error) {
	var (
		t                      domain.ConsoleToken
		created, used, revoked int64
	)
	if err := s.Scan(&t.ID, &t.Name, &t.TokenHash, &created, &used, &revoked, &t.MemberID); err != nil {
		return nil, err
	}
	t.CreatedAt, t.LastUsedAt, t.RevokedAt = fromMS(created), fromMS(used), fromMS(revoked)
	return &t, nil
}
