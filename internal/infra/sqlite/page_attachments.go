package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/myco-ai/myco/internal/domain"
)

// This additive migration runs for old databases too; project rows are moved
// only by the recoverable content migration, never by schema initialization.
const pageAttachmentSchema = `
CREATE TABLE IF NOT EXISTS page_attachments (
 id TEXT PRIMARY KEY, page TEXT NOT NULL, collection TEXT NOT NULL,
 name TEXT NOT NULL, blob TEXT NOT NULL UNIQUE, size INTEGER NOT NULL,
 sha256 TEXT NOT NULL, content_type TEXT NOT NULL, by_name TEXT NOT NULL,
 state TEXT NOT NULL, request_key TEXT NOT NULL, legacy_name TEXT NOT NULL DEFAULT '',
 created_at INTEGER NOT NULL, expires_at INTEGER NOT NULL,
 UNIQUE(page,request_key)
);
CREATE INDEX IF NOT EXISTS idx_page_attachments_page ON page_attachments(page,id);
CREATE TABLE IF NOT EXISTS page_attachment_aliases (
 page TEXT NOT NULL, project TEXT NOT NULL, name TEXT NOT NULL,
 attachment TEXT NOT NULL, PRIMARY KEY(page,name)
);
CREATE INDEX IF NOT EXISTS idx_page_attachment_alias_project ON page_attachment_aliases(project,name);
CREATE TABLE IF NOT EXISTS page_attachment_refs (
 page TEXT NOT NULL, collection TEXT NOT NULL, record TEXT NOT NULL,
 field TEXT NOT NULL, attachment TEXT NOT NULL,
 PRIMARY KEY(page,collection,record,field)
);
CREATE INDEX IF NOT EXISTS idx_page_attachment_refs_id ON page_attachment_refs(attachment);
CREATE TABLE IF NOT EXISTS page_attachment_garbage (blob TEXT PRIMARY KEY);
`

type PageAttachmentRepo struct{ db *sql.DB }

func NewPageAttachmentRepo(db *sql.DB) *PageAttachmentRepo { return &PageAttachmentRepo{db} }

var _ domain.PageAttachmentRepository = (*PageAttachmentRepo)(nil)

const attachmentCols = `id,page,collection,name,blob,size,sha256,content_type,by_name,state,request_key,legacy_name,created_at,expires_at`

