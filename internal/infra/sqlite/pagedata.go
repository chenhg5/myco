package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/chenhg5/myco/internal/domain"
)

// PageDataRepo implements domain.PageDataRepository.
type PageDataRepo struct{ db *sql.DB }

// NewPageDataRepo returns a SQLite-backed store of pages' records.
func NewPageDataRepo(db *sql.DB) *PageDataRepo { return &PageDataRepo{db: db} }

var _ domain.PageDataRepository = (*PageDataRepo)(nil)

const recordCols = `page, collection, id, data, version, updated_by, updated_at, pos`

func scanRecord(s scanner) (*domain.PageRecord, error) {
	var (
		r  domain.PageRecord
		at int64
	)
	if err := s.Scan(&r.Page, &r.Collection, &r.ID, &r.Data, &r.Version, &r.UpdatedBy, &at, &r.Pos); err != nil {
		return nil, err
	}
	r.UpdatedAt = fromMS(at)
	return &r, nil
}

func (r *PageDataRepo) Records(ctx context.Context, page, collection string) ([]*domain.PageRecord, error) {
	q := `SELECT ` + recordCols + ` FROM page_records WHERE page=?`
	args := []any{page}
	if collection != "" {
		q += ` AND collection=?`
		args = append(args, collection)
	}
	rows, err := r.db.QueryContext(ctx, q+` ORDER BY collection, pos`, args...)
	if err != nil {
		return nil, internal(err)
	}
	defer rows.Close()
	out := []*domain.PageRecord{}
	for rows.Next() {
		rec, err := scanRecord(rows)
		if err != nil {
			return nil, internal(err)
		}
		out = append(out, rec)
	}
	return out, internal(rows.Err())
}

func (r *PageDataRepo) Record(ctx context.Context, page, collection, id string) (*domain.PageRecord, error) {
	rec, err := scanRecord(r.db.QueryRowContext(ctx, `SELECT `+recordCols+` FROM page_records WHERE page=? AND collection=? AND id=?`, page, collection, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.Errf(domain.CodeNotFound, "no record %s in %s", id, collection)
	}
	return rec, internal(err)
}

func conflict(collection, id string, ifVersion int) error {
	if ifVersion == 0 {
		return domain.Errf(domain.CodeConflict, "%s already has a record %s", collection, id).
			WithSuggestion("read it and pass the version you read (--if-version), or leave the id out for a new record")
	}
	return domain.Errf(domain.CodeConflict, "record %s in %s changed (it is no longer version %d)", id, collection, ifVersion).
		WithSuggestion("read it again and apply your change to the latest")
}

func (r *PageDataRepo) Put(ctx context.Context, rec *domain.PageRecord, ifVersion int) (*domain.PageChange, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, internal(err)
	}
	defer tx.Rollback()
	var cur, pos int64
	err = tx.QueryRowContext(ctx, `SELECT version, pos FROM page_records WHERE page=? AND collection=? AND id=?`, rec.Page, rec.Collection, rec.ID).Scan(&cur, &pos)
	exists := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, internal(err)
	}
	if ifVersion >= 0 && int64(ifVersion) != cur {
		return nil, conflict(rec.Collection, rec.ID, ifVersion)
	}
	rec.Version = int(cur) + 1
	if exists {
		rec.Pos = pos
		_, err = tx.ExecContext(ctx, `UPDATE page_records SET data=?, version=?, updated_by=?, updated_at=? WHERE page=? AND collection=? AND id=?`,
			rec.Data, rec.Version, rec.UpdatedBy, ms(rec.UpdatedAt), rec.Page, rec.Collection, rec.ID)
	} else {
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(pos), 0) + 1 FROM page_records WHERE page=? AND collection=?`, rec.Page, rec.Collection).Scan(&rec.Pos); err != nil {
			return nil, internal(err)
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO page_records (`+recordCols+`) VALUES (?,?,?,?,?,?,?,?)`,
			rec.Page, rec.Collection, rec.ID, rec.Data, rec.Version, rec.UpdatedBy, ms(rec.UpdatedAt), rec.Pos)
	}
	if err != nil {
		return nil, internal(err)
	}
	ch := &domain.PageChange{Page: rec.Page, Collection: rec.Collection, Record: rec.ID, Op: "put", Data: rec.Data, Version: rec.Version, By: rec.UpdatedBy, At: rec.UpdatedAt}
	if err := addChange(ctx, tx, ch); err != nil {
		return nil, err
	}
	return ch, internal(tx.Commit())
}

