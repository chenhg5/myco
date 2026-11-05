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

const agentCols = `id, name, description, capabilities, delivery, allow_from, token_hash, created_at, last_seen_at`

func (r *AgentRepo) Create(ctx context.Context, a *domain.Agent) error {
	caps, allow := encodeLists(a)
	_, err := r.db.ExecContext(ctx, `INSERT INTO agents (`+agentCols+`) VALUES (?,?,?,?,?,?,?,?,?)`,
		a.ID, a.Name, a.Description, caps, a.Delivery, allow, a.TokenHash, ms(a.CreatedAt), ms(a.LastSeenAt))
	if isUniqueViolation(err) {
		return domain.Errf(domain.CodeConflict, "agent %s already exists", a.Name)
	}
	return internal(err)
}

func (r *AgentRepo) Save(ctx context.Context, a *domain.Agent) error {
	caps, allow := encodeLists(a)
	res, err := r.db.ExecContext(ctx, `UPDATE agents SET description=?, capabilities=?, delivery=?, allow_from=?, token_hash=? WHERE name=?`,
		a.Description, caps, a.Delivery, allow, a.TokenHash, a.Name)
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

type scanner interface{ Scan(dest ...any) error }

func scanAgent(s scanner) (*domain.Agent, error) {
	var (
		a                 domain.Agent
		caps, allow       string
		created, lastSeen int64
	)
	if err := s.Scan(&a.ID, &a.Name, &a.Description, &caps, &a.Delivery, &allow, &a.TokenHash, &created, &lastSeen); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(caps), &a.Capabilities)
	_ = json.Unmarshal([]byte(allow), &a.AllowFrom)
	a.CreatedAt, a.LastSeenAt = fromMS(created), fromMS(lastSeen)
	return &a, nil
}

func encodeLists(a *domain.Agent) (string, string) {
	caps, _ := json.Marshal(nonNil(a.Capabilities))
	allow, _ := json.Marshal(nonNil(a.AllowFrom))
	return string(caps), string(allow)
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
