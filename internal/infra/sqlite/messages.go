package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/domain"
)

// MessageRepo implements domain.MessageRepository.
type MessageRepo struct{ db *sql.DB }

// NewMessageRepo returns a SQLite-backed message repository.
func NewMessageRepo(db *sql.DB) *MessageRepo { return &MessageRepo{db: db} }

var _ domain.MessageRepository = (*MessageRepo)(nil)

const msgCols = `id, from_agent, to_agent, thread_id, reply_to, type, body, status, attempts, lease_until, error, idempotency_key, created_at, updated_at, broadcast_id, attachments, from_owner, project`

type execer interface {
	ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error)
}

func insertMessage(ctx context.Context, x execer, m *domain.Message) error {
	var idem any
	if m.IdempotencyKey != "" {
		idem = m.IdempotencyKey
	}
	att, _ := json.Marshal(nonNil(m.Attachments))
	_, err := x.ExecContext(ctx, `INSERT INTO messages (`+msgCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		m.ID, m.From, m.To, m.ThreadID, m.ReplyTo, m.Type, m.Body, m.Status, m.Attempts,
		ms(m.LeaseUntil), m.Error, idem, ms(m.CreatedAt), ms(m.UpdatedAt), m.BroadcastID, string(att), m.FromOwner, m.Project)
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

func (r *MessageRepo) OpenCounts(ctx context.Context) (map[domain.AgentName]map[domain.Status]int, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT to_agent, status, COUNT(*) FROM messages
		WHERE status IN ('queued','delivered','acked','held') GROUP BY to_agent, status`)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	out := map[domain.AgentName]map[domain.Status]int{}
	for rows.Next() {
		var (
			a domain.AgentName
			s domain.Status
			n int
		)
		if err := rows.Scan(&a, &s, &n); err != nil {
			return nil, internal(err)
		}
		if out[a] == nil {
			out[a] = map[domain.Status]int{}
		}
		out[a][s] = n
	}
	return out, internal(rows.Err())
}

func (r *MessageRepo) ListOpen(ctx context.Context, recipient domain.AgentName, limit int) ([]*domain.Message, error) {
	// In progress first (delivered, acked, held), then what waits (queued).
	return r.many(ctx, r.db, `WHERE to_agent=? AND status IN ('delivered','acked','held','queued')
		ORDER BY CASE status WHEN 'queued' THEN 1 ELSE 0 END, updated_at DESC LIMIT ?`, recipient, limit)
}

func (r *MessageRepo) ListRecent(ctx context.Context, agent domain.AgentName, limit int) ([]*domain.Message, error) {
	if agent == "" {
		return r.many(ctx, r.db, `ORDER BY updated_at DESC, id LIMIT ?`, limit)
	}
	return r.many(ctx, r.db, `WHERE from_agent=? OR to_agent=? ORDER BY updated_at DESC, id LIMIT ?`, agent, agent, limit)
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

func (r *MessageRepo) ClaimDue(ctx context.Context, recipient domain.AgentName, now time.Time, limit int, types []domain.MessageType, claim func(*domain.Message) bool) ([]*domain.Message, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, internal(err)
	}
	defer tx.Rollback()
	// Over-fetch: some candidates may be failed out instead of claimed.
	where, args := `WHERE to_agent=? AND `+dueWhere, []any{recipient, ms(now)}
	if len(types) > 0 {
		where += ` AND type IN (?` + strings.Repeat(",?", len(types)-1) + `)`
		for _, t := range types {
			args = append(args, string(t))
		}
	}
	cands, err := r.many(ctx, tx, where+` ORDER BY created_at, id LIMIT ?`, append(args, limit*2)...)
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
		att                     string
	)
	err := s.Scan(&m.ID, &m.From, &m.To, &m.ThreadID, &m.ReplyTo, &m.Type, &m.Body, &m.Status,
		&m.Attempts, &lease, &m.Error, &idem, &created, &updated, &m.BroadcastID, &att, &m.FromOwner, &m.Project)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(att), &m.Attachments)
	m.IdempotencyKey = idem.String
	m.LeaseUntil, m.CreatedAt, m.UpdatedAt = fromMS(lease), fromMS(created), fromMS(updated)
	return &m, nil
}

// inNames is " IN (?,?,…)" for names, with its arguments.
func inNames(names []domain.AgentName) (string, []any) {
	args := make([]any, len(names))
	for i, n := range names {
		args[i] = string(n)
	}
	return " IN (?" + strings.Repeat(",?", len(names)-1) + ")", args
}

// excludes reports whether a name list that must match rules everything out.
func excludes(names []domain.AgentName) bool { return names != nil && len(names) == 0 }

func (r *MessageRepo) ListRecentThreads(ctx context.Context, f domain.RecentFilter) ([]domain.ThreadRef, error) {
	if excludes(f.Involve) || excludes(f.Match) {
		return nil, nil
	}
	q := `SELECT thread_id, MAX(updated_at) AS u FROM messages WHERE 1=1`
	var args []any
	inThreads := func(not bool, cond string, a ...any) {
		op := " IN "
		if not {
			op = " NOT IN "
		}
		q += ` AND thread_id` + op + `(SELECT thread_id FROM messages WHERE ` + cond + `)`
		args = append(args, a...)
	}
	if f.SenderPrefix != "" {
		like := escapeLike(f.SenderPrefix) + "%"
		inThreads(false, `from_agent LIKE ? ESCAPE '\' OR to_agent LIKE ? ESCAPE '\'`, like, like)
	}
	if skip := append(append([]domain.AgentName{}, f.Nodes...), f.Skip...); len(skip) > 0 {
		in, a := inNames(skip)
		inThreads(true, `from_agent`+in+` OR to_agent`+in, append(a, a...)...)
	}
	for _, names := range [][]domain.AgentName{f.Involve, f.Match} {
		if names != nil {
			in, a := inNames(names)
			inThreads(false, `from_agent`+in+` OR to_agent`+in, append(a, a...)...)
		}
	}
	if f.Project != "" {
		inThreads(false, `project=?`, f.Project)
	}
	if f.Query != "" {
		like := "%" + escapeLike(f.Query) + "%"
		inThreads(false, `body LIKE ? ESCAPE '\' OR from_agent LIKE ? ESCAPE '\' OR to_agent LIKE ? ESCAPE '\'`, like, like, like)
	}
	q += ` GROUP BY thread_id`
	if f.Before > 0 {
		q += ` HAVING u < ?`
		args = append(args, f.Before)
	}
	q += ` ORDER BY u DESC LIMIT ?`
	args = append(args, recentLimit(f.Limit))
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	var out []domain.ThreadRef
	for rows.Next() {
		var id string
		var u int64
		if err := rows.Scan(&id, &u); err != nil {
			return nil, internal(err)
		}
		out = append(out, domain.ThreadRef{ID: id, UpdatedAt: fromMS(u)})
	}
	return out, internal(rows.Err())
}

