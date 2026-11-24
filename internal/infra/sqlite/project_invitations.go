package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/myco-ai/myco/internal/domain"
)

const projectInvitationSchema = `CREATE TABLE IF NOT EXISTS project_invitations (
 id TEXT PRIMARY KEY, project TEXT NOT NULL, requester TEXT NOT NULL, owner TEXT NOT NULL,
 data TEXT NOT NULL, status TEXT NOT NULL, approval_id TEXT NOT NULL DEFAULT '',
 enrollment_id TEXT NOT NULL DEFAULT '', used_by TEXT NOT NULL DEFAULT '',
 created_at INTEGER NOT NULL, expires_at INTEGER NOT NULL
);`

type sqlExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}
type ProjectInvitationRepo struct{ db *sql.DB }

func NewProjectInvitationRepo(db *sql.DB) *ProjectInvitationRepo { return &ProjectInvitationRepo{db} }

const invitationCols = `id,project,requester,owner,data,status,approval_id,enrollment_id,used_by,created_at,expires_at`

func (r *ProjectInvitationRepo) Create(ctx context.Context, v *domain.ProjectInvitation) error {
	b, _ := json.Marshal(v)
	_, err := r.db.ExecContext(ctx, `INSERT INTO project_invitations (`+invitationCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?)`, v.ID, v.Project, v.Requester, v.Owner, string(b), v.Status, v.ApprovalID, v.EnrollmentID, v.UsedBy, ms(v.CreatedAt), ms(v.ExpiresAt))
	return internal(err)
}
func scanInvitation(row scanner) (*domain.ProjectInvitation, error) {
	var v domain.ProjectInvitation
	var data string
	var created, expires int64
	err := row.Scan(&v.ID, &v.Project, &v.Requester, &v.Owner, &data, &v.Status, &v.ApprovalID, &v.EnrollmentID, &v.UsedBy, &created, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.Errf(domain.CodeNotFound, "invitation not found")
	}
	if err != nil {
		return nil, internal(err)
	}
	var meta domain.ProjectInvitation
	if err = json.Unmarshal([]byte(data), &meta); err != nil {
		return nil, internal(err)
	}
	v.DisplayName, v.Description, v.Note = meta.DisplayName, meta.Description, meta.Note
	v.CreatedAt, v.ExpiresAt = fromMS(created), fromMS(expires)
	return &v, nil
}
func (r *ProjectInvitationRepo) Get(ctx context.Context, id string) (*domain.ProjectInvitation, error) {
	return scanInvitation(r.db.QueryRowContext(ctx, `SELECT `+invitationCols+` FROM project_invitations WHERE id=?`, id))
}
func (r *ProjectInvitationRepo) SetApproval(ctx context.Context, id, approval string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE project_invitations SET approval_id=? WHERE id=?`, approval, id)
	return internal(err)
}
func invitationChanged(res sql.Result, err error) error {
	if err != nil {
		return internal(err)
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return domain.Errf(domain.CodeInvalidState, "invitation expired, already claimed, or no longer authorized")
	}
	return nil
}
func (r *ProjectInvitationRepo) Decide(ctx context.Context, id, status string, now time.Time) error {
	return invitationChanged(r.db.ExecContext(ctx, `UPDATE project_invitations SET status=? WHERE id=? AND status='pending' AND expires_at>?`, status, id, ms(now)))
}

// Authorization is rechecked inside each transaction, not only before it.
const invitationAuthorized = `EXISTS (SELECT 1 FROM projects p JOIN project_members pm ON pm.project=p.id WHERE p.id=project_invitations.project AND p.owner=project_invitations.owner AND p.archived=0 AND pm.agent=project_invitations.requester AND pm.role='coordinator')`

func (r *ProjectInvitationRepo) Issue(ctx context.Context, id string, e *domain.Enrollment, now time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return internal(err)
	}
	defer tx.Rollback()
	if err = invitationChanged(tx.ExecContext(ctx, `UPDATE project_invitations SET status='ready', enrollment_id=?,expires_at=? WHERE id=? AND status='approved' AND expires_at>? AND `+invitationAuthorized, e.ID, ms(e.ExpiresAt), id, ms(now))); err != nil {
		return err
	}
	if err = createEnrollment(ctx, tx, e); err != nil {
		return err
	}
	return internal(tx.Commit())
}
func (r *ProjectInvitationRepo) Use(ctx context.Context, id string, a *domain.Agent, now time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return internal(err)
	}
	defer tx.Rollback()
	v, err := scanInvitation(tx.QueryRowContext(ctx, `SELECT `+invitationCols+` FROM project_invitations WHERE id=?`, id))
	if err != nil {
		return err
	}
	if a.Owner != v.Owner || a.InvitedBy != string(v.Requester) || a.Kind() != domain.KindAgent {
		return domain.Errf(domain.CodePermissionDenied, "invitation admits only an ordinary agent owned by the project owner")
	}
	if err = invitationChanged(tx.ExecContext(ctx, `UPDATE project_invitations SET status='used',used_by=? WHERE id=? AND status='ready' AND expires_at>? AND `+invitationAuthorized, a.Name, id, ms(now))); err != nil {
		return err
	}
	if err = invitationChanged(tx.ExecContext(ctx, `UPDATE enrollments SET uses=uses+1 WHERE id=? AND invitation_id=? AND uses=0 AND max_uses=1 AND expires_at>? AND revoked_at=0`, v.EnrollmentID, id, ms(now))); err != nil {
		return err
	}
	if err = createAgent(ctx, tx, a); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO project_members (project,agent,role,note,added_by,added_at) VALUES (?,?,'member',?,?,?)`, v.Project, a.Name, v.Note, v.Requester, ms(now))
	if err != nil {
		return internal(err)
	}
	return internal(tx.Commit())
}
