package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/domain"
)

// HandoverRepo implements domain.HandoverRepository.
type HandoverRepo struct{ db *sql.DB }

func NewHandoverRepo(db *sql.DB) *HandoverRepo { return &HandoverRepo{db: db} }

var _ domain.HandoverRepository = (*HandoverRepo)(nil)

type bindingRow struct {
	Harness     string `json:"harness,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
	Environment string `json:"environment,omitempty"`
	Workdir     string `json:"workdir,omitempty"`
}

func bindingToJSON(b domain.Binding) string {
	out, _ := json.Marshal(bindingRow{b.Harness, b.SessionID, b.Environment, b.Workdir})
	return string(out)
}

func bindingFromJSON(s string) domain.Binding {
	var r bindingRow
	_ = json.Unmarshal([]byte(s), &r)
	return domain.Binding{Harness: r.Harness, SessionID: r.SessionID, Environment: r.Environment, Workdir: r.Workdir}
}

const handoverCols = `id, agent, at, by_who, note, from_binding, from_machine_id, from_runtime, to_binding, to_machine_id, to_runtime,
  state, done_at, open_tasks, back_to, expires_at`

type rowScanner interface{ Scan(...any) error }

func scanHandover(s rowScanner) (*domain.Handover, error) {
	var h domain.Handover
	var at, done, expires int64
	var fromB, fromRT, toB, toRT, open, state string
	if err := s.Scan(&h.ID, &h.Agent, &at, &h.By, &h.Note, &fromB, &h.From.MachineID, &fromRT, &toB, &h.To.MachineID, &toRT,
		&state, &done, &open, &h.Back, &expires); err != nil {
		return nil, err
	}
	h.At, h.DoneAt, h.ExpiresAt = fromMS(at), fromMS(done), fromMS(expires)
	h.State = domain.HandoverState(state)
	h.From.Binding, h.To.Binding = bindingFromJSON(fromB), bindingFromJSON(toB)
	_ = json.Unmarshal([]byte(fromRT), &h.From.Runtime)
	_ = json.Unmarshal([]byte(toRT), &h.To.Runtime)
	_ = json.Unmarshal([]byte(open), &h.OpenTasks)
	return &h, nil
}

func (r *HandoverRepo) Create(ctx context.Context, h *domain.Handover, linkHash string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return internal(err)
	}
	defer tx.Rollback()
	open, _ := json.Marshal(nonNil(h.OpenTasks))
	if _, err := tx.ExecContext(ctx, `INSERT INTO agent_handovers (`+handoverCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		h.ID, h.Agent, ms(h.At), h.By, h.Note, bindingToJSON(h.From.Binding), h.From.MachineID, runtimeJSON(h.From.Runtime),
		bindingToJSON(h.To.Binding), h.To.MachineID, runtimeJSON(h.To.Runtime), string(h.State), ms(h.DoneAt), string(open), h.Back, ms(h.ExpiresAt)); err != nil {
		return internal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO handover_links(token_hash, handover_id, agent, expires_at) VALUES(?,?,?,?)`,
		linkHash, h.ID, h.Agent, ms(h.ExpiresAt)); err != nil {
		return internal(err)
	}
	return internal(tx.Commit())
}

