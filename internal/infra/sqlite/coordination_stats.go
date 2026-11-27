package sqlite

import (
	"context"
	"database/sql"
	"strings"

	"github.com/myco-ai/myco/internal/domain"
)

// CoordinationRepo reads only indexed task metadata. It never selects bodies.
type CoordinationRepo struct{ db *sql.DB }

func NewCoordinationRepo(db *sql.DB) *CoordinationRepo { return &CoordinationRepo{db: db} }

func (r *CoordinationRepo) TaskTimings(ctx context.Context, f domain.CoordinationFilter) ([]domain.TaskTiming, error) {
	out := []domain.TaskTiming{}
	if excludes(f.Names) {
		return out, nil
	}
	q := `SELECT m.id,m.thread_id,m.from_agent,m.to_agent,m.created_at,m.updated_at,m.first_acked_at,m.status,
 (m.error='stopped by the sender' OR m.error LIKE 'stopped in the app by %'),
 COALESCE((SELECT MIN(x.created_at) FROM messages x WHERE x.reply_to=m.id AND x.type='reply' AND x.created_at<=?),0),
 EXISTS(SELECT 1 FROM messages n WHERE n.thread_id=m.thread_id AND n.type='task' AND n.from_agent=m.from_agent AND n.created_at<=? AND
 ((m.status='failed' AND (m.error='stopped by the sender' OR m.error LIKE 'stopped in the app by %') AND n.created_at>m.updated_at) OR
 EXISTS(SELECT 1 FROM messages s WHERE s.reply_to=m.id AND s.type='steer' AND s.created_at<n.created_at))),
 EXISTS(SELECT 1 FROM messages n WHERE n.thread_id=m.thread_id AND n.type='task' AND n.from_agent=m.from_agent AND n.to_agent<>m.to_agent AND n.created_at>m.updated_at AND n.created_at<=?),
 (SELECT COUNT(*) FROM messages s WHERE s.thread_id=m.thread_id AND s.from_agent=m.from_agent AND s.to_agent=m.to_agent AND s.created_at>m.created_at AND s.created_at<=? AND
 ((s.type='steer' AND s.reply_to=m.id AND s.answers='') OR s.type='task') AND s.created_at<COALESCE((SELECT MIN(x.created_at) FROM messages x WHERE x.reply_to=m.id AND x.type='reply'),?))
 FROM messages m INDEXED BY idx_messages_task_stats WHERE m.type='task' AND m.created_at>=? AND m.created_at<=?`
	args := []any{ms(f.Until), ms(f.Until), ms(f.Until), ms(f.Until), ms(f.Until), ms(f.Since), ms(f.Until)}
	if f.Agent != "" {
		q += ` AND (m.from_agent=? OR m.to_agent=?)`
		args = append(args, f.Agent, f.Agent)
	}
	if f.Project != "" {
		q += ` AND m.project=?`
		args = append(args, f.Project)
	}
	if f.Names != nil {
		in, a := inNames(f.Names)
		q += ` AND (m.from_agent` + in + ` OR m.to_agent` + in + `)`
		args = append(args, a...)
		args = append(args, a...)
	}
	// Bound memory and CPU explicitly; never silently publish a partial aggregate.
	q += ` ORDER BY m.created_at,m.id LIMIT 100001`
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var t domain.TaskTiming
		var created, updated, started, replied int64
		var status string
		if err := rows.Scan(&t.ID, &t.Thread, &t.From, &t.To, &created, &updated, &started, &status, &t.Stopped, &replied, &t.Redone, &t.Reassigned, &t.Followups); err != nil {
			return nil, internal(err)
		}
		t.Created, t.Updated = fromMS(created), fromMS(updated)
		if started > 0 {
			t.Started = fromMS(started)
		}
		if replied > 0 {
			t.Replied = fromMS(replied)
		}
		t.Done = status == "done"
		t.Failed = status == "failed" && !t.Stopped
		t.Reassigned = t.Reassigned && t.Stopped
		// Followups are attributed only to a person's first task in a thread.
		if !strings.HasPrefix(string(t.From), "@console:") {
			t.Followups = 0
		}
		out = append(out, t)
		if len(out) > 100000 {
			return nil, domain.Errf(domain.CodeInvalidArgument, "statistics exceed 100000 tasks").WithSuggestion("use a shorter --since window or --project")
		}
	}
	return out, internal(rows.Err())
}
