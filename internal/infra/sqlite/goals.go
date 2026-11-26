package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/myco-ai/myco/internal/domain"
)

const goalSchema = `CREATE TABLE IF NOT EXISTS goals(id TEXT PRIMARY KEY,agent TEXT NOT NULL,state TEXT NOT NULL,data TEXT NOT NULL);
CREATE UNIQUE INDEX IF NOT EXISTS idx_goals_open_agent ON goals(agent) WHERE state IN ('active','paused');`

type GoalRepo struct{ db *sql.DB }

func NewGoalRepo(db *sql.DB) *GoalRepo     { return &GoalRepo{db} }
func goalJSON(g *domain.GoalRecord) string { b, _ := json.Marshal(g); return string(b) }
func (r *GoalRepo) Create(ctx context.Context, g *domain.GoalRecord) error {
	_, e := r.db.ExecContext(ctx, `INSERT INTO goals(id,agent,state,data) VALUES(?,?,?,?)`, g.Goal.ID, g.Goal.Agent, g.Goal.State, goalJSON(g))
	if isUniqueViolation(e) {
		return domain.Errf(domain.CodeConflict, "this agent already has an active or paused goal")
	}
	return internal(e)
}
func loadGoal(ctx context.Context, x interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id string) (*domain.GoalRecord, error) {
	var raw string
	e := x.QueryRowContext(ctx, `SELECT data FROM goals WHERE id=?`, id).Scan(&raw)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, domain.Errf(domain.CodeNotFound, "goal not found")
	}
	if e != nil {
		return nil, internal(e)
	}
	var g domain.GoalRecord
	e = json.Unmarshal([]byte(raw), &g)
	return &g, internal(e)
}
func (r *GoalRepo) Get(ctx context.Context, id string) (*domain.GoalRecord, error) {
	return loadGoal(ctx, r.db, id)
}
func (r *GoalRepo) List(ctx context.Context, open bool) ([]*domain.GoalRecord, error) {
	q := `SELECT data FROM goals`
	if open {
		q += ` WHERE state IN ('active','paused')`
	}
	q += ` ORDER BY rowid DESC`
	rows, e := r.db.QueryContext(ctx, q)
	if e != nil {
		return nil, internal(e)
	}
	defer rows.Close()
	out := []*domain.GoalRecord{}
	for rows.Next() {
		var raw string
		if e = rows.Scan(&raw); e != nil {
			return nil, internal(e)
		}
		var g domain.GoalRecord
		if e = json.Unmarshal([]byte(raw), &g); e != nil {
			return nil, internal(e)
		}
		out = append(out, &g)
	}
	return out, internal(rows.Err())
}
func (r *GoalRepo) Mutate(ctx context.Context, id string, fn domain.GoalMutation) (*domain.GoalRecord, []*domain.Message, error) {
	tx, e := r.db.BeginTx(ctx, nil)
	if e != nil {
		return nil, nil, internal(e)
	}
	defer tx.Rollback()
	g, e := loadGoal(ctx, tx, id)
	if e != nil {
		return nil, nil, e
	}
	var m *domain.Message
	if len(g.Rounds) > 0 {
		m, e = NewMessageRepo(r.db).one(ctx, tx, `WHERE id=?`, g.Rounds[len(g.Rounds)-1].Message)
		if e != nil && !domain.IsCode(e, domain.CodeNotFound) {
			return nil, nil, e
		}
	}
	var n int
	e = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages WHERE to_agent=? AND status IN ('delivered','acked')`, g.Goal.Agent).Scan(&n)
	if e != nil {
		return nil, nil, internal(e)
	}
	before := goalJSON(g)
	created, e := fn(g, m, n > 0)
	if e != nil {
		return nil, nil, e
	}
	after := goalJSON(g)
	if before != after {
		_, e = tx.ExecContext(ctx, `UPDATE goals SET state=?,data=? WHERE id=?`, g.Goal.State, after, id)
		if e != nil {
			return nil, nil, internal(e)
		}
	}
	// Do not leave an undispatched goal turn queued after pause/termination.
	if g.Goal.State != "active" && m != nil && (m.Status == domain.StatusQueued || m.Status == domain.StatusDelivered) {
		m.Status = domain.StatusDone
		if e = updateMessage(ctx, tx, m); e != nil {
			return nil, nil, e
		}
	}
	for _, m := range created {
		if e = insertMessage(ctx, tx, m); e != nil {
			return nil, nil, e
		}
	}
	return g, created, internal(tx.Commit())
}