func (r *HandoverRepo) Get(ctx context.Context, id string) (*domain.Handover, error) {
	h, err := scanHandover(r.db.QueryRowContext(ctx, `SELECT `+handoverCols+` FROM agent_handovers WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.Errf(domain.CodeNotFound, "handover %s not found", id)
	}
	return h, internal(err)
}

func (r *HandoverRepo) List(ctx context.Context, agent domain.AgentName, limit int) ([]*domain.Handover, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.QueryContext(ctx, `SELECT `+handoverCols+` FROM agent_handovers WHERE agent=? ORDER BY at DESC, id DESC LIMIT ?`, agent, limit)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	out := []*domain.Handover{}
	for rows.Next() {
		h, err := scanHandover(rows)
		if err != nil {
			return nil, internal(err)
		}
		out = append(out, h)
	}
	return out, internal(rows.Err())
}

var errLinkGone = domain.Errf(domain.CodeNotFound, "this take-over link has expired or was already used").
	WithSuggestion("ask the node's owner for a new one (myco agent handover-link --name <node>); links work once, within 24 hours")

func (r *HandoverRepo) PeekLink(ctx context.Context, linkHash string, now time.Time) (*domain.Handover, error) {
	var id string
	err := r.db.QueryRowContext(ctx, `SELECT handover_id FROM handover_links WHERE token_hash=? AND used_at=0 AND expires_at>?`, linkHash, ms(now)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errLinkGone
	}
	if err != nil {
		return nil, internal(err)
	}
	return r.Get(ctx, id)
}

func (r *HandoverRepo) Retired(ctx context.Context, tokenHash string) (*domain.Handover, error) {
	var id string
	err := r.db.QueryRowContext(ctx, `SELECT handover_id FROM agent_retired_tokens WHERE token_hash=?`, tokenHash).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.Errf(domain.CodeNotFound, "not a retired credential")
	}
	if err != nil {
		return nil, internal(err)
	}
	return r.Get(ctx, id)
}

func (r *HandoverRepo) TakeOver(ctx context.Context, in domain.TakeOverInput) (*domain.Handover, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, internal(err)
	}
	defer tx.Rollback()
	now := ms(in.Now)
	var id string
	var agent domain.AgentName
	err = tx.QueryRowContext(ctx, `SELECT handover_id, agent FROM handover_links WHERE token_hash=? AND used_at=0 AND expires_at>?`, in.LinkHash, now).Scan(&id, &agent)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errLinkGone
	}
	if err != nil {
		return nil, internal(err)
	}
	var other string
	err = tx.QueryRowContext(ctx, `SELECT name FROM agents WHERE harness=? AND session_id=? AND name<>? LIMIT 1`, in.Binding.Harness, in.Binding.SessionID, agent).Scan(&other)
	if err == nil {
		return nil, domain.Errf(domain.CodeConflict, "this session is already agent %s", other).
			WithSuggestion("take the node over from a session that is not in myco yet, or remove %s first", other)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, internal(err)
	}
	var oldToken, harness, session, env, workdir, machineID, runtime, runSettings string
	err = tx.QueryRowContext(ctx, `SELECT token_hash, harness, session_id, environment, workdir, machine_id, runtime, run_settings FROM agents WHERE name=?`, agent).
		Scan(&oldToken, &harness, &session, &env, &workdir, &machineID, &runtime, &runSettings)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.Errf(domain.CodeNotFound, "agent %s no longer exists", agent)
	}
	if err != nil {
		return nil, internal(err)
	}
	from := domain.Binding{Harness: harness, SessionID: session, Environment: env, Workdir: workdir}
	// The credential being replaced: kept (hashed) so its session learns
	// it was replaced; at most MaxRetiredTokens per agent.
	if oldToken != "" && oldToken != in.TokenHash {
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO agent_retired_tokens(token_hash, agent, handover_id, retired_at) VALUES(?,?,?,?)`,
			oldToken, agent, id, now); err != nil {
			return nil, internal(err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM agent_retired_tokens WHERE agent=? AND token_hash NOT IN
			(SELECT token_hash FROM agent_retired_tokens WHERE agent=? ORDER BY retired_at DESC, rowid DESC LIMIT ?)`, agent, agent, domain.MaxRetiredTokens); err != nil {
			return nil, internal(err)
		}
	}
	// The new session may be one an earlier handover retired (switching back).
	if _, err := tx.ExecContext(ctx, `DELETE FROM agent_retired_tokens WHERE token_hash=?`, in.TokenHash); err != nil {
		return nil, internal(err)
	}
	var rs domain.RunSettings
	_ = json.Unmarshal([]byte(runSettings), &rs)
	if !strings.EqualFold(harness, in.Binding.Harness) {
		rs.Model, rs.Effort = "", ""
	}
	b := in.Binding
	if _, err := tx.ExecContext(ctx, `UPDATE agents SET harness=?, session_id=?, environment=?, workdir=?, token_hash=?, last_seen_at=?,
		machine_id='', runtime='{}', run_settings=? WHERE name=?`,
		b.Harness, b.SessionID, b.Environment, b.Workdir, in.TokenHash, now, runSettingsJSON(rs), agent); err != nil {
		return nil, internal(err)
	}
	// Its open tasks go back to the queue, for the new session; a hand-back
	// is not a failed delivery.
	rows, err := tx.QueryContext(ctx, `SELECT id FROM messages WHERE to_agent=? AND type='task' AND status IN ('delivered','acked','held') ORDER BY created_at`, agent)
	if err != nil {
		return nil, internal(err)
	}
	open := []string{}
	for rows.Next() {
		var mid string
		if err := rows.Scan(&mid); err != nil {
			rows.Close()
			return nil, internal(err)
		}
		open = append(open, mid)
	}
	rows.Close()
	if _, err := tx.ExecContext(ctx, `UPDATE messages SET status='queued', lease_until=0, attempts=0, updated_at=?
		WHERE to_agent=? AND type='task' AND status IN ('delivered','acked','held')`, now, agent); err != nil {
		return nil, internal(err)
	}
	// The old session's recovery link would take the node back: gone.
	if _, err := tx.ExecContext(ctx, `DELETE FROM agent_resumes WHERE agent=?`, agent); err != nil {
		return nil, internal(err)
	}
	openJSON, _ := json.Marshal(open)
	if _, err := tx.ExecContext(ctx, `UPDATE agent_handovers SET from_binding=?, from_machine_id=?, from_runtime=?, to_binding=?,
		state=?, done_at=?, open_tasks=? WHERE id=?`,
		bindingToJSON(from), machineID, runtime, bindingToJSON(b), string(domain.HandoverDone), now, string(openJSON), id); err != nil {
		return nil, internal(err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE handover_links SET used_at=? WHERE token_hash=?`, now, in.LinkHash); err != nil {
		return nil, internal(err)
	}
	// Other waiting links for this agent were made to replace the session
	// that is gone now: they stop working.
	if _, err := tx.ExecContext(ctx, `UPDATE agent_handovers SET state=? WHERE agent=? AND state=? AND id<>?`,
		string(domain.HandoverExpired), agent, string(domain.HandoverWaiting), id); err != nil {
		return nil, internal(err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE handover_links SET used_at=? WHERE agent=? AND used_at=0`, now, agent); err != nil {
		return nil, internal(err)
	}
	h, err := scanHandover(tx.QueryRowContext(ctx, `SELECT `+handoverCols+` FROM agent_handovers WHERE id=?`, id))
	if err != nil {
		return nil, internal(err)
	}
	return h, internal(tx.Commit())
}
