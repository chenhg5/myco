package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/domain"
)

// NoticeRepo implements domain.NoticeRepository.
type NoticeRepo struct{ db *sql.DB }

func NewNoticeRepo(db *sql.DB) *NoticeRepo { return &NoticeRepo{db: db} }

var _ domain.NoticeRepository = (*NoticeRepo)(nil)

const noticeCols = `seq, id, recipient, key, kind, ref, target, project, actor, subject, message_id, announce, quiet,
  created_at, read_at, resolved_at`

func scanNotice(s rowScanner) (*domain.Notice, error) {
	var n domain.Notice
	var kind string
	var announce int
	var created, read, resolved int64
	if err := s.Scan(&n.Seq, &n.ID, &n.Recipient, &n.Key, &kind, &n.Ref, &n.Target, &n.Project, &n.Actor, &n.Subject,
		&n.MessageID, &announce, &n.Quiet, &created, &read, &resolved); err != nil {
		return nil, err
	}
	n.Kind, n.Announce = domain.NoticeKind(kind), announce != 0
	n.CreatedAt, n.ReadAt, n.ResolvedAt = fromMS(created), fromMS(read), fromMS(resolved)
	return &n, nil
}

func (r *NoticeRepo) Add(ctx context.Context, n *domain.Notice) (bool, error) {
	announce := 0
	if n.Announce {
		announce = 1
	}
	res, err := r.db.ExecContext(ctx, `INSERT INTO notices (id, recipient, key, kind, tier, ref, target, project, actor, subject,
  message_id, announce, quiet, created_at, read_at, resolved_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
  ON CONFLICT (recipient, key) DO NOTHING`,
		n.ID, n.Recipient, n.Key, string(n.Kind), string(n.Tier()), n.Ref, n.Target, n.Project, n.Actor, n.Subject,
		n.MessageID, announce, n.Quiet, ms(n.CreatedAt), ms(n.ReadAt), ms(n.ResolvedAt))
	if err != nil {
		return false, internal(err)
	}
	if k, _ := res.RowsAffected(); k == 0 {
		return false, nil
	}
	seq, err := res.LastInsertId()
	if err != nil {
		return false, internal(err)
	}
	n.Seq = seq
	return true, nil
}

func (r *NoticeRepo) List(ctx context.Context, f domain.NoticeFilter) ([]*domain.Notice, error) {
	q := `SELECT ` + noticeCols + ` FROM notices WHERE recipient = ? AND seq > ?`
	args := []any{f.Recipient, f.After}
	if f.Open {
		q += ` AND resolved_at = 0`
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 200
	}
	q += ` ORDER BY seq LIMIT ?`
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	out := []*domain.Notice{}
	for rows.Next() {
		n, err := scanNotice(rows)
		if err != nil {
			return nil, internal(err)
		}
		out = append(out, n)
	}
	return out, internal(rows.Err())
}

func (r *NoticeRepo) Last(ctx context.Context, recipient string) (int64, error) {
	var seq sql.NullInt64
	if err := r.db.QueryRowContext(ctx, `SELECT MAX(seq) FROM notices WHERE recipient = ?`, recipient).Scan(&seq); err != nil {
		return 0, internal(err)
	}
	return seq.Int64, nil
}

func (r *NoticeRepo) Resolve(ctx context.Context, key, ref string, at time.Time) ([]string, error) {
	if key == "" && ref == "" {
		return nil, nil
	}
	var conds []string
	var args []any
	if key != "" {
		conds, args = append(conds, "key = ?"), append(args, key)
	}
	if ref != "" {
		conds, args = append(conds, "ref = ?"), append(args, ref)
	}
	where := `resolved_at = 0 AND (` + strings.Join(conds, " OR ") + `)`
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, internal(err)
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT recipient FROM notices WHERE `+where, args...)
	if err != nil {
		return nil, internal(err)
	}
	var who []string
	for rows.Next() {
		var w string
		if err := rows.Scan(&w); err != nil {
			rows.Close()
			return nil, internal(err)
		}
		who = append(who, w)
	}
	rows.Close()
	if len(who) == 0 {
		return nil, nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE notices SET resolved_at = ? WHERE `+where, append([]any{ms(at)}, args...)...); err != nil {
		return nil, internal(err)
	}
	return who, internal(tx.Commit())
}

// actionableKinds, as SQL: reading one of these does not settle it.
func actionableKinds() string {
	var ks []string
	for _, k := range domain.NoticeKinds {
		if k.Actionable() {
			ks = append(ks, "'"+string(k)+"'")
		}
	}
	return "(" + strings.Join(ks, ",") + ")"
}

func (r *NoticeRepo) Read(ctx context.Context, recipient, target string, through int64, at time.Time) (int, error) {
	act := actionableKinds()
	where := `recipient = ? AND (read_at = 0 OR (resolved_at = 0 AND kind NOT IN ` + act + `))`
	args := []any{recipient}
	if through > 0 {
		where += ` AND seq <= ?`
		args = append(args, through)
	}
	if target != "" {
		// "workflow:w1" covers "workflow:w1?run=r1".
		where += ` AND (target = ? OR target LIKE ? ESCAPE '\')`
		args = append(args, target, likePrefix(target)+"?%")
	}
	t := ms(at)
	res, err := r.db.ExecContext(ctx, `UPDATE notices SET
  read_at = CASE WHEN read_at = 0 THEN ? ELSE read_at END,
  resolved_at = CASE WHEN resolved_at = 0 AND kind NOT IN `+act+` THEN ? ELSE resolved_at END
  WHERE `+where, append([]any{t, t}, args...)...)
	if err != nil {
		return 0, internal(err)
	}
	k, _ := res.RowsAffected()
	return int(k), nil
}

func likePrefix(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func (r *NoticeRepo) OpenCount(ctx context.Context, recipient string, tier domain.NoticeTier) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM notices WHERE recipient = ? AND tier = ? AND resolved_at = 0`,
		recipient, string(tier)).Scan(&n)
	return n, internal(err)
}

func (r *NoticeRepo) Prune(ctx context.Context, before time.Time) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM notices WHERE created_at < ?`, ms(before))
	return internal(err)
}
