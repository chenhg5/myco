package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/chenhg5/myco/internal/domain"
)

// MemberRepo implements domain.MemberRepository.
type MemberRepo struct{ db *sql.DB }

// NewMemberRepo returns a SQLite-backed member repository.
func NewMemberRepo(db *sql.DB) *MemberRepo { return &MemberRepo{db: db} }

var _ domain.MemberRepository = (*MemberRepo)(nil)

const memberCols = `id, name, display_name, role, created_at, removed_at`

func (r *MemberRepo) Create(ctx context.Context, m *domain.Member) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO members (`+memberCols+`) VALUES (?,?,?,?,?,?)`,
		m.ID, m.Name, m.DisplayName, string(m.Role), ms(m.CreatedAt), ms(m.RemovedAt))
	if isUniqueViolation(err) {
		return domain.Errf(domain.CodeConflict, "member %s already exists", m.Name)
	}
	return internal(err)
}

func (r *MemberRepo) Save(ctx context.Context, m *domain.Member) error {
	res, err := r.db.ExecContext(ctx, `UPDATE members SET name=?, display_name=?, role=?, removed_at=? WHERE id=?`,
		m.Name, m.DisplayName, string(m.Role), ms(m.RemovedAt), m.ID)
	if isUniqueViolation(err) {
		return domain.Errf(domain.CodeConflict, "member %s already exists", m.Name)
	}
	if err != nil {
		return internal(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.Errf(domain.CodeNotFound, "member %s not found", m.ID)
	}
	return nil
}

func (r *MemberRepo) AssignUnowned(ctx context.Context, memberID string) error {
	for _, q := range []string{
		`UPDATE agents SET owner=? WHERE owner=''`,
		`UPDATE machines SET owner=? WHERE owner=''`,
		`UPDATE enrollments SET owner=? WHERE owner=''`,
		`UPDATE console_tokens SET member_id=? WHERE member_id=''`,
	} {
		if _, err := r.db.ExecContext(ctx, q, memberID); err != nil {
			return internal(err)
		}
	}
	return nil
}

func (r *MemberRepo) Get(ctx context.Context, id string) (*domain.Member, error) {
	return r.one(ctx, `WHERE id=?`, id)
}

func (r *MemberRepo) GetByName(ctx context.Context, name string) (*domain.Member, error) {
	return r.one(ctx, `WHERE name=?`, name)
}

func (r *MemberRepo) List(ctx context.Context) ([]*domain.Member, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+memberCols+` FROM members ORDER BY created_at`)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	var out []*domain.Member
	for rows.Next() {
		m, err := scanMember(rows)
		if err != nil {
			return nil, internal(err)
		}
		out = append(out, m)
	}
	return out, internal(rows.Err())
}

func (r *MemberRepo) one(ctx context.Context, where string, args ...any) (*domain.Member, error) {
	m, err := scanMember(r.db.QueryRowContext(ctx, `SELECT `+memberCols+` FROM members `+where, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.Errf(domain.CodeNotFound, "member not found").WithSuggestion("list them with: myco member list")
	}
	return m, internal(err)
}

func scanMember(s scanner) (*domain.Member, error) {
	var (
		m                domain.Member
		role             string
		created, removed int64
	)
	if err := s.Scan(&m.ID, &m.Name, &m.DisplayName, &role, &created, &removed); err != nil {
		return nil, err
	}
	m.Role, m.CreatedAt, m.RemovedAt = domain.Role(role), fromMS(created), fromMS(removed)
	return &m, nil
}

// AuditRepo implements domain.AuditRepository (append-only).
type AuditRepo struct{ db *sql.DB }

// NewAuditRepo returns a SQLite-backed audit log.
func NewAuditRepo(db *sql.DB) *AuditRepo { return &AuditRepo{db: db} }

var _ domain.AuditRepository = (*AuditRepo)(nil)

func (r *AuditRepo) Append(ctx context.Context, e *domain.AuditEntry) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO audit_log (id, at, actor, actor_name, actor_kind, action, target, detail) VALUES (?,?,?,?,?,?,?,?)`,
		e.ID, ms(e.At), e.Actor, e.ActorName, e.ActorKind, e.Action, e.Target, e.Detail)
	return internal(err)
}

func (r *AuditRepo) List(ctx context.Context, f domain.AuditFilter) ([]*domain.AuditEntry, error) {
	var where []string
	var args []any
	if !f.Before.IsZero() {
		where, args = append(where, "at < ?"), append(args, ms(f.Before))
	}
	if f.Actor != "" {
		where, args = append(where, "actor_name = ?"), append(args, f.Actor)
	}
	if f.Target != "" {
		where, args = append(where, "target = ?"), append(args, f.Target)
	}
	q := `SELECT id, at, actor, actor_name, actor_kind, action, target, detail FROM audit_log`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q += " ORDER BY at DESC, seq DESC LIMIT ?"
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	var out []*domain.AuditEntry
	for rows.Next() {
		var e domain.AuditEntry
		var at int64
		if err := rows.Scan(&e.ID, &at, &e.Actor, &e.ActorName, &e.ActorKind, &e.Action, &e.Target, &e.Detail); err != nil {
			return nil, internal(err)
		}
		e.At = fromMS(at)
		out = append(out, &e)
	}
	return out, internal(rows.Err())
}
