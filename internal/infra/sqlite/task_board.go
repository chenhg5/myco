package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/domain"
)

// These indexes contain only open task metadata; neither boards nor the
// background evaluator load historical messages or their bodies.
const taskOpen = `m.type='task' AND m.status IN ('queued','delivered','acked','held')`
const taskMonitorSchema = `
CREATE INDEX IF NOT EXISTS idx_tasks_open_sender ON messages(from_agent,project,created_at,id,to_agent,status,updated_at,active_at,thread_id,task_sender_member) WHERE type='task' AND status IN ('queued','delivered','acked','held');
CREATE INDEX IF NOT EXISTS idx_tasks_open_project ON messages(project,from_agent,created_at,id,to_agent,status,updated_at,active_at,thread_id,task_sender_member) WHERE type='task' AND status IN ('queued','delivered','acked','held');
CREATE INDEX IF NOT EXISTS idx_tasks_open_console ON messages(task_sender_member,from_agent,project,created_at,id,to_agent,status,updated_at,active_at,thread_id) WHERE type='task' AND status IN ('queued','delivered','acked','held');
CREATE TABLE IF NOT EXISTS task_alert_policies(project TEXT PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE, stale_ms INTEGER NOT NULL CHECK(stale_ms>0));
CREATE TABLE IF NOT EXISTS task_machine_incidents(machine TEXT NOT NULL, owner TEXT NOT NULL, generation INTEGER NOT NULL, last_failure_at INTEGER NOT NULL, PRIMARY KEY(machine,owner));
CREATE TABLE IF NOT EXISTS task_failure_events(task_id TEXT PRIMARY KEY REFERENCES messages(id) ON DELETE CASCADE, sender TEXT NOT NULL, sender_owner TEXT NOT NULL, recipient TEXT NOT NULL, project TEXT NOT NULL, machine TEXT NOT NULL, owner TEXT NOT NULL, incident INTEGER NOT NULL, failed_at INTEGER NOT NULL);
CREATE INDEX IF NOT EXISTS idx_task_failures_time ON task_failure_events(failed_at,machine,owner,incident,recipient,sender,task_id,project,sender_owner);
CREATE TABLE IF NOT EXISTS task_alerts(key TEXT PRIMARY KEY, kind TEXT NOT NULL, task_id TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE, project TEXT NOT NULL, sender TEXT NOT NULL, destination TEXT NOT NULL, sender_owner TEXT NOT NULL, recipient TEXT NOT NULL, agents INTEGER NOT NULL, at INTEGER NOT NULL, sent_at INTEGER NOT NULL DEFAULT 0);
CREATE INDEX IF NOT EXISTS idx_task_alerts_pending ON task_alerts(sender,at,key) WHERE sent_at=0;
CREATE TABLE IF NOT EXISTS task_alert_deliveries(sender TEXT PRIMARY KEY, sent_at INTEGER NOT NULL);
`

