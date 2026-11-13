package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/chenhg5/myco/internal/domain"
)

type ResumeRepo struct{ db *sql.DB }

func NewResumeRepo(db *sql.DB) *ResumeRepo { return &ResumeRepo{db: db} }

var _ domain.ResumeRepository = (*ResumeRepo)(nil)

func (r *ResumeRepo) Get(ctx context.Context, agent domain.AgentName) (*domain.ResumeCredential, error) {
	return r.get(ctx, `agent=?`, agent)
}

func (r *ResumeRepo) GetByTokenHash(ctx context.Context, hash string) (*domain.ResumeCredential, error) {
	return r.get(ctx, `token_hash=?`, hash)
}

func (r *ResumeRepo) get(ctx context.Context, where string, value any) (*domain.ResumeCredential, error) {
	var c domain.ResumeCredential
	var created, used int64
	err := r.db.QueryRowContext(ctx, `SELECT agent, token_hash, created_at, last_used_at, last_used_env FROM agent_resumes WHERE `+where, value).
		Scan(&c.Agent, &c.TokenHash, &created, &used, &c.LastUsedEnv)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.Errf(domain.CodeNotFound, "resume link not found")
	}
	if err != nil {
		return nil, internal(err)
	}
	c.CreatedAt, c.LastUsedAt = fromMS(created), fromMS(used)
	return &c, nil
}

func (r *ResumeRepo) Put(ctx context.Context, c *domain.ResumeCredential) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO agent_resumes(agent, token_hash, created_at, last_used_at, last_used_env)
		VALUES(?,?,?,?,?) ON CONFLICT(agent) DO UPDATE SET token_hash=excluded.token_hash, created_at=excluded.created_at,
		last_used_at=0, last_used_env=''`, c.Agent, c.TokenHash, ms(c.CreatedAt), 0, "")
	return internal(err)
}

func (r *ResumeRepo) PutIfAbsent(ctx context.Context, c *domain.ResumeCredential) (bool, error) {
	res, err := r.db.ExecContext(ctx, `INSERT INTO agent_resumes(agent, token_hash, created_at, last_used_at, last_used_env)
		VALUES(?,?,?,?,?) ON CONFLICT(agent) DO NOTHING`, c.Agent, c.TokenHash, ms(c.CreatedAt), 0, "")
	if err != nil {
		return false, internal(err)
	}
	n, err := res.RowsAffected()
	return n == 1, internal(err)
}

func (r *ResumeRepo) Delete(ctx context.Context, agent domain.AgentName) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM agent_resumes WHERE agent=?`, agent)
	return internal(err)
}

func (r *ResumeRepo) Use(ctx context.Context, hash string, b domain.Binding, agentTokenHash string, at time.Time) (domain.AgentName, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", internal(err)
	}
	defer tx.Rollback()
	var name domain.AgentName
	if err := tx.QueryRowContext(ctx, `SELECT agent FROM agent_resumes WHERE token_hash=?`, hash).Scan(&name); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", domain.Errf(domain.CodeUnauthenticated, "resume link is invalid or revoked")
		}
		return "", internal(err)
	}
	var other string
	err = tx.QueryRowContext(ctx, `SELECT name FROM agents WHERE harness=? AND session_id=? AND name<>? LIMIT 1`, b.Harness, b.SessionID, name).Scan(&other)
	if err == nil {
		return "", domain.Errf(domain.CodeConflict, "session is already bound to agent %s", other)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", internal(err)
	}
	res, err := tx.ExecContext(ctx, `UPDATE agents SET harness=?, session_id=?, environment=?, workdir=?, token_hash=?, last_seen_at=?, machine_id='', runtime='{}' WHERE name=?`,
		b.Harness, b.SessionID, b.Environment, b.Workdir, agentTokenHash, ms(at), name)
	if err != nil {
		return "", internal(err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return "", domain.Errf(domain.CodeUnauthenticated, "resume link is invalid or revoked")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE agent_resumes SET last_used_at=?, last_used_env=? WHERE agent=?`, ms(at), b.Environment, name); err != nil {
		return "", internal(err)
	}
	return name, internal(tx.Commit())
}