func scanAttachment(s scanner) (*domain.PageAttachment, error) {
	var a domain.PageAttachment
	var at, expires int64
	err := s.Scan(&a.ID, &a.Page, &a.Collection, &a.Name, &a.Blob, &a.Size, &a.SHA256, &a.ContentType, &a.By, &a.State, &a.Key, &a.LegacyName, &at, &expires)
	a.CreatedAt = fromMS(at)
	if expires != 0 {
		a.ExpiresAt = fromMS(expires)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.Errf(domain.CodeNotFound, "page attachment not found")
	}
	return &a, internal(err)
}
func attachmentUsage(ctx context.Context, q sqlQueryer, page string) (int, int64, error) {
	var n int
	var size int64
	err := q.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(size),0) FROM page_attachments WHERE page=?`, page).Scan(&n, &size)
	return n, size, internal(err)
}

type sqlQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (r *PageAttachmentRepo) Usage(ctx context.Context, page string) (int, int64, error) {
	return attachmentUsage(ctx, r.db, page)
}
func (r *PageAttachmentRepo) Reserve(ctx context.Context, a *domain.PageAttachment, max int, bytes int64, importing bool) (*domain.PageAttachment, bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, internal(err)
	}
	defer tx.Rollback()
	var exists int
	if err = tx.QueryRowContext(ctx, `SELECT 1 FROM pages WHERE id=?`, a.Page).Scan(&exists); err != nil {
		return nil, false, domain.Errf(domain.CodeNotFound, "page not found")
	}
	old, err := scanAttachment(tx.QueryRowContext(ctx, `SELECT `+attachmentCols+` FROM page_attachments WHERE page=? AND request_key=?`, a.Page, a.Key))
	if err == nil {
		expired := old.State != "ready" && !old.ExpiresAt.IsZero() && !old.ExpiresAt.After(a.CreatedAt)
		if expired {
			if err = queueAttachmentDelete(ctx, tx, old.ID); err != nil {
				return nil, false, err
			}
		} else {
			if old.Name != a.Name || old.Collection != a.Collection || old.Size != a.Size || old.SHA256 != a.SHA256 || old.ContentType != a.ContentType || old.LegacyName != a.LegacyName {
				return nil, false, domain.Errf(domain.CodeConflict, "upload key already used for different content")
			}
			return old, old.State == "uploading", nil
		}
	}
	if err != nil && !domain.IsCode(err, domain.CodeNotFound) {
		return nil, false, err
	}
	n, size, err := attachmentUsage(ctx, tx, a.Page)
	if err != nil {
		return nil, false, err
	}
	if !importing && (n >= max || size+a.Size > bytes) {
		return nil, false, domain.Errf(domain.CodeInvalidArgument, "page attachment quota exceeded: %d/%d files, %d/%d bytes", n+1, max, size+a.Size, bytes).WithSuggestion("remove unused page attachments or ask the Hub administrator to adjust its page attachment defaults")
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO page_attachments (`+attachmentCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, a.ID, a.Page, a.Collection, a.Name, a.Blob, a.Size, a.SHA256, a.ContentType, a.By, a.State, a.Key, a.LegacyName, ms(a.CreatedAt), ms(a.ExpiresAt))
	if err != nil {
		return nil, false, internal(err)
	}
	return a, true, internal(tx.Commit())
}
func (r *PageAttachmentRepo) Uploaded(ctx context.Context, id string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return internal(err)
	}
	defer tx.Rollback()
	a, err := scanAttachment(tx.QueryRowContext(ctx, `SELECT `+attachmentCols+` FROM page_attachments WHERE id=?`, id))
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE page_attachments SET state='pending' WHERE id=?`, id); err != nil {
		return internal(err)
	}
	if a.LegacyName != "" {
		var project string
		if err = tx.QueryRowContext(ctx, `SELECT project FROM pages WHERE id=?`, a.Page).Scan(&project); err != nil {
			return internal(err)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO page_attachment_aliases(page,project,name,attachment) VALUES(?,?,?,?) ON CONFLICT(page,name) DO UPDATE SET attachment=excluded.attachment,project=excluded.project`, a.Page, project, a.LegacyName, id); err != nil {
			return internal(err)
		}
		if err = rebindPageAttachments(ctx, tx, a.Page); err != nil {
			return err
		}
	}
	return internal(tx.Commit())
}
func (r *PageAttachmentRepo) Get(ctx context.Context, page, id string) (*domain.PageAttachment, error) {
	return scanAttachment(r.db.QueryRowContext(ctx, `SELECT `+attachmentCols+` FROM page_attachments WHERE page=? AND id=? AND state!='uploading'`, page, id))
}
func (r *PageAttachmentRepo) List(ctx context.Context, page, after string, limit int) ([]*domain.PageAttachment, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+attachmentCols+` FROM page_attachments WHERE page=? AND id>? AND state!='uploading' ORDER BY id LIMIT ?`, page, after, limit)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	out := []*domain.PageAttachment{}
	for rows.Next() {
		a, err := scanAttachment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, internal(rows.Err())
}
func queueAttachmentDelete(ctx context.Context, tx *sql.Tx, id string) error {
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO page_attachment_garbage(blob) SELECT blob FROM page_attachments WHERE id=?`, id); err != nil {
		return internal(err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM page_attachment_aliases WHERE attachment=?`, id); err != nil {
		return internal(err)
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM page_attachments WHERE id=?`, id)
	return internal(err)
}
func (r *PageAttachmentRepo) Remove(ctx context.Context, page, id string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return internal(err)
	}
	defer tx.Rollback()
	var n int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM page_attachment_refs WHERE attachment=?`, id).Scan(&n); err != nil {
		return internal(err)
	}
	if n > 0 {
		return domain.Errf(domain.CodeConflict, "attachment is still referenced by %d page records", n)
	}
	if _, err = scanAttachment(tx.QueryRowContext(ctx, `SELECT `+attachmentCols+` FROM page_attachments WHERE page=? AND id=?`, page, id)); err != nil {
		return err
	}
	if err = queueAttachmentDelete(ctx, tx, id); err != nil {
		return err
	}
	return internal(tx.Commit())
}
func (r *PageAttachmentRepo) Legacy(ctx context.Context, page, project, name string) ([]*domain.PageAttachment, error) {
	// A project rename/move cannot preserve access through an obsolete scope.
	rows, err := r.db.QueryContext(ctx, `SELECT `+prefixedAttachmentCols()+` FROM page_attachment_aliases x JOIN page_attachments a ON a.id=x.attachment JOIN pages p ON p.id=a.page WHERE x.name=? AND a.state!='uploading' AND (?='' OR a.page=?) AND (?='' OR p.project=?) ORDER BY a.id`, name, page, page, project, project)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	out := []*domain.PageAttachment{}
	for rows.Next() {
		a, e := scanAttachment(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, a)
	}
	return out, internal(rows.Err())
}
func prefixedAttachmentCols() string {
	return `a.id,a.page,a.collection,a.name,a.blob,a.size,a.sha256,a.content_type,a.by_name,a.state,a.request_key,a.legacy_name,a.created_at,a.expires_at`
}
func (r *PageAttachmentRepo) Sweep(ctx context.Context, now time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return internal(err)
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT id FROM page_attachments WHERE state!='ready' AND expires_at<=? AND NOT EXISTS(SELECT 1 FROM page_attachment_refs WHERE attachment=page_attachments.id)`, ms(now))
	if err != nil {
		return internal(err)
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return internal(err)
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return internal(err)
	}
	for _, id := range ids {
		if err = queueAttachmentDelete(ctx, tx, id); err != nil {
			return err
		}
	}
	return internal(tx.Commit())
}
func (r *PageAttachmentRepo) Garbage(ctx context.Context, limit int) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT blob FROM page_attachment_garbage ORDER BY blob LIMIT ?`, limit)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, internal(err)
		}
		out = append(out, id)
	}
	return out, internal(rows.Err())
}
func (r *PageAttachmentRepo) ForgetGarbage(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM page_attachment_garbage WHERE blob=?`, id)
	return internal(err)
}

// Called inside the record write transaction, closing the delete/link race.
func syncPageAttachmentRefs(ctx context.Context, tx *sql.Tx, rec *domain.PageRecord) error {
	var manifest string
	if err := tx.QueryRowContext(ctx, `SELECT manifest FROM pages WHERE id=?`, rec.Page).Scan(&manifest); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return internal(err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM page_attachment_refs WHERE page=? AND collection=? AND record=?`, rec.Page, rec.Collection, rec.ID); err != nil {
		return internal(err)
	}
	for _, ref := range domain.PageAttachmentReferences(manifest, rec.Collection, rec.Data) {
		id := ref.Value
		if ref.Legacy || !domain.IsPageAttachment(id) {
			err := tx.QueryRowContext(ctx, `SELECT attachment FROM page_attachment_aliases WHERE page=? AND name=?`, rec.Page, id).Scan(&id)
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			if err != nil {
				return internal(err)
			}
		}
		var state string
		var expires int64
		if err := tx.QueryRowContext(ctx, `SELECT state,expires_at FROM page_attachments WHERE page=? AND id=?`, rec.Page, id).Scan(&state, &expires); err != nil || state == "uploading" || (expires != 0 && expires <= ms(rec.UpdatedAt)) {
			return domain.Errf(domain.CodeInvalidArgument, "file field %s must refer to an uploaded attachment of this page", ref.Field)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO page_attachment_refs(page,collection,record,field,attachment) VALUES(?,?,?,?,?)`, rec.Page, rec.Collection, rec.ID, ref.Field, id); err != nil {
			return internal(err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE page_attachments SET state='ready',expires_at=0 WHERE id=?`, id); err != nil {
			return internal(err)
		}
	}
	return nil
}
func rebindPageAttachments(ctx context.Context, tx *sql.Tx, page string) error {
	rows, err := tx.QueryContext(ctx, `SELECT `+recordCols+` FROM page_records WHERE page=?`, page)
	if err != nil {
		return internal(err)
	}
	recs := []*domain.PageRecord{}
	for rows.Next() {
		r, e := scanRecord(rows)
		if e != nil {
			rows.Close()
			return e
		}
		recs = append(recs, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return internal(err)
	}
	for _, rec := range recs {
		if err = syncPageAttachmentRefs(ctx, tx, rec); err != nil {
			return err
		}
	}
	return nil
}