func migrateTaskMonitor(db *sql.DB) error {
	if err := addColumns(db, "messages", [][2]string{{"task_sender_member", "TEXT NOT NULL DEFAULT ''"}}); err != nil {
		return err
	}

	if _, err := db.Exec(taskMonitorSchema); err != nil {
		return err
	}
	// from_owner is a display name in the existing protocol, not a member ID.
	// Bind only new authenticated console tasks to a current, matching member.
	// Do not guess ownership of historical console names after account renames.
	if _, err := db.Exec(`CREATE TRIGGER IF NOT EXISTS task_sender_account AFTER INSERT ON messages
 WHEN NEW.type='task' AND NEW.from_agent GLOB '@console:*' BEGIN
 UPDATE messages SET task_sender_member=COALESCE((SELECT id FROM members WHERE name=NEW.from_owner AND '@console:'||name=NEW.from_agent AND removed_at=0),'') WHERE id=NEW.id;
 END`); err != nil {
		return err
	}
	// Capture failures in the state transition transaction, not an in-memory
	// subscriber. Only machines whose owner still owns the agent may be grouped.
	for _, spec := range []struct{ name, event, condition string }{
		{"task_failure_created", "INSERT", `NEW.type='task' AND NEW.status='failed'`},
		{"task_failure_changed", "UPDATE OF status", `NEW.type='task' AND NEW.status='failed' AND OLD.status!='failed'`},
	} {
		q := `CREATE TRIGGER IF NOT EXISTS ` + spec.name + ` AFTER ` + spec.event + ` ON messages WHEN ` + spec.condition + ` BEGIN
   INSERT INTO task_machine_incidents(machine,owner,generation,last_failure_at)
   SELECT a.machine_id,a.owner,1,NEW.updated_at FROM agents a JOIN machines c ON c.id=a.machine_id AND c.owner=a.owner WHERE a.name=NEW.to_agent AND a.machine_id!=''
   ON CONFLICT(machine,owner) DO UPDATE SET generation=CASE WHEN excluded.last_failure_at>last_failure_at+600000 THEN generation+1 ELSE generation END,last_failure_at=MAX(last_failure_at,excluded.last_failure_at);
   INSERT OR IGNORE INTO task_failure_events(task_id,sender,sender_owner,recipient,project,machine,owner,incident,failed_at)
   SELECT NEW.id,NEW.from_agent,COALESCE(NULLIF((SELECT task_sender_member FROM messages WHERE id=NEW.id),''),(SELECT id FROM members WHERE name=NEW.from_owner AND '@console:'||name=NEW.from_agent AND removed_at=0),''),NEW.to_agent,NEW.project,a.machine_id,a.owner,i.generation,NEW.updated_at FROM agents a JOIN machines c ON c.id=a.machine_id AND c.owner=a.owner JOIN task_machine_incidents i ON i.machine=a.machine_id AND i.owner=a.owner WHERE a.name=NEW.to_agent AND a.machine_id!='';
  END`
		if _, err := db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

type TaskMonitorRepo struct{ db *sql.DB }

func NewTaskMonitorRepo(db *sql.DB) *TaskMonitorRepo { return &TaskMonitorRepo{db: db} }

var _ domain.TaskMonitorRepository = (*TaskMonitorRepo)(nil)

const taskRecordsSQL = `SELECT m.id,m.thread_id,m.from_agent,m.task_sender_member,m.to_agent,m.project,m.status,m.created_at,m.updated_at,m.active_at,COALESCE(a.last_seen_at,0),COALESCE(policy.stale_ms,900000)
 FROM messages m LEFT JOIN agents a ON a.name=m.to_agent LEFT JOIN projects p ON p.name=m.project LEFT JOIN task_alert_policies policy ON policy.project=p.id WHERE ` + taskOpen

func taskQuery(f domain.TaskFilter) (string, []any) {
	q := taskRecordsSQL
	var args []any
	if f.ConsoleOwner != "" {
		q += ` AND m.task_sender_member=? AND m.from_agent GLOB '@console:*'`
		args = append(args, f.ConsoleOwner)
	} else if f.Sender != "" {
		q += ` AND m.from_agent=?`
		args = append(args, f.Sender)
	}
	if f.Project != "" {
		q += ` AND m.project=?`
		args = append(args, f.Project)
	}
	return q, args
}
func (r *TaskMonitorRepo) ListTasks(ctx context.Context, f domain.TaskFilter) ([]domain.TaskRecord, error) {
	q, args := taskQuery(f)
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	out := []domain.TaskRecord{}
	for rows.Next() {
		var x domain.TaskRecord
		var c, u, a, seen, stale int64
		if err := rows.Scan(&x.ID, &x.ThreadID, &x.Sender, &x.SenderOwner, &x.Recipient, &x.Project, &x.Status, &c, &u, &a, &seen, &stale); err != nil {
			return nil, internal(err)
		}
		x.CreatedAt, x.UpdatedAt, x.ActiveAt, x.RecipientSeenAt = fromMS(c), fromMS(u), fromMS(a), fromMS(seen)
		x.StaleAfter = time.Duration(stale) * time.Millisecond
		out = append(out, x)
	}
	return out, internal(rows.Err())
}
func (r *TaskMonitorRepo) TaskFailures(ctx context.Context, since time.Time) ([]domain.TaskFailure, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT task_id,sender,sender_owner,recipient,project,machine,owner,incident,failed_at FROM task_failure_events WHERE failed_at>=? ORDER BY failed_at,task_id`, ms(since))
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	out := []domain.TaskFailure{}
	for rows.Next() {
		var x domain.TaskFailure
		var at int64
		if err := rows.Scan(&x.ID, &x.Sender, &x.SenderOwner, &x.Recipient, &x.Project, &x.Machine, &x.MachineOwner, &x.Incident, &at); err != nil {
			return nil, internal(err)
		}
		x.At = fromMS(at)
		out = append(out, x)
	}
	return out, internal(rows.Err())
}
func (r *TaskMonitorRepo) TaskAlertThreshold(ctx context.Context, project string) (time.Duration, error) {
	var n int64
	err := r.db.QueryRowContext(ctx, `SELECT stale_ms FROM task_alert_policies WHERE project=?`, project).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return 15 * time.Minute, nil
	}
	return time.Duration(n) * time.Millisecond, internal(err)
}
func (r *TaskMonitorRepo) SetTaskAlertThreshold(ctx context.Context, project string, d time.Duration) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO task_alert_policies(project,stale_ms) VALUES(?,?) ON CONFLICT(project) DO UPDATE SET stale_ms=excluded.stale_ms`, project, d.Milliseconds())
	return internal(err)
}
func (r *TaskMonitorRepo) StageTaskAlerts(ctx context.Context, alerts []domain.TaskAlert) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return internal(err)
	}
	defer tx.Rollback()
	// An alert waiting for the cooldown is cancelled if activity resumes, the
	// recipient reconnects, or the task finishes. Sent dedup keys remain durable.
	keep := map[string]bool{}
	for _, a := range alerts {
		keep[a.Key] = true
	}
	rows, err := tx.QueryContext(ctx, `SELECT key,kind FROM task_alerts WHERE sent_at=0`)
	if err != nil {
		return internal(err)
	}
	var remove []string
	for rows.Next() {
		var key, kind string
		if err := rows.Scan(&key, &kind); err != nil {
			rows.Close()
			return internal(err)
		}
		if kind != "machine" && !keep[key] {
			remove = append(remove, key)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return internal(err)
	}
	for _, k := range remove {
		if _, err = tx.ExecContext(ctx, `DELETE FROM task_alerts WHERE key=? AND sent_at=0`, k); err != nil {
			return internal(err)
		}
	}
	for _, a := range alerts {
		if !strings.HasPrefix(string(a.Sender), "@console:") {
			a.SenderOwner = ""
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO task_alerts(key,kind,task_id,project,sender,destination,sender_owner,recipient,agents,at) VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(key) DO NOTHING`, a.Key, a.Kind, a.TaskID, a.Project, a.DeliveryKey(), a.Sender, a.SenderOwner, a.Recipient, a.Agents, ms(a.At))
		if err != nil {
			return internal(err)
		}
	}
	return internal(tx.Commit())
}
func (r *TaskMonitorRepo) PendingTaskAlertSenders(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT DISTINCT sender FROM task_alerts WHERE sent_at=0 ORDER BY sender`)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, internal(err)
		}
		out = append(out, n)
	}
	return out, internal(rows.Err())
}
func (r *TaskMonitorRepo) DeliverTaskAlerts(ctx context.Context, to string, now time.Time, cooldown time.Duration, onlineWindow time.Duration, build func([]domain.TaskAlert) *domain.Message) (*domain.Message, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, internal(err)
	}
	defer tx.Rollback()
	var last int64
	err = tx.QueryRowContext(ctx, `SELECT sent_at FROM task_alert_deliveries WHERE sender=?`, to).Scan(&last)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, internal(err)
	}
	if err == nil && now.Before(fromMS(last).Add(cooldown)) {
		return nil, nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT n.key,n.kind,n.task_id,n.project,CASE WHEN n.sender_owner!='' THEN '@console:'||human.name ELSE n.destination END,n.sender_owner,n.recipient,n.agents,n.at FROM task_alerts n
 JOIN messages m ON m.id=n.task_id LEFT JOIN members human ON human.id=n.sender_owner LEFT JOIN agents a ON a.name=m.to_agent LEFT JOIN projects p ON p.name=m.project LEFT JOIN task_alert_policies policy ON policy.project=p.id
 WHERE n.sender=? AND n.sent_at=0 AND (n.sender_owner='' OR human.removed_at=0) AND (n.kind='machine' OR (
 m.type='task' AND m.status IN ('queued','delivered','acked','held') AND (
 (n.kind='stale' AND MAX(m.active_at,m.updated_at) <= ?-COALESCE(policy.stale_ms,900000)) OR
 (n.kind='offline' AND m.status IN ('delivered','acked') AND a.last_seen_at>0 AND a.last_seen_at<? AND a.last_seen_at+?>=m.created_at))))
 ORDER BY n.at,n.key LIMIT 50`, to, ms(now), ms(now.Add(-onlineWindow)), onlineWindow.Milliseconds())
	if err != nil {
		return nil, internal(err)
	}
	var batch []domain.TaskAlert
	for rows.Next() {
		var a domain.TaskAlert
		var at int64
		if err = rows.Scan(&a.Key, &a.Kind, &a.TaskID, &a.Project, &a.Sender, &a.SenderOwner, &a.Recipient, &a.Agents, &at); err != nil {
			rows.Close()
			return nil, internal(err)
		}
		a.At = fromMS(at)
		batch = append(batch, a)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, internal(err)
	}
	if len(batch) == 0 {
		return nil, nil
	}
	note := build(batch)
	if note == nil || note.Type != domain.TypeNote || note.To != batch[0].Sender {
		return nil, fmt.Errorf("invalid task alert note")
	}
	if err = insertMessage(ctx, tx, note); err != nil {
		return nil, err
	}
	for _, a := range batch {
		if _, err = tx.ExecContext(ctx, `UPDATE task_alerts SET sent_at=? WHERE key=? AND sent_at=0`, ms(now), a.Key); err != nil {
			return nil, internal(err)
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO task_alert_deliveries(sender,sent_at) VALUES(?,?) ON CONFLICT(sender) DO UPDATE SET sent_at=excluded.sent_at`, to, ms(now)); err != nil {
		return nil, internal(err)
	}
	if err = tx.Commit(); err != nil {
		return nil, internal(err)
	}
	return note, nil
}
