package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/domain"
)

const agentForkSchema = `
CREATE TABLE IF NOT EXISTS agent_forks (
 id TEXT PRIMARY KEY, copy_name TEXT NOT NULL UNIQUE, copy_id TEXT NOT NULL DEFAULT '',
 source_id TEXT NOT NULL, source_name TEXT NOT NULL, creator_id TEXT NOT NULL,
 machine_id TEXT NOT NULL, node_id TEXT NOT NULL, state TEXT NOT NULL,
 created_at INTEGER NOT NULL, data TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_agent_forks_machine ON agent_forks(machine_id,state);
CREATE INDEX IF NOT EXISTS idx_agent_forks_identity ON agent_forks(copy_id);
`

type ForkRepo struct{ db *sql.DB }

func NewForkRepo(db *sql.DB) *ForkRepo { return &ForkRepo{db} }

// creator_id is private metadata, stored separately from the public JSON.
func scanFork(row rowScanner) (*domain.AgentFork, error) {
	var b, creator string
	if err := row.Scan(&b, &creator); err != nil {
		return nil, err
	}
	var f domain.AgentFork
	if err := json.Unmarshal([]byte(b), &f); err != nil {
		return nil, err
	}
	f.CreatorID = creator
	return &f, nil
}
func forkJSON(f *domain.AgentFork) string { b, _ := json.Marshal(f); return string(b) }
func (r *ForkRepo) Get(ctx context.Context, id string) (*domain.AgentFork, error) {
	f, err := scanFork(r.db.QueryRowContext(ctx, `SELECT data,creator_id FROM agent_forks WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.Errf(domain.CodeNotFound, "fork not found")
	}
	return f, internal(err)
}
func (r *ForkRepo) List(ctx context.Context) ([]*domain.AgentFork, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT data,creator_id FROM agent_forks ORDER BY created_at,id`)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	out := []*domain.AgentFork{}
	for rows.Next() {
		f, err := scanFork(rows)
		if err != nil {
			return nil, internal(err)
		}
		out = append(out, f)
	}
	return out, internal(rows.Err())
}
func (r *ForkRepo) CreateBatch(ctx context.Context, fs []*domain.AgentFork, capacity int) ([]*domain.AgentFork, error) {
	if len(fs) == 0 {
		return nil, domain.Errf(domain.CodeInvalidArgument, "empty fork batch")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, internal(err)
	}
	defer tx.Rollback()
	// Retrying the same caller/batch returns exactly the original reservations.
	rows, err := tx.QueryContext(ctx, `SELECT data,creator_id FROM agent_forks WHERE creator_id=?`, fs[0].CreatorID)
	if err != nil {
		return nil, internal(err)
	}
	prior := []*domain.AgentFork{}
	for rows.Next() {
		f, e := scanFork(rows)
		if e != nil {
			rows.Close()
			return nil, e
		}
		if f.Batch == fs[0].Batch {
			prior = append(prior, f)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, internal(err)
	}
	if len(prior) > 0 {
		if len(prior) != len(fs) {
			return nil, domain.Errf(domain.CodeConflict, "batch key was used with a different count")
		}
		for _, f := range prior {
			if f.SourceID != fs[0].SourceID || f.Worktree != fs[0].Worktree || f.Force != fs[0].Force || f.ForTask != fs[0].ForTask || f.NamePrefix != fs[0].NamePrefix {
				return nil, domain.Errf(domain.CodeConflict, "batch key was used for a different fork request")
			}
		}
		return prior, nil
	}
	if capacity >= 0 {
		var n int
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM agent_forks WHERE machine_id=? AND state!='retired'`, fs[0].MachineID).Scan(&n); err != nil {
			return nil, internal(err)
		}
		if n+len(fs) > capacity {
			return nil, domain.Errf(domain.CodeConflict, "runner has insufficient unreserved slots").WithSuggestion("wait for copies to retire or pass --force to override the resource check")
		}
	}
	for _, f := range fs {
		_, err = tx.ExecContext(ctx, `INSERT INTO agent_forks(id,copy_name,source_id,source_name,creator_id,machine_id,node_id,state,created_at,data) VALUES(?,?,?,?,?,?,?,?,?,?)`, f.ID, f.Name, f.SourceID, f.ForkOf, f.CreatorID, f.MachineID, f.NodeID, f.State, ms(f.CreatedAt), forkJSON(f))
		if isUniqueViolation(err) {
			return nil, domain.Errf(domain.CodeConflict, "copy name is already reserved")
		}
		if err != nil {
			return nil, internal(err)
		}
	}
	return fs, internal(tx.Commit())
}
func (r *ForkRepo) Mutate(ctx context.Context, id string, change func(*domain.AgentFork) error) (*domain.AgentFork, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, internal(err)
	}
	defer tx.Rollback()
	f, err := scanFork(tx.QueryRowContext(ctx, `SELECT data,creator_id FROM agent_forks WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.Errf(domain.CodeNotFound, "fork not found")
	}
	if err != nil {
		return nil, internal(err)
	}
	if err = change(f); err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE agent_forks SET state=?,copy_id=?,data=? WHERE id=?`, f.State, f.AgentID, forkJSON(f), id)
	if err != nil {
		return nil, internal(err)
	}
	return f, internal(tx.Commit())
}
func (r *ForkRepo) Busy(ctx context.Context, f *domain.AgentFork) (bool, error) {
	var n int
	query := `SELECT count(*) FROM messages WHERE to_agent=? AND status IN ('delivered','acked','held')`
	args := []any{f.Name}
	if f.State == "retiring" && !f.Now {
		b, _ := json.Marshal(f.Finishing)
		query = `SELECT count(*) FROM messages WHERE to_agent=? AND status NOT IN ('done','failed') AND id IN (SELECT value FROM json_each(?))`
		args = append(args, string(b))
	}
	err := r.db.QueryRowContext(ctx, query, args...).Scan(&n)
	return n > 0, internal(err)
}
func (r *ForkRepo) Idle(ctx context.Context, f *domain.AgentFork, cutoff time.Time) (bool, error) {
	var n int
	var at int64
	err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM messages WHERE to_agent=? AND status NOT IN ('done','failed')`, f.Name).Scan(&n)
	if err != nil || n > 0 {
		return false, internal(err)
	}
	err = r.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(updated_at),0) FROM messages WHERE from_agent=? OR to_agent=?`, f.Name, f.Name).Scan(&at)
	if err != nil {
		return false, internal(err)
	}
	return f.CreatedAt.Before(cutoff) && at < ms(cutoff), nil
}

// Finalize removes exactly the captured identity and fails its remaining
// messages in the same transaction. A reused name is never deleted.
func (r *ForkRepo) Finalize(ctx context.Context, id string, now time.Time, newID func() string) (*domain.ForkFinalization, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, internal(err)
	}
	defer tx.Rollback()
	f, err := scanFork(tx.QueryRowContext(ctx, `SELECT data,creator_id FROM agent_forks WHERE id=?`, id))
	if err != nil {
		return nil, internal(err)
	}
	if f.State == "retired" {
		return &domain.ForkFinalization{Fork: f}, nil
	}
	if f.State != "retiring" {
		return nil, domain.Errf(domain.CodeInvalidState, "copy is not retiring")
	}
	a, err := scanAgent(tx.QueryRowContext(ctx, `SELECT `+agentCols+` FROM agents WHERE name=?`, f.Name))
	if err == nil && (f.AgentID == "" || a.ID != f.AgentID || a.MachineID != f.MachineID || a.Binding.Workdir != f.Workdir) {
		return nil, domain.Errf(domain.CodeConflict, "copy identity or binding changed; retirement refused")
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, internal(err)
	}
	out := &domain.ForkFinalization{Fork: f}
	if err == nil {
		mr := NewMessageRepo(r.db)
		pending, e := mr.many(ctx, tx, `WHERE to_agent=? AND status NOT IN ('done','failed')`, f.Name)
		if e != nil {
			return nil, e
		}
		out.Closed = pending
		for _, m := range pending {
			if m.Status == domain.StatusQueued {
				m.Status = domain.StatusDelivered
			}
			ev, e := m.Fail(m.To, newID(), "copy retired", now)
			if e != nil {
				return nil, e
			}
			if e = updateMessage(ctx, tx, m); e != nil {
				return nil, e
			}
			if ev != nil {
				out.Events = append(out.Events, ev)
				if e = insertMessage(ctx, tx, ev); e != nil {
					return nil, e
				}
			}
		}
		for _, q := range []string{`DELETE FROM aliases WHERE target=?`, `DELETE FROM project_members WHERE agent=?`, `DELETE FROM agent_resumes WHERE agent=?`} {
			if _, e := tx.ExecContext(ctx, q, f.Name); e != nil {
				return nil, internal(e)
			}
		}
		if _, e := tx.ExecContext(ctx, `DELETE FROM agents WHERE id=?`, f.AgentID); e != nil {
			return nil, internal(e)
		}
	}
	f.State = "retired"
	f.Error = ""
	if _, err = tx.ExecContext(ctx, `UPDATE agent_forks SET state='retired',data=? WHERE id=?`, forkJSON(f), id); err != nil {
		return nil, internal(err)
	}
	return out, internal(tx.Commit())
}

func (r *ForkRepo) Activate(ctx context.Context, id, workdir, repository, branch string) (*domain.AgentFork, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, internal(err)
	}
	defer tx.Rollback()
	f, err := scanFork(tx.QueryRowContext(ctx, `SELECT data,creator_id FROM agent_forks WHERE id=?`, id))
	if err != nil {
		return nil, internal(err)
	}
	if f.State == "ready" {
		return f, nil
	}
	if f.State != "planned" && !(f.State == "retiring" && f.AgentID == "") {
		return nil, domain.Errf(domain.CodeInvalidState, "copy is not awaiting registration")
	}
	a, err := scanAgent(tx.QueryRowContext(ctx, `SELECT `+agentCols+` FROM agents WHERE name=?`, f.Name))
	if err != nil {
		return nil, internal(err)
	}
	node, err := scanAgent(tx.QueryRowContext(ctx, `SELECT `+agentCols+` FROM agents WHERE id=?`, f.NodeID))
	if err != nil {
		return nil, internal(err)
	}
	var owner string
	if err = tx.QueryRowContext(ctx, `SELECT owner FROM machines WHERE id=?`, f.MachineID).Scan(&owner); err != nil {
		return nil, internal(err)
	}
	if owner != f.Owner || node.Owner != owner || node.MachineID != f.MachineID || a.Owner != owner || a.EnrolledBy == "" || a.EnrolledBy != node.EnrolledBy || a.CreatedAt.Before(f.CreatedAt) || a.Binding.Harness != f.CLI || a.Binding.Workdir != workdir || a.Binding.SessionID == "" || a.Binding.SessionID == f.SourceSession || (a.MachineID != "" && a.MachineID != f.MachineID) {
		return nil, domain.Errf(domain.CodeConflict, "registered identity does not match this copy")
	}
	// The node resolves symlinks against its local spawn policy; a canonical
	// copy path can differ from the source's reported spelling.
	f.AgentID = a.ID
	f.Workdir = workdir
	f.Repository = repository
	f.Branch = branch
	if f.State != "retiring" {
		f.State = "ready"
	}
	f.Error = ""
	allow := append([]domain.AgentName(nil), f.SourceAllowFrom...)
	if len(allow) > 0 && strings.HasPrefix(f.CreatorID, "agent:") && !slices.Contains(allow, domain.AgentName(f.ForkedBy)) {
		allow = append(allow, domain.AgentName(f.ForkedBy))
	}
	allowed, _ := json.Marshal(allow)
	if _, err = tx.ExecContext(ctx, `UPDATE agents SET machine_id=?,visibility=?,allow_from=? WHERE id=?`, f.MachineID, f.SourceVisibility, string(allowed), a.ID); err != nil {
		return nil, internal(err)
	}
	if _, err = tx.ExecContext(ctx, `UPDATE agent_forks SET state=?,copy_id=?,data=? WHERE id=?`, f.State, a.ID, forkJSON(f), id); err != nil {
		return nil, internal(err)
	}
	return f, internal(tx.Commit())
}

// ExpireIdle closes the idle-check/claim race in one write transaction.
func (r *ForkRepo) ExpireIdle(ctx context.Context, id string, cutoff time.Time) (*domain.AgentFork, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, internal(err)
	}
	defer tx.Rollback()
	f, err := scanFork(tx.QueryRowContext(ctx, `SELECT data,creator_id FROM agent_forks WHERE id=?`, id))
	if err != nil {
		return nil, internal(err)
	}
	if (f.State != "ready" && f.State != "planned") || !f.CreatedAt.Before(cutoff) {
		return f, nil
	}
	var n int
	var at int64
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM messages WHERE to_agent=? AND status NOT IN ('done','failed')`, f.Name).Scan(&n); err != nil {
		return nil, internal(err)
	}
	if n > 0 {
		return f, nil
	}
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(updated_at),0) FROM messages WHERE from_agent=? OR to_agent=?`, f.Name, f.Name).Scan(&at); err != nil {
		return nil, internal(err)
	}
	if at >= ms(cutoff) {
		return f, nil
	}
	f.State = "retiring"
	if _, err = tx.ExecContext(ctx, `UPDATE agent_forks SET state='retiring',data=? WHERE id=?`, forkJSON(f), id); err != nil {
		return nil, internal(err)
	}
	return f, internal(tx.Commit())
}

// BeginRetirement captures exactly the work already claimed. Those messages
// can finish (including after a runner restart); new queued work cannot start.
func (r *ForkRepo) BeginRetirement(ctx context.Context, id string, now bool) (*domain.AgentFork, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, internal(err)
	}
	defer tx.Rollback()
	f, err := scanFork(tx.QueryRowContext(ctx, `SELECT data,creator_id FROM agent_forks WHERE id=?`, id))
	if err != nil {
		return nil, internal(err)
	}
	if f.State == "retired" {
		return f, nil
	}
	if f.State != "retiring" {
		rows, e := tx.QueryContext(ctx, `SELECT id FROM messages WHERE to_agent=? AND status IN ('delivered','acked','held')`, f.Name)
		if e != nil {
			return nil, internal(e)
		}
		f.Finishing = []string{}
		for rows.Next() {
			var mid string
			if e = rows.Scan(&mid); e != nil {
				rows.Close()
				return nil, internal(e)
			}
			f.Finishing = append(f.Finishing, mid)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, internal(e)
		}
	}
	f.State = "retiring"
	f.Now = f.Now || now
	if _, err = tx.ExecContext(ctx, `UPDATE agent_forks SET state='retiring',data=? WHERE id=?`, forkJSON(f), id); err != nil {
		return nil, internal(err)
	}
	return f, internal(tx.Commit())
}
