package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/chenhg5/myco/internal/domain"
)

// SuperagentRepo implements domain.SuperagentRepository.
type SuperagentRepo struct{ db *sql.DB }

// NewSuperagentRepo returns a SQLite-backed superagent repository.
func NewSuperagentRepo(db *sql.DB) *SuperagentRepo { return &SuperagentRepo{db: db} }

var _ domain.SuperagentRepository = (*SuperagentRepo)(nil)

func (r *SuperagentRepo) Of(ctx context.Context, member string) (domain.AgentName, error) {
	var a string
	err := r.db.QueryRowContext(ctx, `SELECT agent FROM superagents WHERE member_id=?`, member).Scan(&a)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return domain.AgentName(a), internal(err)
}

func (r *SuperagentRepo) OwnerOf(ctx context.Context, agent domain.AgentName) (string, error) {
	var m string
	err := r.db.QueryRowContext(ctx, `SELECT member_id FROM superagents WHERE agent=?`, string(agent)).Scan(&m)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return m, internal(err)
}

func (r *SuperagentRepo) Set(ctx context.Context, member string, agent domain.AgentName, at time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return internal(err)
	}
	defer tx.Rollback()
	// An agent is one member's superagent at most.
	if _, err := tx.ExecContext(ctx, `DELETE FROM superagents WHERE agent=? OR member_id=?`, string(agent), member); err != nil {
		return internal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO superagents (member_id, agent, set_at) VALUES (?,?,?)`, member, string(agent), ms(at)); err != nil {
		return internal(err)
	}
	return internal(tx.Commit())
}

func (r *SuperagentRepo) Clear(ctx context.Context, member string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM superagents WHERE member_id=?`, member)
	return internal(err)
}

// ApprovalRepo implements domain.ApprovalRepository.
type ApprovalRepo struct{ db *sql.DB }

// NewApprovalRepo returns a SQLite-backed approval repository.
func NewApprovalRepo(db *sql.DB) *ApprovalRepo { return &ApprovalRepo{db: db} }

var _ domain.ApprovalRepository = (*ApprovalRepo)(nil)

const approvalCols = `id, owner, agent, what, reason, method, path, body, content_type, status, result, result_body, decided_by, created_at, decided_at`

func (r *ApprovalRepo) Create(ctx context.Context, a *domain.Approval) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO approvals (`+approvalCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		a.ID, a.Owner, string(a.Agent), a.What, a.Reason, a.Method, a.Path, a.Body, a.ContentType, a.Status, a.Result, a.ResultBody,
		a.DecidedBy, ms(a.CreatedAt), ms(a.DecidedAt))
	return internal(err)
}

func (r *ApprovalRepo) Save(ctx context.Context, a *domain.Approval) error {
	_, err := r.db.ExecContext(ctx, `UPDATE approvals SET status=?, result=?, result_body=?, decided_by=?, decided_at=? WHERE id=?`,
		a.Status, a.Result, a.ResultBody, a.DecidedBy, ms(a.DecidedAt), a.ID)
	return internal(err)
}

func scanApproval(s scanner) (*domain.Approval, error) {
	var (
		a                domain.Approval
		agent            string
		created, decided int64
	)
	if err := s.Scan(&a.ID, &a.Owner, &agent, &a.What, &a.Reason, &a.Method, &a.Path, &a.Body, &a.ContentType, &a.Status, &a.Result,
		&a.ResultBody, &a.DecidedBy, &created, &decided); err != nil {
		return nil, err
	}
	a.Agent = domain.AgentName(agent)
	a.CreatedAt, a.DecidedAt = fromMS(created), fromMS(decided)
	return &a, nil
}

func (r *ApprovalRepo) Get(ctx context.Context, id string) (*domain.Approval, error) {
	a, err := scanApproval(r.db.QueryRowContext(ctx, `SELECT `+approvalCols+` FROM approvals WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.Errf(domain.CodeNotFound, "approval %s not found", id)
	}
	return a, internal(err)
}

func (r *ApprovalRepo) list(ctx context.Context, q string, args ...any) ([]*domain.Approval, error) {
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	var out []*domain.Approval
	for rows.Next() {
		a, err := scanApproval(rows)
		if err != nil {
			return nil, internal(err)
		}
		out = append(out, a)
	}
	return out, internal(rows.Err())
}

func (r *ApprovalRepo) List(ctx context.Context, owner, status string, limit int) ([]*domain.Approval, error) {
	if limit <= 0 {
		limit = 50
	}
	if status != "" {
		return r.list(ctx, `SELECT `+approvalCols+` FROM approvals WHERE owner=? AND status=? ORDER BY created_at DESC LIMIT ?`, owner, status, limit)
	}
	return r.list(ctx, `SELECT `+approvalCols+` FROM approvals WHERE owner=? ORDER BY created_at DESC LIMIT ?`, owner, limit)
}

func (r *ApprovalRepo) ListByAgent(ctx context.Context, agent domain.AgentName, limit int) ([]*domain.Approval, error) {
	if limit <= 0 {
		limit = 50
	}
	return r.list(ctx, `SELECT `+approvalCols+` FROM approvals WHERE agent=? ORDER BY created_at DESC LIMIT ?`, string(agent), limit)
}
