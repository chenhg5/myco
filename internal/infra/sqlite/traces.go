package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/chenhg5/myco/internal/domain"
)

// TraceRepo implements domain.TraceRepository.
type TraceRepo struct{ db *sql.DB }

// NewTraceRepo returns a SQLite-backed trace repository.
func NewTraceRepo(db *sql.DB) *TraceRepo { return &TraceRepo{db: db} }

var _ domain.TraceRepository = (*TraceRepo)(nil)

func (r *TraceRepo) Append(ctx context.Context, events []*domain.TraceEvent) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return internal(err)
	}
	defer tx.Rollback()
	for _, e := range events {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO message_traces (message, seq, kind, data, at) VALUES (?,?,?,?,?)`,
			e.Message, e.Seq, e.Kind, e.Data, ms(e.At)); err != nil {
			return internal(err)
		}
	}
	return internal(tx.Commit())
}

func (r *TraceRepo) List(ctx context.Context, message string, after, limit int) ([]*domain.TraceEvent, error) {
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	rows, err := r.db.QueryContext(ctx, `SELECT message, seq, kind, data, at FROM message_traces WHERE message=? AND seq>? ORDER BY seq LIMIT ?`, message, after, limit)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	var out []*domain.TraceEvent
	for rows.Next() {
		var (
			e  domain.TraceEvent
			at int64
		)
		if err := rows.Scan(&e.Message, &e.Seq, &e.Kind, &e.Data, &at); err != nil {
			return nil, internal(err)
		}
		e.At = fromMS(at)
		out = append(out, &e)
	}
	return out, internal(rows.Err())
}

func (r *TraceRepo) Bytes(ctx context.Context, message string) (int64, error) {
	var n sql.NullInt64
	err := r.db.QueryRowContext(ctx, `SELECT SUM(LENGTH(data)) FROM message_traces WHERE message=?`, message).Scan(&n)
	return n.Int64, internal(err)
}

func (r *TraceRepo) DeleteBefore(ctx context.Context, t time.Time) (int, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM message_traces WHERE at < ?`, ms(t))
	if err != nil {
		return 0, internal(err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
