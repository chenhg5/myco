package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/myco-ai/myco/internal/domain"
)

// Traversal does not select bodies, attachments or structured results.
const taskMetaCols = `id,from_agent,to_agent,thread_id,reply_to,parent_id,project,type,status,created_at,updated_at,active_at,after,hold_until,asking,attempts`

func scanTaskMeta(s scanner) (*domain.Message, error) {
	var m domain.Message
	var created, updated, active int64
	err := s.Scan(&m.ID, &m.From, &m.To, &m.ThreadID, &m.ReplyTo, &m.ParentID, &m.Project, &m.Type, &m.Status, &created, &updated, &active, &m.After, &m.HoldUntil, &m.Asking, &m.Attempts)
	m.CreatedAt, m.UpdatedAt = fromMS(created), fromMS(updated)
	if active > 0 {
		m.ActiveAt = fromMS(active)
	}
	return &m, err
}

func (r *MessageRepo) TaskMetadata(ctx context.Context, id string) (*domain.Message, error) {
	m, err := scanTaskMeta(r.db.QueryRowContext(ctx, `SELECT `+taskMetaCols+` FROM messages WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.Errf(domain.CodeNotFound, "message not found")
	}
	if err != nil {
		return nil, internal(err)
	}
	return m, nil
}

func (r *MessageRepo) TaskChildren(ctx context.Context, parent *domain.Message, limit int) ([]*domain.Message, error) {
	// Explicit parent/reply and live hold links take precedence in the service.
	// Older tasks can also be discovered from delegations by this recipient in
	// the same thread; that relationship is reported as inferred, not asserted.
	rows, err := r.db.QueryContext(ctx, `SELECT `+taskMetaCols+` FROM messages WHERE id<>? AND id IN (
 SELECT id FROM messages WHERE type='task' AND parent_id=? AND parent_id<>''
 UNION SELECT id FROM messages WHERE type='task' AND reply_to=?
 UNION SELECT id FROM messages WHERE type='task' AND id=?
 UNION SELECT id FROM messages WHERE type='task' AND parent_id<>'' AND parent_id IN (SELECT id FROM messages WHERE reply_to=?)
 UNION SELECT id FROM messages WHERE type='task' AND parent_id='' AND thread_id=? AND from_agent=? AND created_at>=?
 ) ORDER BY CASE WHEN parent_id=? THEN 0 WHEN id=? THEN 1 ELSE 2 END, created_at,id LIMIT ?`,
		parent.ID, parent.ID, parent.ID, parent.HoldUntil, parent.ID, parent.ThreadID, parent.To, ms(parent.CreatedAt), parent.ID, parent.HoldUntil, limit)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	out := []*domain.Message{}
	for rows.Next() {
		m, err := scanTaskMeta(rows)
		if err != nil {
			return nil, internal(err)
		}
		out = append(out, m)
	}
	return out, internal(rows.Err())
}

func (r *MessageRepo) ReassignTask(ctx context.Context, id string, apply func(*domain.Message) (*domain.AuditEntry, error)) (*domain.Message, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, internal(err)
	}
	defer tx.Rollback()
	m, err := r.one(ctx, tx, `WHERE id=?`, id)
	if err != nil {
		return nil, err
	}
	audit, err := apply(m)
	if err != nil {
		return nil, err
	}
	changed, err := tx.ExecContext(ctx, `UPDATE messages SET to_agent=?,updated_at=?,reassign_note=?,granted_by='' WHERE id=? AND type='task' AND status='queued' AND ever_delivered=0 AND attempts=0`, m.To, ms(m.UpdatedAt), m.ReassignNote, m.ID)
	if err != nil {
		return nil, internal(err)
	}
	n, err := changed.RowsAffected()
	if err != nil {
		return nil, internal(err)
	}
	if n != 1 {
		return nil, domain.Errf(domain.CodeInvalidState, "only an unclaimed queued task can be reassigned").WithSuggestion("stop the task, then send a new task")
	}
	// Attachment sharing and the audit record commit with the recipient change.
	for _, att := range m.Attachments {
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO file_access(file_id,agent) VALUES(?,?)`, att.FileID, m.To); err != nil {
			return nil, internal(err)
		}
	}
	if err = appendAudit(ctx, tx, audit); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, internal(err)
	}
	return m, nil
}
