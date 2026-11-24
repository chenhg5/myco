package dbstudio

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

func (c Connection) sqlOpen(ctx context.Context, database string) (*sql.DB, error) {
	if database == "" {
		database = c.Database
	}
	if len(database) > 256 || strings.ContainsRune(database, 0) {
		return nil, failure("invalid_argument", "invalid database name")
	}
	var driver, dsn string
	switch c.Engine {
	case "mysql":
		cfg := mysql.NewConfig()
		cfg.User = c.User
		cfg.Passwd = c.secret.Password
		cfg.Net = "tcp"
		cfg.Addr = net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
		cfg.DBName = database
		cfg.Timeout = 10 * time.Second
		cfg.ReadTimeout = 125 * time.Second
		cfg.WriteTimeout = 125 * time.Second
		cfg.MultiStatements = false
		cfg.AllowNativePasswords = true
		if c.TLS == "verify" {
			cfg.TLSConfig = "true"
		} else if c.TLS == "require" {
			cfg.TLSConfig = "skip-verify"
		}
		driver, dsn = "mysql", cfg.FormatDSN()
	case "postgres":
		if database == "" {
			database = "postgres"
		}
		u := url.URL{Scheme: "postgres", Host: net.JoinHostPort(c.Host, strconv.Itoa(c.Port)), Path: "/" + database, User: url.UserPassword(c.User, c.secret.Password)}
		q := url.Values{"connect_timeout": {"10"}, "standard_conforming_strings": {"on"}, "application_name": {"myco-database"}}
		mode := "verify-full"
		if c.TLS == "disable" {
			mode = "disable"
		} else if c.TLS == "require" {
			mode = "require"
		}
		q.Set("sslmode", mode)
		u.RawQuery = q.Encode()
		driver, dsn = "postgres", u.String()
	case "sqlite":
		if _, err := os.Stat(c.Path); err != nil {
			return nil, failure("not_found", "SQLite file not found; choose an existing database file")
		}
		mode := "rw"
		if c.ReadOnly {
			mode = "ro"
		}
		path := strings.ReplaceAll(c.Path, `\`, "/")
		if filepath.VolumeName(c.Path) != "" && !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		u := url.URL{Scheme: "file", Path: path}
		q := url.Values{"mode": {mode}, "_pragma": {"busy_timeout(5000)", "foreign_keys(ON)"}}
		u.RawQuery = q.Encode()
		driver, dsn = "sqlite", u.String()
	default:
		return nil, failure("unsupported", "this operation needs a SQL connection")
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, c.redact(err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err = db.PingContext(ctx); err != nil {
		db.Close()
		return nil, c.redact(err)
	}
	return db, nil
}
func (c Connection) redact(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := err.(*Error); ok {
		return err
	}
	message := err.Error()
	secrets := []string{c.secret.URI, c.secret.Password, url.QueryEscape(c.secret.Password), url.PathEscape(c.secret.Password)}
	if u, err := url.Parse(c.secret.URI); err == nil && u.User != nil {
		pw, _ := u.User.Password()
		secrets = append(secrets, pw, url.QueryEscape(pw), url.PathEscape(pw))
	}
	for _, secret := range secrets {
		if secret != "" {
			message = strings.ReplaceAll(message, secret, "[redacted]")
		}
	}
	if len(message) > 1000 {
		message = message[:1000]
	}
	return failure("failed", message)
}
func (c Connection) writeError(err error) error {
	var network net.Error
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, driver.ErrBadConn) || errors.Is(err, mysql.ErrInvalidConn) || errors.As(err, &network) {
		return failure("outcome_unknown", "write ended without acknowledgement; inspect the database before retrying")
	}
	return c.redact(err)
}
func sqlCell(v any, typ string) Cell {
	if v == nil {
		return Cell{Kind: "null"}
	}
	switch x := v.(type) {
	case int64:
		return Cell{Kind: "number", Value: strconv.FormatInt(x, 10)}
	case float64:
		return Cell{Kind: "number", Value: strconv.FormatFloat(x, 'g', -1, 64)}
	case bool:
		return Cell{Kind: "bool", Value: strconv.FormatBool(x)}
	case time.Time:
		return Cell{Kind: "text", Value: x.Format(time.RFC3339Nano)}
	case []byte:
		typ = strings.ToUpper(typ)
		if strings.Contains(typ, "BLOB") || strings.Contains(typ, "BINARY") || typ == "BYTEA" || !utf8.Valid(x) {
			return Cell{Kind: "binary", Value: base64.StdEncoding.EncodeToString(x)}
		}
		kind := "text"
		if typ == "JSON" || typ == "JSONB" {
			kind = "json"
		} else if strings.Contains(typ, "INT") || strings.Contains(typ, "DECIMAL") || strings.Contains(typ, "NUMERIC") || strings.Contains(typ, "FLOAT") || strings.Contains(typ, "DOUBLE") {
			kind = "number"
		}
		return Cell{Kind: kind, Value: string(x)}
	default:
		return Cell{Kind: "text", Value: fmt.Sprint(v)}
	}
}
func (v Cell) argument(engine string) (any, error) {
	if len(v.Value) > maxCell {
		return nil, failure("too_large", "cell exceeds 512 KiB")
	}
	switch v.Kind {
	case "null":
		return nil, nil
	case "text", "json":
		return v.Value, nil
	case "binary":
		b, err := base64.StdEncoding.DecodeString(v.Value)
		if err != nil {
			return nil, failure("invalid_argument", "invalid base64 cell")
		}
		return b, nil
	case "bool":
		b, err := strconv.ParseBool(v.Value)
		if err != nil {
			return nil, failure("invalid_argument", "invalid boolean cell")
		}
		return b, nil
	case "number":
		if n, err := strconv.ParseInt(v.Value, 10, 64); err == nil {
			return n, nil
		}
		f, err := strconv.ParseFloat(v.Value, 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, failure("invalid_argument", "invalid numeric cell")
		}
		if engine == "sqlite" {
			return f, nil
		}
		return v.Value, nil
	}
	return nil, failure("invalid_argument", "invalid cell type")
}

type sqlQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func readRows(ctx context.Context, q sqlQuerier, statement string, args []any, n int) (*Result, error) {
	r, err := q.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	types, err := r.ColumnTypes()
	if err != nil {
		return nil, err
	}
	if len(types) > 512 {
		return nil, failure("too_large", "select at most 512 columns")
	}
	out := &Result{Columns: []Column{}, Rows: []Row{}, ReadOnly: true}
	for _, t := range types {
		nullable, _ := t.Nullable()
		out.Columns = append(out.Columns, Column{Name: t.Name(), Type: t.DatabaseTypeName(), Nullable: nullable})
	}
	size := 0
	for r.Next() {
		if len(out.Rows) >= n {
			out.Truncated = true
			break
		}
		values := make([]any, len(types))
		dest := make([]any, len(types))
		for i := range values {
			dest[i] = &values[i]
		}
		if err = r.Scan(dest...); err != nil {
			return nil, err
		}
		row := Row{Values: make([]Cell, len(types))}
		for i, v := range values {
			row.Values[i] = sqlCell(v, types[i].DatabaseTypeName())
			if len(row.Values[i].Value) > maxCell {
				return nil, failure("too_large", "a result cell exceeds 512 KiB; select a smaller projection")
			}
		}
		b, _ := json.Marshal(row)
		if size+len(b) > maxResult {
			out.Truncated = true
			break
		}
		size += len(b)
		out.Rows = append(out.Rows, row)
	}
	if err = r.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
func (c Connection) SQL(ctx context.Context, r Request) (*Result, error) {
	start := time.Now()
	if r.Mode != "query" && r.Mode != "execute" {
		return nil, failure("invalid_argument", "mode must be query or execute")
	}
	if r.Mode == "execute" && c.ReadOnly {
		return nil, failure("permission_denied", "this connection is read-only")
	}
	word, err := singleStatement(r.SQL, c.Engine)
	if err != nil {
		return nil, err
	}
	if r.Mode == "execute" {
		switch word {
		case "BEGIN", "START", "COMMIT", "END", "ROLLBACK", "ABORT", "SAVEPOINT", "RELEASE", "USE", "SET":
			return nil, failure("unsupported", "each query uses an independent session; manual transaction/session commands are not supported")
		}
	}
	if r.Mode == "query" {
		if err := queryStatement(r.SQL, c.Engine); err != nil {
			return nil, err
		}
	}
	db, err := c.sqlOpen(ctx, r.Database)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if r.Mode == "execute" {
		value, err := db.ExecContext(ctx, r.SQL)
		if err != nil {
			if ctx.Err() != nil {
				return nil, failure("outcome_unknown", "execution ended without acknowledgement; inspect the database before retrying")
			}
			return nil, c.writeError(err)
		}
		affected, _ := value.RowsAffected()
		return &Result{Columns: []Column{}, Rows: []Row{}, Affected: strconv.FormatInt(affected, 10), DurationMS: time.Since(start).Milliseconds()}, nil
	}
	if c.Engine == "sqlite" {
		if _, err = db.ExecContext(ctx, "PRAGMA query_only=ON"); err != nil {
			return nil, c.redact(err)
		}
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, c.redact(err)
	}
	defer tx.Rollback()
	out, err := readRows(ctx, tx, r.SQL, nil, limit(r.Limit))
	if err != nil {
		return nil, c.redact(err)
	}
	out.DurationMS = time.Since(start).Milliseconds()
	return out, nil
}
func (c Connection) Ping(ctx context.Context) error {
	if c.Engine == "mongodb" {
		client, err := c.mongoOpen(ctx)
		if err == nil {
			defer client.Disconnect(context.Background())
		}
		return err
	}
	db, err := c.sqlOpen(ctx, "")
	if err == nil {
		db.Close()
	}
	return err
}