func (r *PageDataRepo) Remove(ctx context.Context, page, collection, id string, ifVersion int, by string, at time.Time) (*domain.PageChange, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, internal(err)
	}
	defer tx.Rollback()
	var cur int
	err = tx.QueryRowContext(ctx, `SELECT version FROM page_records WHERE page=? AND collection=? AND id=?`, page, collection, id).Scan(&cur)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.Errf(domain.CodeNotFound, "no record %s in %s", id, collection)
	}
	if err != nil {
		return nil, internal(err)
	}
	if ifVersion >= 0 && ifVersion != cur {
		return nil, conflict(collection, id, ifVersion)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM page_records WHERE page=? AND collection=? AND id=?`, page, collection, id); err != nil {
		return nil, internal(err)
	}
	ch := &domain.PageChange{Page: page, Collection: collection, Record: id, Op: "delete", Version: cur + 1, By: by, At: at}
	if err := addChange(ctx, tx, ch); err != nil {
		return nil, err
	}
	return ch, internal(tx.Commit())
}

func addChange(ctx context.Context, tx *sql.Tx, ch *domain.PageChange) error {
	res, err := tx.ExecContext(ctx, `INSERT INTO page_changes (page, collection, record, op, data, version, by_name, at) VALUES (?,?,?,?,?,?,?,?)`,
		ch.Page, ch.Collection, ch.Record, ch.Op, ch.Data, ch.Version, ch.By, ms(ch.At))
	if err != nil {
		return internal(err)
	}
	ch.Seq, err = res.LastInsertId()
	return internal(err)
}

func (r *PageDataRepo) Changes(ctx context.Context, page string, after int64, limit int) ([]*domain.PageChange, int64, error) {
	var oldest sql.NullInt64
	if err := r.db.QueryRowContext(ctx, `SELECT MIN(seq) FROM page_changes WHERE page=?`, page).Scan(&oldest); err != nil {
		return nil, 0, internal(err)
	}
	rows, err := r.db.QueryContext(ctx, `SELECT seq, page, collection, record, op, data, version, by_name, at FROM page_changes
		WHERE page=? AND seq>? ORDER BY seq LIMIT ?`, page, after, limit)
	if err != nil {
		return nil, 0, internal(err)
	}
	defer rows.Close()
	out := []*domain.PageChange{}
	for rows.Next() {
		var (
			c  domain.PageChange
			at int64
		)
		if err := rows.Scan(&c.Seq, &c.Page, &c.Collection, &c.Record, &c.Op, &c.Data, &c.Version, &c.By, &at); err != nil {
			return nil, 0, internal(err)
		}
		c.At = fromMS(at)
		out = append(out, &c)
	}
	return out, oldest.Int64, internal(rows.Err())
}

func (r *PageDataRepo) Seq(ctx context.Context, page string) (int64, error) {
	var seq sql.NullInt64
	err := r.db.QueryRowContext(ctx, `SELECT MAX(seq) FROM page_changes WHERE page=?`, page).Scan(&seq)
	return seq.Int64, internal(err)
}

func (r *PageDataRepo) Count(ctx context.Context, page string) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM page_records WHERE page=?`, page).Scan(&n)
	return n, internal(err)
}

func (r *PageDataRepo) Trim(ctx context.Context, page string, keep int) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM page_changes WHERE page=? AND seq <= (
		SELECT seq FROM page_changes WHERE page=? ORDER BY seq DESC LIMIT 1 OFFSET ?)`, page, page, keep)
	return internal(err)
}

func (r *PageDataRepo) DeletePage(ctx context.Context, page string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM page_records WHERE page=?`, page); err != nil {
		return internal(err)
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM page_changes WHERE page=?`, page)
	return internal(err)
}
