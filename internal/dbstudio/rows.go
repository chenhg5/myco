package dbstudio

import (
	"context"
	"database/sql"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Mutate locks a keyed row, compares the complete original snapshot, then changes
// exactly that row. Parameter values never become part of an SQL string.
func (c Connection) Mutate(ctx context.Context, r Request) (*Result, error) {
	start := time.Now()
	if c.ReadOnly {
		return nil, failure("permission_denied", "this connection is read-only")
	}
	if r.Action != "insert" && r.Action != "update" && r.Action != "delete" {
		return nil, failure("invalid_argument", "unknown row operation")
	}
	db, err := c.sqlOpen(ctx, r.Database)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if c.Engine == "mysql" {
		transactional, err := mysqlTransactional(ctx, db, r.Table)
		if err != nil {
			return nil, c.redact(err)
		}
		if !transactional {
			return nil, failure("unsupported", "row editing needs a transactional InnoDB table; use explicit SQL for this table")
		}
	}
	columns, err := c.columns(ctx, db, r)
	if err != nil {
		return nil, err
	}
	table, err := tableIdent(r, c.Engine)
	if err != nil {
		return nil, err
	}
	known := map[string]Column{}
	keys := []string{}
	for _, col := range columns {
		known[col.Name] = col
		if col.Primary {
			keys = append(keys, col.Name)
		}
	}
	if r.Action != "insert" && len(keys) == 0 {
		return nil, failure("permission_denied", "a primary key is needed to edit or delete rows")
	}
	if len(r.Values) > len(columns) {
		return nil, failure("invalid_argument", "unknown column")
	}
	names := []string{}
	for name := range r.Values {
		col, ok := known[name]
		if !ok || col.Generated {
			return nil, failure("invalid_argument", "unknown or generated column cannot be edited")
		}
		names = append(names, name)
	}
	sort.Strings(names)
	if r.Action == "update" && len(names) == 0 {
		return nil, failure("invalid_argument", "no changed columns")
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, c.redact(err)
	}
	defer tx.Rollback()
	where := []string{}
	keyArgs := []any{}
	if r.Action != "insert" {
		if len(r.Key) != len(keys) || r.Token == "" {
			return nil, failure("invalid_argument", "the row key and original snapshot are required")
		}
		for _, name := range keys {
			v, ok := r.Key[name]
			if !ok {
				return nil, failure("invalid_argument", "incomplete primary key")
			}
			arg, x := v.argument(c.Engine)
			if x != nil {
				return nil, x
			}
			q, _ := ident(name, c.Engine)
			if v.Kind == "null" {
				where = append(where, q+" IS NULL")
			} else {
				keyArgs = append(keyArgs, arg)
				where = append(where, q+"="+placeholder(c.Engine, len(keyArgs)))
			}
		}
		query := "SELECT * FROM " + table + " WHERE " + strings.Join(where, " AND ")
		if c.Engine != "sqlite" {
			query += " FOR UPDATE"
		}
		before, x := readRows(ctx, tx, query, keyArgs, 2)
		if x != nil {
			return nil, c.redact(x)
		}
		if len(before.Rows) != 1 || rowToken(columns, before.Rows[0].Values) != r.Token {
			return nil, failure("conflict", "this row changed or was deleted; reload it before editing")
		}
	}
	var statement string
	args := []any{}
	if r.Action == "insert" {
		if len(names) == 0 {
			statement = "INSERT INTO " + table + " DEFAULT VALUES"
			if c.Engine == "mysql" {
				statement = "INSERT INTO " + table + " () VALUES ()"
			}
		} else {
			qs := []string{}
			marks := []string{}
			for _, name := range names {
				q, _ := ident(name, c.Engine)
				arg, x := r.Values[name].argument(c.Engine)
				if x != nil {
					return nil, x
				}
				args = append(args, arg)
				qs = append(qs, q)
				marks = append(marks, placeholder(c.Engine, len(args)))
			}
			statement = "INSERT INTO " + table + " (" + strings.Join(qs, ",") + ") VALUES (" + strings.Join(marks, ",") + ")"
		}
	} else if r.Action == "delete" {
		statement = "DELETE FROM " + table + " WHERE " + strings.Join(where, " AND ")
		args = keyArgs
	} else {
		sets := []string{}
		for _, name := range names {
			q, _ := ident(name, c.Engine)
			arg, x := r.Values[name].argument(c.Engine)
			if x != nil {
				return nil, x
			}
			args = append(args, arg)
			sets = append(sets, q+"="+placeholder(c.Engine, len(args)))
		}
		updatedWhere := []string{}
		for _, name := range keys {
			q, _ := ident(name, c.Engine)
			v := r.Key[name]
			if v.Kind == "null" {
				updatedWhere = append(updatedWhere, q+" IS NULL")
			} else {
				arg, _ := v.argument(c.Engine)
				args = append(args, arg)
				updatedWhere = append(updatedWhere, q+"="+placeholder(c.Engine, len(args)))
			}
		}
		statement = "UPDATE " + table + " SET " + strings.Join(sets, ",") + " WHERE " + strings.Join(updatedWhere, " AND ")
	}
	result, err := tx.ExecContext(ctx, statement, args...)
	if err != nil {
		return nil, c.writeError(err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, c.redact(err)
	}
	if r.Action != "insert" && affected > 1 {
		return nil, failure("conflict", "more than one row matched; no changes were committed")
	}
	if err = tx.Commit(); err != nil {
		return nil, failure("outcome_unknown", "commit was not acknowledged; inspect the database before retrying")
	}
	return &Result{Columns: []Column{}, Rows: []Row{}, Affected: strconv.FormatInt(affected, 10), DurationMS: time.Since(start).Milliseconds()}, nil
}
