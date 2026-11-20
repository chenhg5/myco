package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/chenhg5/myco/internal/domain"
)

// WorkflowRepo implements domain.WorkflowRepository.
type WorkflowRepo struct{ db *sql.DB }

// NewWorkflowRepo returns a SQLite-backed workflow repository.
func NewWorkflowRepo(db *sql.DB) *WorkflowRepo { return &WorkflowRepo{db: db} }

var _ domain.WorkflowRepository = (*WorkflowRepo)(nil)

const wfCols = `id, name, project, owner, owner_id, version, def, enabled, created_at, updated_at`

func defJSON(d *domain.WorkflowDef) string {
	b, _ := json.Marshal(d)
	return string(b)
}

func (r *WorkflowRepo) Create(ctx context.Context, w *domain.Workflow, by string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return internal(err)
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO workflows (`+wfCols+`) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		w.ID, w.Name, w.Project, w.Owner, w.OwnerID, w.Version, defJSON(w.Def), w.Enabled, ms(w.CreatedAt), ms(w.UpdatedAt))
	if isUniqueViolation(err) {
		return domain.Errf(domain.CodeConflict, "a workflow named %q exists", w.Name)
	}
	if err != nil {
		return internal(err)
	}
	if err := addVersion(ctx, tx, w, by); err != nil {
		return err
	}
	return internal(tx.Commit())
}

func addVersion(ctx context.Context, x execer, w *domain.Workflow, by string) error {
	_, err := x.ExecContext(ctx, `INSERT OR REPLACE INTO workflow_versions (workflow_id, version, def, by_name, created_at) VALUES (?,?,?,?,?)`,
		w.ID, w.Version, defJSON(w.Def), by, ms(w.UpdatedAt))
	return internal(err)
}

func (r *WorkflowRepo) Save(ctx context.Context, w *domain.Workflow, newVersion bool, by string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return internal(err)
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE workflows SET name=?, project=?, version=?, def=?, enabled=?, updated_at=? WHERE id=?`,
		w.Name, w.Project, w.Version, defJSON(w.Def), w.Enabled, ms(w.UpdatedAt), w.ID)
	if isUniqueViolation(err) {
		return domain.Errf(domain.CodeConflict, "a workflow named %q exists", w.Name)
	}
	if err != nil {
		return internal(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.Errf(domain.CodeNotFound, "workflow %s not found", w.ID)
	}
	if newVersion {
		if err := addVersion(ctx, tx, w, by); err != nil {
			return err
		}
	}
	return internal(tx.Commit())
}

func scanWorkflow(s scanner) (*domain.Workflow, error) {
	var (
		w                domain.Workflow
		def              string
		created, updated int64
	)
	if err := s.Scan(&w.ID, &w.Name, &w.Project, &w.Owner, &w.OwnerID, &w.Version, &def, &w.Enabled, &created, &updated); err != nil {
		return nil, err
	}
	w.Def = &domain.WorkflowDef{}
	_ = json.Unmarshal([]byte(def), w.Def)
	w.CreatedAt, w.UpdatedAt = fromMS(created), fromMS(updated)
	return &w, nil
}

func (r *WorkflowRepo) one(ctx context.Context, what, where string, arg any) (*domain.Workflow, error) {
	w, err := scanWorkflow(r.db.QueryRowContext(ctx, `SELECT `+wfCols+` FROM workflows WHERE `+where, arg))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.Errf(domain.CodeNotFound, "workflow %s not found", what).WithSuggestion("list workflows with: myco workflow list")
	}
	return w, internal(err)
}

func (r *WorkflowRepo) Get(ctx context.Context, id string) (*domain.Workflow, error) {
	return r.one(ctx, id, `id=?`, id)
}

func (r *WorkflowRepo) GetByName(ctx context.Context, name string) (*domain.Workflow, error) {
	return r.one(ctx, name, `name=?`, name)
}

func (r *WorkflowRepo) List(ctx context.Context) ([]*domain.Workflow, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+wfCols+` FROM workflows ORDER BY name`)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	var out []*domain.Workflow
	for rows.Next() {
		w, err := scanWorkflow(rows)
		if err != nil {
			return nil, internal(err)
		}
		out = append(out, w)
	}
	return out, internal(rows.Err())
}

func (r *WorkflowRepo) Count(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM workflows`).Scan(&n)
	return n, internal(err)
}

