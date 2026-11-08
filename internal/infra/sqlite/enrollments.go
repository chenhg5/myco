package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/chenhg5/myco/internal/domain"
)

// EnrollmentRepo implements domain.EnrollmentRepository.
type EnrollmentRepo struct{ db *sql.DB }

// NewEnrollmentRepo returns a SQLite-backed enrollment repository.
func NewEnrollmentRepo(db *sql.DB) *EnrollmentRepo { return &EnrollmentRepo{db: db} }

var _ domain.EnrollmentRepository = (*EnrollmentRepo)(nil)

const enrCols = `id, token_hash, name_prefix, harness, allow_from, capabilities, note, max_uses, uses, expires_at, revoked_at, created_at, labels, kind, parent_id, machine, owner`

func (r *EnrollmentRepo) Create(ctx context.Context, e *domain.Enrollment) error {
	allow, _ := json.Marshal(nonNil(e.AllowFrom))
	caps, _ := json.Marshal(nonNil(e.Capabilities))
	_, err := r.db.ExecContext(ctx, `INSERT INTO enrollments (`+enrCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		e.ID, e.TokenHash, e.NamePrefix, e.Harness, string(allow), string(caps), e.Note, e.MaxUses, e.Uses,
		ms(e.ExpiresAt), ms(e.RevokedAt), ms(e.CreatedAt), labelsJSON(e.Labels), e.Kind, e.ParentID, e.Machine, e.Owner)
	if isUniqueViolation(err) {
		return domain.Errf(domain.CodeConflict, "enrollment already exists")
	}
	return internal(err)
}

func (r *EnrollmentRepo) Save(ctx context.Context, e *domain.Enrollment) error {
	res, err := r.db.ExecContext(ctx, `UPDATE enrollments SET revoked_at=?, uses=?, token_hash=? WHERE id=?`, ms(e.RevokedAt), e.Uses, e.TokenHash, e.ID)
	if err != nil {
		return internal(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.Errf(domain.CodeNotFound, "enrollment %s not found", e.ID)
	}
	return nil
}

func (r *EnrollmentRepo) ConsumeUse(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `UPDATE enrollments SET uses=uses+1 WHERE id=? AND (max_uses=0 OR uses<max_uses)`, id)
	if err != nil {
		return internal(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.Errf(domain.CodePermissionDenied, "enrollment %s has no uses left", id)
	}
	return nil
}

func (r *EnrollmentRepo) Get(ctx context.Context, id string) (*domain.Enrollment, error) {
	return r.one(ctx, `WHERE id=?`, id)
}

func (r *EnrollmentRepo) GetByTokenHash(ctx context.Context, hash string) (*domain.Enrollment, error) {
	return r.one(ctx, `WHERE token_hash=?`, hash)
}

func (r *EnrollmentRepo) List(ctx context.Context) ([]*domain.Enrollment, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+enrCols+` FROM enrollments ORDER BY created_at`)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	var out []*domain.Enrollment
	for rows.Next() {
		e, err := scanEnrollment(rows)
		if err != nil {
			return nil, internal(err)
		}
		out = append(out, e)
	}
	return out, internal(rows.Err())
}

func (r *EnrollmentRepo) one(ctx context.Context, where string, args ...any) (*domain.Enrollment, error) {
	e, err := scanEnrollment(r.db.QueryRowContext(ctx, `SELECT `+enrCols+` FROM enrollments `+where, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.Errf(domain.CodeNotFound, "enrollment not found").WithSuggestion("list them with: myco enrollment list")
	}
	return e, internal(err)
}

func scanEnrollment(s scanner) (*domain.Enrollment, error) {
	var (
		e                         domain.Enrollment
		allow, caps, labels       string
		expires, revoked, created int64
	)
	if err := s.Scan(&e.ID, &e.TokenHash, &e.NamePrefix, &e.Harness, &allow, &caps, &e.Note,
		&e.MaxUses, &e.Uses, &expires, &revoked, &created, &labels, &e.Kind, &e.ParentID, &e.Machine, &e.Owner); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(allow), &e.AllowFrom)
	_ = json.Unmarshal([]byte(caps), &e.Capabilities)
	_ = json.Unmarshal([]byte(labels), &e.Labels)
	e.ExpiresAt, e.RevokedAt, e.CreatedAt = fromMS(expires), fromMS(revoked), fromMS(created)
	return &e, nil
}
