package dbstudio

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
)

func text(c Cell) string { return c.Value }
func (c Connection) Databases(ctx context.Context) ([]string, error) {
	if c.Engine == "sqlite" {
		return []string{"main"}, nil
	}
	if c.Engine == "mongodb" {
		client, err := c.mongoOpen(ctx)
		if err != nil {
			return nil, err
		}
		defer client.Disconnect(context.Background())
		names, err := client.ListDatabaseNames(ctx, map[string]any{})
		return names, c.redact(err)
	}
	db, err := c.sqlOpen(ctx, "")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	query := "SHOW DATABASES"
	if c.Engine == "postgres" {
		query = "SELECT datname FROM pg_database WHERE datallowconn AND has_database_privilege(datname, 'CONNECT') ORDER BY datname"
	}
	out, err := readRows(ctx, db, query, nil, 1000)
	if err != nil {
		return nil, c.redact(err)
	}
	names := []string{}
	for _, r := range out.Rows {
		names = append(names, text(r.Values[0]))
	}
	return names, nil
}
func (c Connection) Tables(ctx context.Context, r Request) ([]Table, error) {
	if c.Engine == "mongodb" {
		client, err := c.mongoOpen(ctx)
		if err != nil {
			return nil, err
		}
		defer client.Disconnect(context.Background())
		names, err := client.Database(c.database(r.Database)).ListCollectionNames(ctx, map[string]any{})
		if err != nil {
			return nil, c.redact(err)
		}
		out := []Table{}
		for _, name := range names {
			out = append(out, Table{Name: name, Kind: "collection"})
		}
		return out, nil
	}
	db, err := c.sqlOpen(ctx, r.Database)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	query := "SELECT 'main',name,type FROM sqlite_schema WHERE type IN ('table','view') AND name NOT LIKE 'sqlite_%' ORDER BY name"
	var args []any
	if c.Engine == "mysql" {
		query = "SELECT TABLE_SCHEMA,TABLE_NAME,TABLE_TYPE FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() ORDER BY TABLE_NAME"
	} else if c.Engine == "postgres" {
		query = "SELECT n.nspname,c.relname,CASE WHEN c.relkind IN ('v','m') THEN 'view' ELSE 'table' END FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE c.relkind IN ('r','p','v','m','f') AND n.nspname NOT IN ('pg_catalog','information_schema') AND n.nspname NOT LIKE 'pg_toast%' ORDER BY n.nspname,c.relname"
	}
	out, err := readRows(ctx, db, query, args, 1000)
	if err != nil {
		return nil, c.redact(err)
	}
	tables := []Table{}
	for _, row := range out.Rows {
		kind := strings.ToLower(text(row.Values[2]))
		if kind == "base table" {
			kind = "table"
		}
		tables = append(tables, Table{Schema: text(row.Values[0]), Name: text(row.Values[1]), Kind: kind})
	}
	return tables, nil
}
func (c Connection) Columns(ctx context.Context, r Request) ([]Column, error) {
	if c.Engine == "mongodb" {
		return nil, failure("unsupported", "MongoDB collections are schema-flexible; inspect a document")
	}
	db, err := c.sqlOpen(ctx, r.Database)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return c.columns(ctx, db, r)
}
func (c Connection) columns(ctx context.Context, db *sql.DB, r Request) ([]Column, error) {
	if _, err := ident(r.Table, c.Engine); err != nil {
		return nil, err
	}
	var query string
	var args []any
	switch c.Engine {
	case "sqlite":
		q, _ := ident(r.Table, c.Engine)
		query = "PRAGMA table_xinfo(" + q + ")"
	case "mysql":
		query = "SELECT COLUMN_NAME,COLUMN_TYPE,IS_NULLABLE,COALESCE(COLUMN_DEFAULT,''),COLUMN_KEY,EXTRA FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? ORDER BY ORDINAL_POSITION"
		args = []any{r.Table}
	case "postgres":
		schema := r.Schema
		if schema == "" {
			schema = "public"
		}
		query = `SELECT c.column_name,c.data_type,c.is_nullable,COALESCE(c.column_default,''),EXISTS(SELECT 1 FROM information_schema.key_column_usage k JOIN information_schema.table_constraints t ON t.constraint_catalog=k.constraint_catalog AND t.constraint_schema=k.constraint_schema AND t.constraint_name=k.constraint_name WHERE t.constraint_type='PRIMARY KEY' AND k.table_schema=c.table_schema AND k.table_name=c.table_name AND k.column_name=c.column_name),c.is_generated,c.is_identity FROM information_schema.columns c WHERE c.table_schema=$1 AND c.table_name=$2 ORDER BY c.ordinal_position`
		args = []any{schema, r.Table}
	}
	out, err := readRows(ctx, db, query, args, 512)
	if err != nil {
		return nil, c.redact(err)
	}
	columns := []Column{}
	for _, row := range out.Rows {
		v := row.Values
		col := Column{}
		switch c.Engine {
		case "sqlite":
			if len(v) < 7 {
				continue
			}
			if v[6].Value == "1" {
				continue
			}
			col = Column{Name: v[1].Value, Type: v[2].Value, Nullable: v[3].Value != "1", Default: v[4].Value, Primary: v[5].Value != "0", Generated: v[6].Value != "0"}
			if col.Primary && strings.EqualFold(col.Type, "INTEGER") {
				col.Default = "auto"
			}
		case "mysql":
			col = Column{Name: v[0].Value, Type: v[1].Value, Nullable: v[2].Value == "YES", Default: v[3].Value, Primary: v[4].Value == "PRI", Generated: strings.Contains(strings.ToUpper(v[5].Value), "GENERATED")}
			if strings.Contains(v[5].Value, "auto_increment") {
				col.Default = "auto"
			}
		case "postgres":
			col = Column{Name: v[0].Value, Type: v[1].Value, Nullable: v[2].Value == "YES", Default: v[3].Value, Primary: v[4].Value == "true", Generated: v[5].Value != "NEVER" || v[6].Value == "YES"}
		}
		columns = append(columns, col)
	}
	if len(columns) == 0 {
		return nil, failure("not_found", "table or columns were not found")
	}
	return columns, nil
}
func (c Connection) database(name string) string {
	if name != "" {
		return name
	}
	if c.Database != "" {
		return c.Database
	}
	return "test"
}
func (c Connection) Browse(ctx context.Context, r Request) (*Result, error) {
	db, err := c.sqlOpen(ctx, r.Database)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	columns, err := c.columns(ctx, db, r)
	if err != nil {
		return nil, err
	}
	table, err := tableIdent(r, c.Engine)
	if err != nil {
		return nil, err
	}
	if r.Offset < 0 || r.Offset > 10_000_000 {
		return nil, failure("invalid_argument", "offset must be between 0 and 10000000")
	}
	query := "SELECT * FROM " + table
	order := []string{}
	if r.OrderBy != "" {
		found := false
		for _, col := range columns {
			if col.Name == r.OrderBy {
				found = true
				q, _ := ident(col.Name, c.Engine)
				order = append(order, q)
			}
		}
		if !found {
			return nil, failure("invalid_argument", "sort column not found")
		}
	} else {
		for _, col := range columns {
			if col.Primary {
				q, _ := ident(col.Name, c.Engine)
				order = append(order, q)
			}
		}
	}
	if len(order) > 0 {
		query += " ORDER BY " + strings.Join(order, ",")
		if r.Descending {
			query += " DESC"
		}
	}
	query += " LIMIT " + strconv.Itoa(limit(r.Limit)+1) + " OFFSET " + strconv.Itoa(r.Offset)
	out, err := readRows(ctx, db, query, nil, limit(r.Limit))
	if err != nil {
		return nil, c.redact(err)
	}
	out.Columns = columns
	hasKey := false
	for _, col := range columns {
		hasKey = hasKey || col.Primary
	}
	out.ReadOnly = c.ReadOnly || !hasKey
	if c.Engine == "mysql" {
		transactional, err := mysqlTransactional(ctx, db, r.Table)
		if err != nil {
			return nil, c.redact(err)
		}
		out.ReadOnly = out.ReadOnly || !transactional
	}
	for i := range out.Rows {
		row := &out.Rows[i]
		if len(row.Values) != len(columns) {
			return nil, failure("conflict", "table structure changed; reload the table")
		}
		row.Token = rowToken(columns, row.Values)
		row.Key = map[string]Cell{}
		for j, col := range columns {
			if col.Primary {
				row.Key[col.Name] = row.Values[j]
			}
		}
	}
	return out, nil
}
func mysqlTransactional(ctx context.Context, db *sql.DB, table string) (bool, error) {
	var engine sql.NullString
	err := db.QueryRowContext(ctx, "SELECT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=?", table).Scan(&engine)
	return strings.EqualFold(engine.String, "InnoDB"), err
}
