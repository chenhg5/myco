package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/myco-ai/myco/internal/domain"
)

const watchSchema = `CREATE TABLE IF NOT EXISTS watches (
 id TEXT PRIMARY KEY, agent TEXT NOT NULL, project TEXT NOT NULL DEFAULT '',
 task TEXT NOT NULL DEFAULT '', state TEXT NOT NULL, deadline INTEGER NOT NULL,
 checks INTEGER NOT NULL DEFAULT 0, data TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_watches_active ON watches(state,deadline);
CREATE INDEX IF NOT EXISTS idx_watches_agent ON watches(agent,state);
CREATE UNIQUE INDEX IF NOT EXISTS idx_watches_held_task ON watches(task) WHERE task!='' AND state='watching';
`

type WatchRepo struct{ db *sql.DB }

func NewWatchRepo(db *sql.DB) *WatchRepo { return &WatchRepo{db} }
func watchJSON(w *domain.Watch) string   { b, _ := json.Marshal(w); return string(b) }
func insertWatch(ctx context.Context, x execer, w *domain.Watch) error {
	_, err := x.ExecContext(ctx, `INSERT INTO watches(id,agent,project,task,state,deadline,checks,data) VALUES(?,?,?,?,?,?,?,?)`, w.ID, w.Agent, w.Project, w.Task, w.State, ms(w.Deadline), w.Checks, watchJSON(w))
	if isUniqueViolation(err) {
		return domain.Errf(domain.CodeConflict, "a watch already exists for this held task")
	}
	return internal(err)
}
func (r *WatchRepo) Create(ctx context.Context, w *domain.Watch) error {
	return insertWatch(ctx, r.db, w)
}
func (r *WatchRepo) Get(ctx context.Context, id string) (*domain.Watch, error) {
	var data string
	err := r.db.QueryRowContext(ctx, `SELECT data FROM watches WHERE id=?`, id).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.Errf(domain.CodeNotFound, "watch %s not found", id)
	}
	if err != nil {
		return nil, internal(err)
	}
	var w domain.Watch
	if err = json.Unmarshal([]byte(data), &w); err != nil {
		return nil, internal(err)
	}
	return &w, nil
}
func (r *WatchRepo) List(ctx context.Context, active bool) ([]*domain.Watch, error) {
	q := `SELECT data FROM watches`
	if active {
		q += ` WHERE state='watching'`
	}
	q += ` ORDER BY rowid DESC`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	out := []*domain.Watch{}
	for rows.Next() {
		var data string
		if err = rows.Scan(&data); err != nil {
			return nil, internal(err)
		}
		var w domain.Watch
		if err = json.Unmarshal([]byte(data), &w); err != nil {
			return nil, internal(err)
		}
		out = append(out, &w)
	}
	return out, internal(rows.Err())
}
func (r *WatchRepo) Hold(ctx context.Context, id string, w *domain.Watch, fn func(*domain.Message) error) (*domain.Message, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, internal(err)
	}
	defer tx.Rollback()
	m, err := NewMessageRepo(r.db).one(ctx, tx, `WHERE id=?`, id)
	if err != nil {
		return nil, err
	}
	if err = fn(m); err != nil {
		return nil, err
	}
	var existing string
	err = tx.QueryRowContext(ctx, `SELECT data FROM watches WHERE task=? AND state='watching'`, id).Scan(&existing)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, internal(err)
	}
	if existing == "" {
		if err = insertWatch(ctx, tx, w); err != nil {
			return nil, err
		}
	} else {
		var previous domain.Watch
		if err = json.Unmarshal([]byte(existing), &previous); err != nil {
			return nil, internal(err)
		}
		if previous.Target != w.Target || previous.Kind != w.Kind {
			return nil, domain.Errf(domain.CodeConflict, "this task already has a different active watch; stop it first")
		}
		*w = previous
	}
	if err = updateMessage(ctx, tx, m); err != nil {
		return nil, err
	}
	return m, internal(tx.Commit())
}
func (r *WatchRepo) Save(ctx context.Context, w *domain.Watch, deadline time.Time, checks int, wake *domain.Message) (bool, *domain.Message, *domain.Message, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, nil, nil, internal(err)
	}
	defer tx.Rollback()
	if wake != nil && w.Task != "" {
		p, e := NewMessageRepo(r.db).one(ctx, tx, `WHERE id=?`, w.Task)
		if e != nil && !domain.IsCode(e, domain.CodeNotFound) {
			return false, nil, nil, e
		}
		if p == nil || p.Status.Terminal() || p.Status != domain.StatusHeld || p.Asking != "" || p.HoldUntil != "" && p.HoldUntil != domain.ManualHold && p.HoldUntil != w.Target {
			w.State, w.Outcome = domain.WatchStopped, "associated task ended"
			wake = nil
		}
	}
	res, err := tx.ExecContext(ctx, `UPDATE watches SET state=?,deadline=?,checks=?,data=? WHERE id=? AND state='watching' AND deadline=? AND checks=?`, w.State, ms(w.Deadline), w.Checks, watchJSON(w), w.ID, ms(deadline), checks)
	if err != nil {
		return false, nil, nil, internal(err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return false, nil, nil, nil
	}
	var parent *domain.Message
	if wake != nil {
		if w.Task != "" {
			parent, err = NewMessageRepo(r.db).one(ctx, tx, `WHERE id=?`, w.Task)
			if err != nil && !domain.IsCode(err, domain.CodeNotFound) {
				return false, nil, nil, err
			}
			if parent != nil && parent.To == domain.AgentName(w.Agent) && parent.Status == domain.StatusHeld && parent.Asking == "" && (parent.HoldUntil == domain.ManualHold || parent.HoldUntil == "" || parent.HoldUntil == w.Target) {
				parent.Status, parent.UpdatedAt, parent.LeaseUntil = domain.StatusQueued, wake.CreatedAt, time.Time{}
				if parent.Attempts > 0 {
					parent.Attempts--
				}
				parent.HoldWake, parent.HoldUntil = wake.ID, ""
				if w.State == domain.WatchMet && w.Kind != "workflow" {
					reply, e := NewMessageRepo(r.db).one(ctx, tx, `WHERE reply_to=? AND to_agent=? AND type IN ('reply','event') ORDER BY created_at,id LIMIT 1`, w.Target, w.Agent)
					if e != nil && !domain.IsCode(e, domain.CodeNotFound) {
						return false, nil, nil, e
					}
					if reply != nil {
						parent.HoldWake = reply.ID
					}
				}
				if err = updateMessage(ctx, tx, parent); err != nil {
					return false, nil, nil, err
				}
				wake.Status = domain.StatusDone
				// A completed child's reply is context for the resumed task, not a second delivery.
				if w.State == domain.WatchMet {
					_, err = tx.ExecContext(ctx, `UPDATE messages SET status='done',updated_at=? WHERE reply_to=? AND to_agent=? AND type IN ('reply','event') AND status IN ('queued','delivered')`, ms(wake.CreatedAt), w.Target, w.Agent)
					if err != nil {
						return false, nil, nil, internal(err)
					}
				}
			} else {
				parent = nil
			}
		}
		if err = insertMessage(ctx, tx, wake); err != nil {
			return false, nil, nil, err
		}
	}
	return true, parent, wake, internal(tx.Commit())
}

// finishHeldWatches participates in the runner's existing wake transaction.
func finishHeldWatches(ctx context.Context, tx *sql.Tx, task, state string, now time.Time) error {
	rows, err := tx.QueryContext(ctx, `SELECT data FROM watches WHERE task=? AND state='watching'`, task)
	if err != nil {
		return internal(err)
	}
	var ws []*domain.Watch
	for rows.Next() {
		var data string
		if err = rows.Scan(&data); err != nil {
			rows.Close()
			return internal(err)
		}
		var w domain.Watch
		if err = json.Unmarshal([]byte(data), &w); err != nil {
			rows.Close()
			return internal(err)
		}
		ws = append(ws, &w)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return internal(err)
	}
	for _, w := range ws {
		if err = w.End(now, state, "hub", "held task resumed"); err != nil {
			return err
		}
		w.LastCheckAt = &now
		w.Checks++
		w.LastResult = "task resumed"
		if _, err = tx.ExecContext(ctx, `UPDATE watches SET state=?,checks=?,data=? WHERE id=?`, w.State, w.Checks, watchJSON(w), w.ID); err != nil {
			return internal(err)
		}
	}
	return nil
}