// Delete removes a workflow, its versions and runs (the tasks they sent
// stay).
func (r *WorkflowRepo) Delete(ctx context.Context, id string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return internal(err)
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `DELETE FROM workflows WHERE id=?`, id)
	if err != nil {
		return internal(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.Errf(domain.CodeNotFound, "workflow %s not found", id)
	}
	for _, q := range []string{
		`DELETE FROM workflow_versions WHERE workflow_id=?`,
		`DELETE FROM run_events WHERE run_id IN (SELECT id FROM workflow_runs WHERE workflow_id=?)`,
		`DELETE FROM run_steps WHERE run_id IN (SELECT id FROM workflow_runs WHERE workflow_id=?)`,
		`DELETE FROM workflow_runs WHERE workflow_id=?`,
	} {
		if _, err := tx.ExecContext(ctx, q, id); err != nil {
			return internal(err)
		}
	}
	return internal(tx.Commit())
}

func (r *WorkflowRepo) Version(ctx context.Context, id string, version int) (*domain.WorkflowVersion, error) {
	var (
		v       = domain.WorkflowVersion{WorkflowID: id, Version: version, Def: &domain.WorkflowDef{}}
		def     string
		created int64
	)
	err := r.db.QueryRowContext(ctx, `SELECT def, by_name, created_at FROM workflow_versions WHERE workflow_id=? AND version=?`, id, version).
		Scan(&def, &v.By, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.Errf(domain.CodeNotFound, "workflow %s has no version %d", id, version)
	}
	if err != nil {
		return nil, internal(err)
	}
	_ = json.Unmarshal([]byte(def), v.Def)
	v.CreatedAt = fromMS(created)
	return &v, nil
}

const runCols = `id, workflow_id, name, version, project, status, inputs, owner, owner_id, by_name, run_trigger, error, state, created_at, updated_at, ended_at`

func (r *WorkflowRepo) CreateRun(ctx context.Context, run *domain.WorkflowRun) error {
	in, _ := json.Marshal(nonNilMap(run.Inputs))
	st, _ := json.Marshal(run.State)
	_, err := r.db.ExecContext(ctx, `INSERT INTO workflow_runs (`+runCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		run.ID, run.WorkflowID, run.Name, run.Version, run.Project, run.Status, string(in), run.Owner, run.OwnerID, run.By, run.Trigger,
		run.Error, string(st), ms(run.CreatedAt), ms(run.UpdatedAt), ms(run.EndedAt))
	return internal(err)
}

func nonNilMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func (r *WorkflowRepo) SaveRun(ctx context.Context, run *domain.WorkflowRun) error {
	st, _ := json.Marshal(run.State)
	_, err := r.db.ExecContext(ctx, `UPDATE workflow_runs SET status=?, error=?, state=?, updated_at=?, ended_at=? WHERE id=?`,
		run.Status, run.Error, string(st), ms(run.UpdatedAt), ms(run.EndedAt), run.ID)
	return internal(err)
}

func scanFlowRun(s scanner) (*domain.WorkflowRun, error) {
	var (
		run                     domain.WorkflowRun
		in, st                  string
		created, updated, ended int64
	)
	if err := s.Scan(&run.ID, &run.WorkflowID, &run.Name, &run.Version, &run.Project, &run.Status, &in, &run.Owner, &run.OwnerID,
		&run.By, &run.Trigger, &run.Error, &st, &created, &updated, &ended); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(in), &run.Inputs)
	_ = json.Unmarshal([]byte(st), &run.State)
	run.CreatedAt, run.UpdatedAt, run.EndedAt = fromMS(created), fromMS(updated), fromMS(ended)
	return &run, nil
}

