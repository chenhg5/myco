package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/chenhg5/myco/internal/domain"
)

// MessageRepo implements domain.MessageRepository.
type MessageRepo struct{ db *sql.DB }

// NewMessageRepo returns a SQLite-backed message repository.
func NewMessageRepo(db *sql.DB) *MessageRepo { return &MessageRepo{db: db} }

var _ domain.MessageRepository = (*MessageRepo)(nil)

const msgCols = `id, from_agent, to_agent, thread_id, reply_to, type, body, status, attempts, lease_until, error, idempotency_key, created_at, updated_at, broadcast_id`

type execer interface {
	ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error)
}

func insertMessage(ctx context.Context, x execer, m *domain.Message) error {
	var idem any
	if m.IdempotencyKey != "" {
		idem = m.IdempotencyKey
	}
	_, err := x.ExecContext(ctx, `INSERT INTO messages (`+msgCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		m.ID, m.From, m.To, m.ThreadID, m.ReplyTo, m.Type, m.Body, m.Status, m.Attempts,
		ms(m.LeaseUntil), m.Error, idem, ms(m.CreatedAt), ms(m.UpdatedAt), m.BroadcastID)
	if isUniqueViolation(err) {
		return domain.Errf(domain.CodeConflict, "duplicate message (id or idempotency key)")
	}
	return internal(err)
}

func updateMessage(ctx context.Context, x execer, m *domain.Message) error {
	_, err := x.ExecContext(ctx, `UPDATE messages SET status=?, attempts=?, lease_until=?, error=?, updated_at=? WHERE id=?`,
		m.Status, m.Attempts, ms(m.LeaseUntil), m.Error, ms(m.UpdatedAt), m.ID)
	return internal(err)
}

func (r *MessageRepo) Create(ctx context.Context, m *domain.Message) error {
	return insertMessage(ctx, r.db, m)
}

func (r *MessageRepo) CreateMany(ctx context.Context, msgs []*domain.Message) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return internal(err)
	}
	defer tx.Rollback()
	for _, m := range msgs {
		if err := insertMessage(ctx, tx, m); err != nil {
			return err
		}
	}
	return internal(tx.Commit())
}

func (r *MessageRepo) ListBroadcast(ctx context.Context, broadcastID string) ([]*domain.Message, error) {
	return r.many(ctx, r.db, `WHERE broadcast_id=? ORDER BY created_at, id`, broadcastID)
}

func (r *MessageRepo) Get(ctx context.Context, id string) (*domain.Message, error) {
	return r.one(ctx, r.db, `WHERE id=?`, id)
}

func (r *MessageRepo) FindByIdempotencyKey(ctx context.Context, from domain.AgentName, key string) (*domain.Message, error) {
	return r.one(ctx, r.db, `WHERE from_agent=? AND idempotency_key=?`, from, key)
}

func (r *MessageRepo) FindReply(ctx context.Context, messageID string) (*domain.Message, error) {
	return r.one(ctx, r.db, `WHERE reply_to=? AND type IN ('reply','event') ORDER BY created_at LIMIT 1`, messageID)
}

func (r *MessageRepo) ListThread(ctx context.Context, threadID string) ([]*domain.Message, error) {
	return r.many(ctx, r.db, `WHERE thread_id=? ORDER BY created_at, id`, threadID)
}

const dueWhere = `(status='queued' OR (status='delivered' AND lease_until<=?))`

func (r *MessageRepo) ListDue(ctx context.Context, recipient domain.AgentName, now time.Time, limit int) ([]*domain.Message, error) {
	return r.many(ctx, r.db, `WHERE to_agent=? AND `+dueWhere+` ORDER BY created_at, id LIMIT ?`, recipient, ms(now), limit)
}

func (r *MessageRepo) DueRecipients(ctx context.Context, now time.Time) ([]domain.AgentName, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT DISTINCT to_agent FROM messages WHERE `+dueWhere, ms(now))
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	var out []domain.AgentName
	for rows.Next() {
		var n domain.AgentName
		if err := rows.Scan(&n); err != nil {
			return nil, internal(err)
		}
		out = append(out, n)
	}
	return out, internal(rows.Err())
}

func (r *MessageRepo) Mutate(ctx context.Context, id string, fn domain.MutateFunc) (*domain.Message, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, internal(err)
	}
	defer tx.Rollback()
	m, err := r.one(ctx, tx, `WHERE id=?`, id)
	if err != nil {
		return nil, err
	}
	created, err := fn(m)
	if err != nil {
		return nil, err
	}
	if err := updateMessage(ctx, tx, m); err != nil {
		return nil, err
	}
	for _, c := range created {
		if err := insertMessage(ctx, tx, c); err != nil {
			return nil, err
		}
	}
	return m, internal(tx.Commit())
}

func (r *MessageRepo) ClaimDue(ctx context.Context, recipient domain.AgentName, now time.Time, limit int, claim func(*domain.Message) bool) ([]*domain.Message, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, internal(err)
	}
	defer tx.Rollback()
	// Over-fetch: some candidates may be failed out instead of claimed.
	cands, err := r.many(ctx, tx, `WHERE to_agent=? AND `+dueWhere+` ORDER BY created_at, id LIMIT ?`,
		recipient, ms(now), limit*2)
	if err != nil {
		return nil, err
	}
	var out []*domain.Message
	for _, m := range cands {
		if len(out) >= limit {
			break
		}
		if !m.IsDue(now) {
			continue
		}
		ok := claim(m)
		if err := updateMessage(ctx, tx, m); err != nil {
			return nil, err
		}
		if ok {
			out = append(out, m)
		}
	}
	return out, internal(tx.Commit())
}

type querier interface {
	QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row
}

func (r *MessageRepo) one(ctx context.Context, q querier, where string, args ...any) (*domain.Message, error) {
	m, err := scanMessage(q.QueryRowContext(ctx, `SELECT `+msgCols+` FROM messages `+where, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.Errf(domain.CodeNotFound, "message not found")
	}
	return m, internal(err)
}

func (r *MessageRepo) many(ctx context.Context, q querier, where string, args ...any) ([]*domain.Message, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+msgCols+` FROM messages `+where, args...)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	var out []*domain.Message
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, internal(err)
		}
		out = append(out, m)
	}
	return out, internal(rows.Err())
}

func scanMessage(s scanner) (*domain.Message, error) {
	var (
		m                       domain.Message
		lease, created, updated int64
		idem                    sql.NullString
	)
	err := s.Scan(&m.ID, &m.From, &m.To, &m.ThreadID, &m.ReplyTo, &m.Type, &m.Body, &m.Status,
		&m.Attempts, &lease, &m.Error, &idem, &created, &updated, &m.BroadcastID)
	if err != nil {
		return nil, err
	}
	m.IdempotencyKey = idem.String
	m.LeaseUntil, m.CreatedAt, m.UpdatedAt = fromMS(lease), fromMS(created), fromMS(updated)
	return &m, nil
}