func (r *MessageRepo) ListRecentPairs(ctx context.Context, f domain.RecentFilter) ([]domain.PairRef, error) {
	if f.AllPairs {
		return r.listAllPairs(ctx, f)
	}
	if len(f.Nodes) == 0 || excludes(f.Involve) || excludes(f.Match) {
		return nil, nil
	}
	nodes, na := inNames(f.Nodes)
	q := `SELECT CASE WHEN to_agent` + nodes + ` THEN from_agent ELSE to_agent END AS other,
		CASE WHEN to_agent` + nodes + ` THEN to_agent ELSE from_agent END AS node,
		COUNT(*), MAX(updated_at) AS u
		FROM messages WHERE (to_agent` + nodes + ` OR from_agent` + nodes + `) AND from_agent != to_agent`
	args := append(append(append(append([]any{}, na...), na...), na...), na...)
	for _, names := range [][]domain.AgentName{f.Involve, f.Match} {
		if names != nil {
			in, a := inNames(names)
			q += ` AND (from_agent` + in + ` OR to_agent` + in + `)`
			args = append(args, append(a, a...)...)
		}
	}
	if f.Project != "" {
		q += ` AND project=?`
		args = append(args, f.Project)
	}
	if f.Query != "" {
		like := "%" + escapeLike(f.Query) + "%"
		q += ` AND (body LIKE ? ESCAPE '\' OR from_agent LIKE ? ESCAPE '\' OR to_agent LIKE ? ESCAPE '\')`
		args = append(args, like, like, like)
	}
	q += ` GROUP BY other, node`
	if f.Before > 0 {
		q += ` HAVING u < ?`
		args = append(args, f.Before)
	}
	q += ` ORDER BY u DESC LIMIT ?`
	args = append(args, recentLimit(f.Limit))
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	var out []domain.PairRef
	for rows.Next() {
		var p domain.PairRef
		var u int64
		if err := rows.Scan(&p.Other, &p.Node, &p.Messages, &u); err != nil {
			return nil, internal(err)
		}
		p.UpdatedAt = fromMS(u)
		out = append(out, p)
	}
	return out, internal(rows.Err())
}