func (r *WorkflowRepo) GetRun(ctx context.Context, id string) (*domain.WorkflowRun, error) {
	run, err := scanFlowRun(r.db.QueryRowContext(ctx, `SELECT `+runCols+` FROM workflow_runs WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.Errf(domain.CodeNotFound, "workflow run %s not found", id).WithSuggestion("list runs with: myco workflow runs")
	}
	return run, internal(err)
}

func (r *WorkflowRepo) ListRuns(ctx context.Context, f domain.RunFilter) ([]*domain.WorkflowRun, error) {
	q, args := `SELECT `+runCols+` FROM workflow_runs WHERE 1=1`, []any{}
	if f.WorkflowID != "" {
		q += ` AND workflow_id=?`
		args = append(args, f.WorkflowID)
	}
	if f.Active {
		q += ` AND status IN ('running', 'waiting')`
	}
	if f.Status != "" {
		q += ` AND status=?`
		args = append(args, f.Status)
	}
	q += ` ORDER BY created_at DESC`
	if f.Limit > 0 {
		q += ` LIMIT ?`
		args = append(args, f.Limit)
	}
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	var out []*domain.WorkflowRun
	for rows.Next() {
		run, err := scanFlowRun(rows)
		if err != nil {
			return nil, internal(err)
		}
		out = append(out, run)
	}
	return out, internal(rows.Err())
}

const stepCols = `id, run_id, step_id, kind, attempt, status, target, message_id, input, output, summary, error, retries, bounced, started_at, ended_at, due_at, payload, bounces, tasks, usage`

func (r *WorkflowRepo) SaveStep(ctx context.Context, s *domain.RunStep) error {
	out := ""
	if s.Output != nil {
		b, _ := json.Marshal(s.Output)
		out = string(b)
	}
	payload := ""
	if s.Payload != nil {
		b, _ := json.Marshal(s.Payload)
		payload = string(b)
	}
	tasks, usage := "", ""
	if len(s.Tasks) > 0 {
		b, _ := json.Marshal(s.Tasks)
		tasks = string(b)
	}
	if s.Usage != nil {
		b, _ := json.Marshal(s.Usage)
		usage = string(b)
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO run_steps (`+stepCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET status=excluded.status, target=excluded.target, message_id=excluded.message_id, input=excluded.input,
		output=excluded.output, summary=excluded.summary, error=excluded.error, retries=excluded.retries, bounced=excluded.bounced,
		ended_at=excluded.ended_at, due_at=excluded.due_at, payload=excluded.payload, bounces=excluded.bounces, tasks=excluded.tasks,
		usage=excluded.usage`,
		s.ID, s.RunID, s.StepID, s.Kind, s.Attempt, s.Status, s.Target, s.MessageID, s.Input, out, s.Summary, s.Error, s.Retries, s.Bounced,
		ms(s.StartedAt), ms(s.EndedAt), ms(s.DueAt), payload, s.Bounces, tasks, usage)
	return internal(err)
}

func scanStep(sc scanner) (*domain.RunStep, error) {
	var (
		s                   domain.RunStep
		out, payload        string
		tasks, usage        string
		started, ended, due int64
	)
	if err := sc.Scan(&s.ID, &s.RunID, &s.StepID, &s.Kind, &s.Attempt, &s.Status, &s.Target, &s.MessageID, &s.Input, &out, &s.Summary,
		&s.Error, &s.Retries, &s.Bounced, &started, &ended, &due, &payload, &s.Bounces, &tasks, &usage); err != nil {
		return nil, err
	}
	if tasks != "" {
		_ = json.Unmarshal([]byte(tasks), &s.Tasks)
	}
	if usage != "" {
		var u domain.Usage
		if json.Unmarshal([]byte(usage), &u) == nil {
			s.Usage = &u
		}
	}
	if payload != "" {
		_ = json.Unmarshal([]byte(payload), &s.Payload)
	}
	if out != "" {
		_ = json.Unmarshal([]byte(out), &s.Output)
	}
	s.StartedAt, s.EndedAt, s.DueAt = fromMS(started), fromMS(ended), fromMS(due)
	return &s, nil
}

func (r *WorkflowRepo) Steps(ctx context.Context, runID string) ([]*domain.RunStep, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+stepCols+` FROM run_steps WHERE run_id=? ORDER BY started_at, rowid`, runID)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	var out []*domain.RunStep
	for rows.Next() {
		s, err := scanStep(rows)
		if err != nil {
			return nil, internal(err)
		}
		out = append(out, s)
	}
	return out, internal(rows.Err())
}

func (r *WorkflowRepo) StepByMessage(ctx context.Context, messageID string) (*domain.RunStep, error) {
	s, err := scanStep(r.db.QueryRowContext(ctx, `SELECT `+stepCols+` FROM run_steps WHERE message_id=? AND message_id != ''`, messageID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.Errf(domain.CodeNotFound, "no workflow step sent %s", messageID)
	}
	return s, internal(err)
}

// AddEvent appends to a run's log, dropping its oldest lines past
// domain.MaxRunEvents.
func (r *WorkflowRepo) AddEvent(ctx context.Context, e *domain.RunEvent) error {
	res, err := r.db.ExecContext(ctx, `INSERT INTO run_events (run_id, at, kind, step, text) VALUES (?,?,?,?,?)`, e.RunID, ms(e.At), e.Kind, e.Step, e.Text)
	if err != nil {
		return internal(err)
	}
	e.ID, _ = res.LastInsertId()
	_, err = r.db.ExecContext(ctx, `DELETE FROM run_events WHERE run_id=? AND id <= ?`, e.RunID, e.ID-domain.MaxRunEvents)
	return internal(err)
}

func (r *WorkflowRepo) Events(ctx context.Context, runID string) ([]*domain.RunEvent, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, run_id, at, kind, step, text FROM run_events WHERE run_id=? ORDER BY id`, runID)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	var out []*domain.RunEvent
	for rows.Next() {
		var (
			e  domain.RunEvent
			at int64
		)
		if err := rows.Scan(&e.ID, &e.RunID, &at, &e.Kind, &e.Step, &e.Text); err != nil {
			return nil, internal(err)
		}
		e.At = fromMS(at)
		out = append(out, &e)
	}
	return out, internal(rows.Err())
}

func (r *WorkflowRepo) RunCounts(ctx context.Context, workflowID string) (map[string]int, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT status, COUNT(*) FROM workflow_runs WHERE workflow_id=? GROUP BY status`, workflowID)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var (
			st string
			n  int
		)
		if err := rows.Scan(&st, &n); err != nil {
			return nil, internal(err)
		}
		out[st] = n
	}
	return out, internal(rows.Err())
}

// inIDs is " IN (?,?,…)" for ids, with its arguments.
func inIDs(ids []string) (string, []any) {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return " IN (?" + strings.Repeat(",?", len(ids)-1) + ")", args
}

// chunks splits ids into SQLite-sized pieces.
func chunks(ids []string, n int) [][]string {
	var out [][]string
	for len(ids) > n {
		out = append(out, ids[:n])
		ids = ids[n:]
	}
	if len(ids) > 0 {
		out = append(out, ids)
	}
	return out
}

func (r *WorkflowRepo) StepsOf(ctx context.Context, runIDs []string) (map[string][]*domain.RunStep, error) {
	out := map[string][]*domain.RunStep{}
	for _, part := range chunks(runIDs, 400) {
		in, args := inIDs(part)
		rows, err := r.db.QueryContext(ctx, `SELECT `+stepCols+` FROM run_steps WHERE run_id`+in+` ORDER BY started_at, rowid`, args...)
		if err != nil {
			return nil, internal(err)
		}
		for rows.Next() {
			s, err := scanStep(rows)
			if err != nil {
				rows.Close()
				return nil, internal(err)
			}
			out[s.RunID] = append(out[s.RunID], s)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, internal(err)
		}
	}
	return out, nil
}

func (r *WorkflowRepo) AddEvaluation(ctx context.Context, e *domain.RunEvaluation) error {
	res, err := r.db.ExecContext(ctx, `INSERT INTO run_evaluations (run_id, step, name, value, unit, direction, note, by_name, at) VALUES (?,?,?,?,?,?,?,?,?)`,
		e.RunID, e.Step, e.Name, e.Value, e.Unit, e.Direction, e.Note, e.By, ms(e.At))
	if err != nil {
		return internal(err)
	}
	e.ID, _ = res.LastInsertId()
	_, err = r.db.ExecContext(ctx, `DELETE FROM run_evaluations WHERE run_id=? AND id NOT IN
		(SELECT id FROM run_evaluations WHERE run_id=? ORDER BY id DESC LIMIT ?)`, e.RunID, e.RunID, domain.MaxRunEvaluations)
	return internal(err)
}

func (r *WorkflowRepo) Evaluations(ctx context.Context, runIDs []string) ([]*domain.RunEvaluation, error) {
	var out []*domain.RunEvaluation
	for _, part := range chunks(runIDs, 400) {
		in, args := inIDs(part)
		rows, err := r.db.QueryContext(ctx, `SELECT id, run_id, step, name, value, unit, direction, note, by_name, at
			FROM run_evaluations WHERE run_id`+in+` ORDER BY id`, args...)
		if err != nil {
			return nil, internal(err)
		}
		for rows.Next() {
			var (
				e  domain.RunEvaluation
				at int64
			)
			if err := rows.Scan(&e.ID, &e.RunID, &e.Step, &e.Name, &e.Value, &e.Unit, &e.Direction, &e.Note, &e.By, &at); err != nil {
				rows.Close()
				return nil, internal(err)
			}
			e.At = fromMS(at)
			out = append(out, &e)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, internal(err)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
