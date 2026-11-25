package sqlite

import (
	"context"
	"time"

	"github.com/myco-ai/myco/internal/domain"
)

// WakeHeld finds durable wake conditions while the runner is polling. Moving
// the original task, consuming its wake message and recording a recovery are
// one transaction, so another poll or a process restart cannot wake it twice.
func (r *MessageRepo) WakeHeld(ctx context.Context, recipient domain.AgentName, now, cutoff time.Time, limit int, recovery func(*domain.Message) *domain.Message) ([]*domain.Message, []*domain.Message, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, internal(err)
	}
	defer tx.Rollback()
	const answer = `SELECT st.id FROM messages st WHERE st.reply_to=messages.id AND st.type='steer'
    AND st.answers!='' AND (st.id=messages.hold_wake OR
      (messages.hold_wake='' AND st.created_at>=messages.updated_at AND st.status IN ('queued','delivered')))
    ORDER BY st.created_at DESC, st.id DESC LIMIT 1`
	const ready = `SELECT child.id FROM messages child WHERE child.id=messages.hold_until
    AND child.from_agent=messages.to_agent AND child.status IN ('done','failed')`
	rows, err := tx.QueryContext(ctx, `SELECT id, COALESCE((`+answer+`), ''), COALESCE((`+ready+`), '')
    FROM messages WHERE to_agent=? AND status='held' AND type='task' AND asking='' AND
    NOT EXISTS (SELECT 1 FROM watches w WHERE w.task=messages.id AND w.state='watching' AND w.deadline<=?) AND
    (EXISTS(`+answer+`) OR EXISTS(`+ready+`) OR (hold_until='' AND hold_recovered=0 AND updated_at<=?))
    ORDER BY updated_at, id LIMIT ?`, recipient, ms(now), ms(cutoff), limit)
	if err != nil {
		return nil, nil, internal(err)
	}
	type candidate struct{ id, answer, child string }
	var candidates []candidate
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.id, &item.answer, &item.child); err != nil {
			rows.Close()
			return nil, nil, internal(err)
		}
		candidates = append(candidates, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, nil, internal(err)
	}
	var changed, created []*domain.Message
	for _, item := range candidates {
		task, err := r.one(ctx, tx, `WHERE id=?`, item.id)
		if err != nil {
			return nil, nil, err
		}
		wakeID := item.answer
		if wakeID == "" {
			wakeID = item.child
			if item.child != "" {
				reply, err := r.one(ctx, tx, `WHERE reply_to=? AND type IN ('reply','event') ORDER BY created_at,id LIMIT 1`, item.child)
				if err != nil && !domain.IsCode(err, domain.CodeNotFound) {
					return nil, nil, err
				}
				if reply != nil {
					wakeID = reply.ID
				}
			}
		}
		if wakeID == "" {
			progress := recovery(task)
			if err := insertMessage(ctx, tx, progress); err != nil {
				return nil, nil, err
			}
			created = append(created, progress)
			task.HoldRecovered = true
			wakeID = domain.RecoveredHold
		} else {
			// A child may already have been consumed via message wait, or may
			// have closed without a reply. Neither case re-delivers the child.
			if _, err := tx.ExecContext(ctx, `UPDATE messages SET status='done', updated_at=? WHERE id=?
        AND to_agent=? AND type IN ('reply','event','steer') AND status IN ('queued','delivered')`,
				ms(now), wakeID, recipient); err != nil {
				return nil, nil, internal(err)
			}
		}
		task.Status, task.UpdatedAt, task.LeaseUntil = domain.StatusQueued, now, time.Time{}
		if task.Attempts > 0 {
			task.Attempts--
		}
		task.HoldWake, task.HoldUntil = wakeID, ""
		if err := updateMessage(ctx, tx, task); err != nil {
			return nil, nil, err
		}
		state := domain.WatchStopped
		if item.child != "" {
			state = domain.WatchMet
		}
		if err := finishHeldWatches(ctx, tx, task.ID, state, now); err != nil {
			return nil, nil, err
		}
		changed = append(changed, task)
	}
	return changed, created, internal(tx.Commit())
}
