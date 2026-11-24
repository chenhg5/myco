package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/chenhg5/myco/internal/domain"
)

// usageSchema is an additive, idempotent migration for existing workspaces.
const usageSchema = `
CREATE TABLE IF NOT EXISTS usage_daily (
 day TEXT NOT NULL,agent TEXT NOT NULL,project TEXT NOT NULL DEFAULT '',model TEXT NOT NULL DEFAULT '',source TEXT NOT NULL,
 calls INTEGER NOT NULL DEFAULT 0,input_tokens INTEGER NOT NULL DEFAULT 0,cache_write_tokens INTEGER NOT NULL DEFAULT 0,
 cache_read_tokens INTEGER NOT NULL DEFAULT 0,output_tokens INTEGER NOT NULL DEFAULT 0,cost_usd REAL NOT NULL DEFAULT 0,
 PRIMARY KEY(day,agent,project,model,source));
CREATE TABLE IF NOT EXISTS usage_keys (agent TEXT NOT NULL,key TEXT NOT NULL,at INTEGER NOT NULL,PRIMARY KEY(agent,key));
CREATE INDEX IF NOT EXISTS usage_keys_at ON usage_keys(at);
`

type UsageRepo struct{ db *sql.DB }

func NewUsageRepo(db *sql.DB) *UsageRepo { return &UsageRepo{db} }
func (r *UsageRepo) Record(ctx context.Context, agent, source string, reports []domain.UsageReport, now time.Time) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, internal(err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "DELETE FROM usage_keys WHERE at < ?", ms(now.AddDate(0, 0, -30))); err != nil {
		return 0, internal(err)
	}
	n := 0
	for _, p := range reports {
		if p.Key != "" {
			result, err := tx.ExecContext(ctx, "INSERT INTO usage_keys(agent,key,at) VALUES(?,?,?) ON CONFLICT DO NOTHING", agent, p.Key, ms(now))
			if err != nil {
				return 0, internal(err)
			}
			added, err := result.RowsAffected()
			if err != nil {
				return 0, internal(err)
			}
			if added == 0 {
				continue
			}
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO usage_daily(day,agent,project,model,source,calls,input_tokens,cache_write_tokens,cache_read_tokens,output_tokens,cost_usd)
 VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(day,agent,project,model,source) DO UPDATE SET
 calls=calls+excluded.calls,input_tokens=input_tokens+excluded.input_tokens,cache_write_tokens=cache_write_tokens+excluded.cache_write_tokens,
 cache_read_tokens=cache_read_tokens+excluded.cache_read_tokens,output_tokens=output_tokens+excluded.output_tokens,cost_usd=cost_usd+excluded.cost_usd`,
			p.Day, agent, p.Project, p.Model, source, p.Calls, p.InputTokens, p.CacheWriteTokens, p.CacheReadTokens, p.OutputTokens, p.CostUSD)
		if err != nil {
			return 0, internal(err)
		}
		n++
	}
	if err = tx.Commit(); err != nil {
		return 0, internal(err)
	}
	return n, nil
}
func (r *UsageRepo) List(ctx context.Context, f domain.UsageFilter) ([]domain.UsageRow, error) {
	query := `SELECT u.day,u.agent,u.project,u.model,u.source,u.calls,u.input_tokens,u.cache_write_tokens,u.cache_read_tokens,u.output_tokens,u.cost_usd FROM usage_daily u WHERE u.day>=? AND u.day<=?`
	args := []any{f.From, f.To}
	if f.Project != "" {
		if f.Project == "-" {
			f.Project = ""
		}
		query += " AND u.project=?"
		args = append(args, f.Project)
	}
	if f.Owner != "" {
		query += " AND EXISTS(SELECT 1 FROM agents a WHERE a.name=u.agent AND a.owner=?)"
		args = append(args, f.Owner)
	}
	query += " ORDER BY u.day,u.agent,u.project,u.model,u.source"
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	out := []domain.UsageRow{}
	for rows.Next() {
		var p domain.UsageRow
		if err := rows.Scan(&p.Day, &p.Agent, &p.Project, &p.Model, &p.Source, &p.Calls, &p.InputTokens, &p.CacheWriteTokens, &p.CacheReadTokens, &p.OutputTokens, &p.CostUSD); err != nil {
			return nil, internal(err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