// listAllPairs lists every pair of names that exchanged messages, newest
// first: A is the alphabetically first name, B the other.
func (r *MessageRepo) listAllPairs(ctx context.Context, f domain.RecentFilter) ([]domain.PairRef, error) {
	if excludes(f.Involve) || excludes(f.Match) {
		return nil, nil
	}
	q := `SELECT CASE WHEN from_agent < to_agent THEN from_agent ELSE to_agent END AS a,
		CASE WHEN from_agent < to_agent THEN to_agent ELSE from_agent END AS b,
		COUNT(*), MAX(updated_at) AS u
		FROM messages WHERE from_agent != to_agent`
	var args []any
	if len(f.Skip) > 0 {
		in, a := inNames(f.Skip)
		q += ` AND from_agent NOT` + in + ` AND to_agent NOT` + in
		args = append(args, append(a, a...)...)
	}
	for _, names := range [][]domain.AgentName{f.Involve, f.Match} {
		if names != nil {
			in, a := inNames(names)
			q += ` AND (from_agent` + in + ` OR to_agent` + in + `)`
			args = append(args, append(a, a...)...)
		}
	}
	if f.Party != "" {
		if f.WithoutParty {
			q += ` AND from_agent != ? AND to_agent != ?`
		} else {
			q += ` AND (from_agent = ? OR to_agent = ?)`
		}
		args = append(args, f.Party, f.Party)
	}
	if f.Project != "" {
		q += ` AND project=?`
		args = append(args, f.Project)
	}
	if f.Query != "" {
		like := "%" + escapeLike(f.Query) + "%"
		q += ` AND (body LIKE ? ESCAPE '\' OR from_agent LIKE ? ESCAPE '\' OR to_agent LIKE ? ESCAPE '\')`
		args = append(args, like, like, like)
	}
	q += ` GROUP BY a, b`
	if f.Before > 0 {
		q += ` HAVING u < ?`
		args = append(args, f.Before)
	}
	q += ` ORDER BY u DESC LIMIT ?`
	args = append(args, recentLimit(f.Limit))
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	var out []domain.PairRef
	for rows.Next() {
		var p domain.PairRef
		var u int64
		if err := rows.Scan(&p.Other, &p.Node, &p.Messages, &u); err != nil {
			return nil, internal(err)
		}
		p.UpdatedAt = fromMS(u)
		out = append(out, p)
	}
	return out, internal(rows.Err())
}

func recentLimit(n int) int {
	if n <= 0 || n > 200 {
		return 50
	}
	return n
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(s)
}

func (r *MessageRepo) ListPeers(ctx context.Context, agent domain.AgentName, limit int) ([]domain.PeerRef, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT CASE WHEN from_agent=? THEN to_agent ELSE from_agent END AS peer, COUNT(*), MAX(created_at)
		FROM messages WHERE (from_agent=? OR to_agent=?) AND from_agent NOT LIKE '@%' AND to_agent NOT LIKE '@%' AND from_agent != to_agent
		GROUP BY peer ORDER BY MAX(created_at) DESC LIMIT ?`, agent, agent, agent, limit)
	if err != nil {
		return nil, internal(err)
	}
	var out []domain.PeerRef
	for rows.Next() {
		var (
			p    domain.PeerRef
			last int64
		)
		if err := rows.Scan(&p.Agent, &p.Messages, &last); err != nil {
			rows.Close()
			return nil, internal(err)
		}
		out = append(out, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, internal(err)
	}
	for i := range out {
		ms, err := r.ListBetween(ctx, agent, out[i].Agent, 0, 1)
		if err != nil {
			return nil, err
		}
		if len(ms) > 0 {
			out[i].Last = ms[0]
		}
	}
	return out, nil
}

func (r *MessageRepo) ListBetweenSince(ctx context.Context, a, b domain.AgentName, since int64, limit int) ([]*domain.Message, error) {
	return r.many(ctx, r.db, `WHERE ((from_agent=? AND to_agent=?) OR (from_agent=? AND to_agent=?)) AND updated_at > ?
		ORDER BY updated_at ASC, id ASC LIMIT ?`, a, b, b, a, since, limit)
}

func (r *MessageRepo) ListBetween(ctx context.Context, a, b domain.AgentName, before int64, limit int) ([]*domain.Message, error) {
	where := `WHERE ((from_agent=? AND to_agent=?) OR (from_agent=? AND to_agent=?))`
	args := []any{a, b, b, a}
	if before > 0 {
		where += ` AND created_at < ?`
		args = append(args, before)
	}
	return r.many(ctx, r.db, where+` ORDER BY created_at DESC, id DESC LIMIT ?`, append(args, limit)...)
}
