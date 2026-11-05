package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/chenhg5/myco/internal/domain"
)

// AgentRepo implements domain.AgentRepository.
type AgentRepo struct{ db *sql.DB }

// NewAgentRepo returns a SQLite-backed agent repository.
func NewAgentRepo(db *sql.DB) *AgentRepo { return &AgentRepo{db: db} }

var _ domain.AgentRepository = (*AgentRepo)(nil)

const agentCols = `id, name, description, capabilities, delivery, allow_from, token_hash, created_at, last_seen_at,
  harness, session_id, environment, workdir, delivery_url, delivery_secret, delivery_params, enrolled_by`

func (r *AgentRepo) Create(ctx context.Context, a *domain.Agent) error {
	caps, allow, params := encodeLists(a)
	b, d := a.Binding, a.Delivery
	_, err := r.db.ExecContext(ctx, `INSERT INTO agents (`+agentCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		a.ID, a.Name, a.Description, caps, d.Mode, allow, a.TokenHash, ms(a.CreatedAt), ms(a.LastSeenAt),
		b.Harness, b.SessionID, b.Environment, b.Workdir, d.URL, d.Secret, params, a.EnrolledBy)
	if isUniqueViolation(err) {
		return domain.Errf(domain.CodeConflict, "agent %s already exists", a.Name)
	}
	return internal(err)
}

func (r *AgentRepo) Save(ctx context.Context, a *domain.Agent) error {
	caps, allow, params := encodeLists(a)
	b, d := a.Binding, a.Delivery
	res, err := r.db.ExecContext(ctx, `UPDATE agents SET description=?, capabilities=?, delivery=?, allow_from=?, token_hash=?,
		harness=?, session_id=?, environment=?, workdir=?, delivery_url=?, delivery_secret=?, delivery_params=? WHERE name=?`,
		a.Description, caps, d.Mode, allow, a.TokenHash,
		b.Harness, b.SessionID, b.Environment, b.Workdir, d.URL, d.Secret, params, a.Name)
	if err != nil {
		return internal(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.Errf(domain.CodeNotFound, "agent %s not found", a.Name)
	}
	return nil
}

func (r *AgentRepo) Get(ctx context.Context, name domain.AgentName) (*domain.Agent, error) {
	a, err := scanAgent(r.db.QueryRowContext(ctx, `SELECT `+agentCols+` FROM agents WHERE name=?`, name))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.Errf(domain.CodeNotFound, "agent %s not found", name).
			WithSuggestion("list agents with: myco agent list")
	}
	return a, internal(err)
}

func (r *AgentRepo) GetByTokenHash(ctx context.Context, hash string) (*domain.Agent, error) {
	a, err := scanAgent(r.db.QueryRowContext(ctx, `SELECT `+agentCols+` FROM agents WHERE token_hash=?`, hash))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.Errf(domain.CodeNotFound, "no agent for token")
	}
	return a, internal(err)
}

func (r *AgentRepo) List(ctx context.Context) ([]*domain.Agent, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+agentCols+` FROM agents ORDER BY name`)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	var out []*domain.Agent
	for rows.Next() {
		a, err := scanAgent(rows)
		if err != nil {
			return nil, internal(err)
		}
		out = append(out, a)
	}
	return out, internal(rows.Err())
}

func (r *AgentRepo) Delete(ctx context.Context, name domain.AgentName) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM agents WHERE name=?`, name)
	if err != nil {
		return internal(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.Errf(domain.CodeNotFound, "agent %s not found", name)
	}
	return nil
}

func (r *AgentRepo) Touch(ctx context.Context, name domain.AgentName, at time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE agents SET last_seen_at=? WHERE name=?`, ms(at), name)
	return internal(err)
}

func (r *AgentRepo) FindBySession(ctx context.Context, harness, sessionID string) (*domain.Agent, error) {
	a, err := scanAgent(r.db.QueryRowContext(ctx, `SELECT `+agentCols+` FROM agents WHERE session_id=? AND harness=? LIMIT 1`, sessionID, harness))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.Errf(domain.CodeNotFound, "no agent bound to session %s", sessionID)
	}
	return a, internal(err)
}

type scanner interface{ Scan(dest ...any) error }

func scanAgent(s scanner) (*domain.Agent, error) {
	var (
		a                   domain.Agent
		caps, allow, params string
		created, lastSeen   int64
	)
	b, d := &a.Binding, &a.Delivery
	if err := s.Scan(&a.ID, &a.Name, &a.Description, &caps, &d.Mode, &allow, &a.TokenHash, &created, &lastSeen,
		&b.Harness, &b.SessionID, &b.Environment, &b.Workdir, &d.URL, &d.Secret, &params, &a.EnrolledBy); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(caps), &a.Capabilities)
	_ = json.Unmarshal([]byte(allow), &a.AllowFrom)
	_ = json.Unmarshal([]byte(params), &d.Params)
	a.CreatedAt, a.LastSeenAt = fromMS(created), fromMS(lastSeen)
	return &a, nil
}

func encodeLists(a *domain.Agent) (caps, allow, params string) {
	c, _ := json.Marshal(nonNil(a.Capabilities))
	al, _ := json.Marshal(nonNil(a.AllowFrom))
	p := a.Delivery.Params
	if p == nil {
		p = map[string]string{}
	}
	pj, _ := json.Marshal(p)
	return string(c), string(al), string(pj)
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
