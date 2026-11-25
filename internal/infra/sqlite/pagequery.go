package sqlite

import (
	"context"
	"database/sql"
	"strconv"
	"strings"

	"github.com/myco-ai/myco/internal/domain"
)

// Paths are literal only after the domain parser validates flat ASCII field
// names; all values remain bound parameters. Literal paths let SQLite use
// expression indexes (bound JSON paths would not match those indexes).
func jsonField(field string) string { return "json_extract(data,'$." + field + "')" }
func jsonKind(field string) string  { return "json_type(data,'$." + field + "')" }
func queryNumber(v domain.RecordValue) any {
	if n, e := strconv.ParseInt(v.Text, 10, 64); e == nil {
		return n
	}
	n, _ := strconv.ParseFloat(v.Text, 64)
	return n
}
func equalSQL(field string, v domain.RecordValue) (string, []any) {
	k, x := jsonKind(field), jsonField(field)
	switch v.Type {
	case "null", "true", "false":
		return k + "=?", []any{v.Type}
	case "number":
		return "(" + k + " IN ('integer','real') AND " + x + "=?)", []any{queryNumber(v)}
	default:
		return "(" + k + "='text' AND " + x + "=?)", []any{v.Text}
	}
}
func pageQueryWhere(page, coll string, q domain.PageQuery) (string, []any) {
	parts := []string{"page=?"}
	args := []any{page}
	if coll != "" {
		parts = append(parts, "collection=?")
		args = append(args, coll)
	}
	for _, f := range q.Filters {
		k, x := jsonKind(f.Field), jsonField(f.Field)
		switch f.Op {
		case "exists":
			op := "IS NOT NULL"
			if f.Values[0].Type == "false" {
				op = "IS NULL"
			}
			parts = append(parts, k+" "+op)
		case "eq", "ne", "in":
			clauses := []string{}
			for _, v := range f.Values {
				expr, values := equalSQL(f.Field, v)
				clauses = append(clauses, expr)
				args = append(args, values...)
			}
			expr := "(" + strings.Join(clauses, " OR ") + ")"
			if f.Op == "ne" {
				expr = "(" + k + " IS NOT NULL AND NOT " + expr + ")"
			}
			parts = append(parts, expr)
		default:
			op := map[string]string{"gt": ">", "gte": ">=", "lt": "<", "lte": "<="}[f.Op]
			v := f.Values[0]
			if v.Type == "number" {
				parts = append(parts, "("+k+" IN ('integer','real') AND "+x+op+"?)")
				args = append(args, queryNumber(v))
			} else {
				// Accept ISO dates and RFC3339 instants, not numeric-looking strings
				// which SQLite would otherwise interpret as a Julian day number.
				dateShape := "(" + x + " GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]' OR (" + x + " GLOB '????-??-??T??:??:??*' AND (" + x + " GLOB '*Z' OR " + x + " GLOB '*[+-][0-9][0-9]:[0-9][0-9]')))"
				parts = append(parts, "("+k+"='text' AND "+dateShape+" AND (length("+x+")=10 OR CAST(substr("+x+",12,2) AS INTEGER)<24) AND strftime('%Y-%m-%d',substr("+x+",1,10))=substr("+x+",1,10) AND julianday("+x+")"+op+"julianday(?))")
				args = append(args, v.Text)
			}
		}
	}
	return strings.Join(parts, " AND "), args
}

// Query reads the sequence and rows in one SQLite snapshot. A cursor is tied
// to that sequence so edits between pages produce a restartable conflict,
// instead of silently losing or duplicating records with offset pagination.
func (r *PageDataRepo) Query(ctx context.Context, page, coll string, q domain.PageQuery) (domain.RecordQueryResult, error) {
	out := domain.RecordQueryResult{Records: []*domain.PageRecord{}}
	if err := q.Validate(); err != nil {
		return out, err
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, internal(err)
	}
	defer tx.Rollback()
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq),0) FROM page_changes WHERE page=?`, page).Scan(&out.Seq); err != nil {
		return out, internal(err)
	}
	offset, err := q.Offset(page, coll, out.Seq)
	if err != nil {
		return out, err
	}
	where, args := pageQueryWhere(page, coll, q)
	if q.Count {
		var n int
		if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM page_records WHERE "+where, args...).Scan(&n); err != nil {
			return out, internal(err)
		}
		out.Count = &n
		return out, internal(tx.Commit())
	}
	order := []string{}
	for _, s := range q.Sort {
		direction := " ASC"
		if s.Desc {
			direction = " DESC"
		}
		order = append(order, "("+jsonKind(s.Field)+" IS NULL) ASC", jsonField(s.Field)+direction)
	}
	order = append(order, "collection ASC", "pos ASC", "id ASC")
	query := "SELECT " + recordCols + " FROM page_records WHERE " + where + " ORDER BY " + strings.Join(order, ",")
	if q.Limit > 0 {
		query += " LIMIT ? OFFSET ?"
		args = append(args, q.Limit+1, offset)
	} else if offset > 0 {
		query += " LIMIT -1 OFFSET ?"
		args = append(args, offset)
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return out, internal(err)
	}
	for rows.Next() {
		rec, e := scanRecord(rows)
		if e != nil {
			rows.Close()
			return out, internal(e)
		}
		out.Records = append(out.Records, domain.ProjectPageRecord(rec, q.Fields))
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, internal(err)
	}
	if q.Limit > 0 && len(out.Records) > q.Limit {
		out.Records = out.Records[:q.Limit]
		out.Next = q.Cursor(page, coll, out.Seq, offset+q.Limit)
	}
	return out, internal(tx.Commit())
}
