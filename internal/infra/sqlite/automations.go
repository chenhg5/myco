package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/myco-ai/myco/internal/domain"
)

// AutomationRepo implements domain.AutomationRepository.
type AutomationRepo struct{ db *sql.DB }

// NewAutomationRepo returns a SQLite-backed automation repository.
func NewAutomationRepo(db *sql.DB) *AutomationRepo { return &AutomationRepo{db: db} }

var _ domain.AutomationRepository = (*AutomationRepo)(nil)

const autCols = `id, name, owner, owner_id, from_agent, to_agent, project, body, cron, tz, webhook_hash, skip_if_busy, enabled, created_at, updated_at, next_run_at, last_task, workflow, inputs`

func (r *AutomationRepo) Create(ctx context.Context, a *domain.Automation) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO automations (`+autCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		a.ID, a.Name, a.Owner, a.OwnerID, a.From, a.To, a.Project, a.Body, a.Cron, a.TZ, a.WebhookHash, a.SkipIfBusy, a.Enabled,
		ms(a.CreatedAt), ms(a.UpdatedAt), ms(a.NextRunAt), a.LastTask, a.Workflow, inputsJSON(a.Inputs))
	if isUniqueViolation(err) {
		return domain.Errf(domain.CodeConflict, "automation %s exists", a.ID)
	}
	return internal(err)
}

func (r *AutomationRepo) Save(ctx context.Context, a *domain.Automation) error {
	res, err := r.db.ExecContext(ctx, `UPDATE automations SET name=?, to_agent=?, project=?, body=?, cron=?, tz=?, webhook_hash=?,
		skip_if_busy=?, enabled=?, updated_at=?, next_run_at=?, last_task=?, workflow=?, inputs=? WHERE id=?`,
		a.Name, a.To, a.Project, a.Body, a.Cron, a.TZ, a.WebhookHash, a.SkipIfBusy, a.Enabled, ms(a.UpdatedAt), ms(a.NextRunAt), a.LastTask,
		a.Workflow, inputsJSON(a.Inputs), a.ID)
	if err != nil {
		return internal(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.Errf(domain.CodeNotFound, "automation %s not found", a.ID)
	}
	return nil
}

func scanAutomation(s scanner) (*domain.Automation, error) {
	var (
		a                      domain.Automation
		created, updated, next int64
		inputs                 string
	)
	err := s.Scan(&a.ID, &a.Name, &a.Owner, &a.OwnerID, &a.From, &a.To, &a.Project, &a.Body, &a.Cron, &a.TZ, &a.WebhookHash,
		&a.SkipIfBusy, &a.Enabled, &created, &updated, &next, &a.LastTask, &a.Workflow, &inputs)
	if err != nil {
		return nil, err
	}
	if inputs != "" {
		_ = json.Unmarshal([]byte(inputs), &a.Inputs)
	}
	a.CreatedAt, a.UpdatedAt, a.NextRunAt = fromMS(created), fromMS(updated), fromMS(next)
	return &a, nil
}

func (r *AutomationRepo) Get(ctx context.Context, id string) (*domain.Automation, error) {
	a, err := scanAutomation(r.db.QueryRowContext(ctx, `SELECT `+autCols+` FROM automations WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.Errf(domain.CodeNotFound, "automation %s not found", id)
	}
	if err != nil {
		return nil, internal(err)
	}
	return a, nil
}

func (r *AutomationRepo) list(ctx context.Context, q string, args ...any) ([]*domain.Automation, error) {
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	var out []*domain.Automation
	for rows.Next() {
		a, err := scanAutomation(rows)
		if err != nil {
			return nil, internal(err)
		}
		out = append(out, a)
	}
	return out, internal(rows.Err())
}

func (r *AutomationRepo) List(ctx context.Context) ([]*domain.Automation, error) {
	return r.list(ctx, `SELECT `+autCols+` FROM automations ORDER BY created_at, id`)
}

func (r *AutomationRepo) Due(ctx context.Context, now time.Time) ([]*domain.Automation, error) {
	return r.list(ctx, `SELECT `+autCols+` FROM automations WHERE enabled = 1 AND next_run_at > 0 AND next_run_at <= ? ORDER BY next_run_at`, ms(now))
}

func (r *AutomationRepo) Count(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM automations`).Scan(&n)
	return n, internal(err)
}

func (r *AutomationRepo) Delete(ctx context.Context, id string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return internal(err)
	}
	defer tx.Rollback() //nolint:errcheck
	res, err := tx.ExecContext(ctx, `DELETE FROM automations WHERE id=?`, id)
	if err != nil {
		return internal(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.Errf(domain.CodeNotFound, "automation %s not found", id)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM automation_runs WHERE automation_id=?`, id); err != nil {
		return internal(err)
	}
	return internal(tx.Commit())
}

// AddRun records a run and keeps the newest keep runs of its automation.
func (r *AutomationRepo) AddRun(ctx context.Context, run *domain.AutomationRun, keep int) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return internal(err)
	}
	defer tx.Rollback() //nolint:errcheck
	req := ""
	if run.Request != nil {
		b, err := json.Marshal(run.Request)
		if err != nil {
			return internal(err)
		}
		req = string(b)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO automation_runs (id, automation_id, at, run_trigger, outcome, message_id, error, request, workflow_run) VALUES (?,?,?,?,?,?,?,?,?)`,
		run.ID, run.AutomationID, ms(run.At), run.Trigger, run.Outcome, run.MessageID, run.Error, req, run.WorkflowRun); err != nil {
		return internal(err)
	}
	if keep > 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM automation_runs WHERE automation_id=? AND id NOT IN
			(SELECT id FROM automation_runs WHERE automation_id=? ORDER BY at DESC, rowid DESC LIMIT ?)`, run.AutomationID, run.AutomationID, keep); err != nil {
			return internal(err)
		}
	}
	return internal(tx.Commit())
}

func (r *AutomationRepo) Runs(ctx context.Context, automationID string, limit int) ([]*domain.AutomationRun, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+runColumns+` FROM automation_runs
		WHERE automation_id=? ORDER BY at DESC, rowid DESC LIMIT ?`, automationID, limit)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	var out []*domain.AutomationRun
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, internal(rows.Err())
}

func (r *AutomationRepo) Run(ctx context.Context, automationID, id string) (*domain.AutomationRun, error) {
	run, err := scanRun(r.db.QueryRowContext(ctx, `SELECT `+runColumns+` FROM automation_runs WHERE automation_id=? AND id=?`, automationID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.Errf(domain.CodeNotFound, "run %s not found", id)
	}
	return run, err
}

const runColumns = `id, automation_id, at, run_trigger, outcome, message_id, error, request, workflow_run`

func scanRun(row interface{ Scan(...any) error }) (*domain.AutomationRun, error) {
	var (
		run domain.AutomationRun
		at  int64
		req string
	)
	if err := row.Scan(&run.ID, &run.AutomationID, &at, &run.Trigger, &run.Outcome, &run.MessageID, &run.Error, &req, &run.WorkflowRun); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		return nil, internal(err)
	}
	run.At = fromMS(at)
	if req != "" {
		var wr domain.WebhookRequest
		if json.Unmarshal([]byte(req), &wr) == nil {
			run.Request = &wr
		}
	}
	return &run, nil
}

func (r *AutomationRepo) RunsSince(ctx context.Context, automationID, trigger string, since time.Time) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM automation_runs WHERE automation_id=? AND run_trigger=? AND at >= ?`,
		automationID, trigger, ms(since)).Scan(&n)
	return n, internal(err)
}

func inputsJSON(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	b, _ := json.Marshal(m)
	return string(b)
}
