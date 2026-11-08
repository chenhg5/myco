package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

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

// ConsolePairingRepo implements domain.ConsolePairingRepository.
type ConsolePairingRepo struct{ db *sql.DB }

// NewConsolePairingRepo returns a SQLite-backed pairing code store.
func NewConsolePairingRepo(db *sql.DB) *ConsolePairingRepo { return &ConsolePairingRepo{db: db} }

var _ domain.ConsolePairingRepository = (*ConsolePairingRepo)(nil)

func (r *ConsolePairingRepo) Create(ctx context.Context, p *domain.ConsolePairing) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO console_pairings (id, code_hash, member_id, created_at, expires_at, used_at) VALUES (?,?,?,?,?,?)`,
		p.ID, p.CodeHash, p.MemberID, ms(p.CreatedAt), ms(p.ExpiresAt), ms(p.UsedAt))
	return internal(err)
}

func (r *ConsolePairingRepo) Use(ctx context.Context, hash string, now time.Time) (*domain.ConsolePairing, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE console_pairings SET used_at=? WHERE code_hash=? AND used_at=0 AND expires_at>?`, ms(now), hash, ms(now))
	if err != nil {
		return nil, internal(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, domain.Errf(domain.CodeNotFound, "this link has expired or was already used").
			WithSuggestion("ask for a new one (they work once, within 24 hours)")
	}
	var p domain.ConsolePairing
	var created, expires, used int64
	err = r.db.QueryRowContext(ctx, `SELECT id, code_hash, member_id, created_at, expires_at, used_at FROM console_pairings WHERE code_hash=?`, hash).
		Scan(&p.ID, &p.CodeHash, &p.MemberID, &created, &expires, &used)
	if err != nil {
		return nil, internal(err)
	}
	p.CreatedAt, p.ExpiresAt, p.UsedAt = fromMS(created), fromMS(expires), fromMS(used)
	return &p, nil
}
