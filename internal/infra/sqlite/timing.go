package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"hash/fnv"
	"log/slog"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	msqlite "modernc.org/sqlite"

	"github.com/myco-ai/myco/internal/perf"
	"github.com/myco-ai/myco/internal/requestid"
)

// DefaultSlowStatement is when a statement is logged as slow.
const DefaultSlowStatement = 50 * time.Millisecond

// Timing says how a database's statements are timed. Each statement is
// named after the repository method that ran it, the statement's verb and
// table, and a short hash of its text ("MessageRepo.Recent: SELECT messages
// #1a2b"): never its parameters or what it read.
type Timing struct {
	// Log gets a warning ("slow statement") for a statement taking Slow or
	// longer: its name, duration and rows.
	Log *slog.Logger
	// Slow: 0 is DefaultSlowStatement, negative never.
	Slow time.Duration
	// Perf, when set, collects every statement's duration.
	Perf *perf.Recorder

	// live: opened and migrated (the migrations are not timed).
	live *atomic.Bool
}

// OpenTimed opens (and migrates) a SQLite database whose statements are
// timed (Open does not time them).
func OpenTimed(path string, t Timing) (*sql.DB, error) {
	if t.Slow == 0 {
		t.Slow = DefaultSlowStatement
	}
	t.live = new(atomic.Bool)
	return open(path, &t)
}

func openDB(dsn string, t *Timing) *sql.DB {
	if t == nil {
		db, _ := sql.Open("sqlite", dsn) // (registered: never fails)
		return db
	}
	return sql.OpenDB(timedConnector{dsn: dsn, t: t})
}

var baseDriver = &msqlite.Driver{}

type timedConnector struct {
	dsn string
	t   *Timing
}

func (c timedConnector) Connect(context.Context) (driver.Conn, error) {
	inner, err := baseDriver.Open(c.dsn)
	if err != nil {
		return nil, err
	}
	return &timedConn{Conn: inner, t: c.t}, nil
}

func (c timedConnector) Driver() driver.Driver { return baseDriver }

// done records a statement; id is the request it ran for (X-Request-Id,
// from its context), so a slow statement is found with its slow request.
func (t *Timing) done(name string, d time.Duration, rows int64, id string) {
	if !t.live.Load() {
		return
	}
	t.Perf.Add(name, d, rows)
	if t.Slow > 0 && d >= t.Slow && t.Log != nil {
		args := []any{"name", name, "duration", d.Round(100 * time.Microsecond), "rows", rows}
		if id != "" {
			args = append(args, "id", id)
		}
		t.Log.Warn("slow statement", args...)
	}
}

// timedConn passes everything to the driver's connection, timing what runs
// a statement. (modernc's connection implements each interface asserted.)
type timedConn struct {
	driver.Conn
	t *Timing
}

func (c *timedConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	s, err := c.Conn.(driver.ConnPrepareContext).PrepareContext(ctx, query)
	if err != nil {
		return nil, err
	}
	return &timedStmt{Stmt: s, t: c.t, query: query}, nil
}

func (c *timedConn) Prepare(query string) (driver.Stmt, error) {
	return c.PrepareContext(context.Background(), query)
}

func (c *timedConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	tx, err := c.Conn.(driver.ConnBeginTx).BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &timedTx{Tx: tx, t: c.t, name: statementName("COMMIT"), id: requestid.From(ctx)}, nil
}

// clock times statements (tests replace it: Windows' clock can be too
// coarse to see a fast statement take any time).
var clock = time.Now

// statementHook, set by tests, sees each statement a timed connection runs.
var statementHook func(query string, args []driver.NamedValue)

func (c *timedConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if statementHook != nil {
		statementHook(query, args)
	}
	start := clock()
	res, err := c.Conn.(driver.ExecerContext).ExecContext(ctx, query, args)
	c.t.exec(ctx, query, clock().Sub(start), res, err)
	return res, err
}

func (c *timedConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if statementHook != nil {
		statementHook(query, args)
	}
	start := clock()
	rows, err := c.Conn.(driver.QueryerContext).QueryContext(ctx, query, args)
	return c.t.rows(ctx, query, clock().Sub(start), rows, err)
}

func (c *timedConn) Ping(ctx context.Context) error { return c.Conn.(driver.Pinger).Ping(ctx) }
func (c *timedConn) ResetSession(ctx context.Context) error {
	return c.Conn.(driver.SessionResetter).ResetSession(ctx)
}
func (c *timedConn) IsValid() bool { return c.Conn.(driver.Validator).IsValid() }

func (t *Timing) exec(ctx context.Context, query string, d time.Duration, res driver.Result, err error) {
	var n int64
	if err == nil && res != nil {
		n, _ = res.RowsAffected()
	}
	t.done(statementName(query), d, n, requestid.From(ctx))
}

func (t *Timing) rows(ctx context.Context, query string, d time.Duration, rows driver.Rows, err error) (driver.Rows, error) {
	name := statementName(query)
	id := requestid.From(ctx)
	if err != nil {
		t.done(name, d, 0, id)
		return nil, err
	}
	return &timedRows{Rows: rows, t: t, name: name, d: d, id: id}, nil
}

type timedStmt struct {
	driver.Stmt
	t     *Timing
	query string
}

func (s *timedStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	start := clock()
	res, err := s.Stmt.(driver.StmtExecContext).ExecContext(ctx, args)
	s.t.exec(ctx, s.query, clock().Sub(start), res, err)
	return res, err
}

func (s *timedStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	start := clock()
	rows, err := s.Stmt.(driver.StmtQueryContext).QueryContext(ctx, args)
	return s.t.rows(ctx, s.query, clock().Sub(start), rows, err)
}

type timedTx struct {
	driver.Tx
	t    *Timing
	name string
	id   string
}

func (x *timedTx) Commit() error {
	start := clock()
	err := x.Tx.Commit()
	x.t.done(x.name, clock().Sub(start), 0, x.id)
	return err
}

// timedRows counts the time spent reading rows (not what the caller does
// between them) and records the statement when closed.
type timedRows struct {
	driver.Rows
	t      *Timing
	name   string
	d      time.Duration
	n      int64
	id     string
	closed bool
}

func (r *timedRows) Next(dest []driver.Value) error {
	start := clock()
	err := r.Rows.Next(dest)
	r.d += clock().Sub(start)
	if err == nil {
		r.n++
	}
	return err
}

func (r *timedRows) Close() error {
	err := r.Rows.Close()
	if !r.closed {
		r.closed = true
		r.t.done(r.name, r.d, r.n, r.id)
	}
	return err
}

func (r *timedRows) ColumnTypeDatabaseTypeName(i int) string {
	return r.Rows.(driver.RowsColumnTypeDatabaseTypeName).ColumnTypeDatabaseTypeName(i)
}
func (r *timedRows) ColumnTypeLength(i int) (int64, bool) {
	return r.Rows.(driver.RowsColumnTypeLength).ColumnTypeLength(i)
}
func (r *timedRows) ColumnTypeNullable(i int) (bool, bool) {
	return r.Rows.(driver.RowsColumnTypeNullable).ColumnTypeNullable(i)
}
func (r *timedRows) ColumnTypePrecisionScale(i int) (int64, int64, bool) {
	return r.Rows.(driver.RowsColumnTypePrecisionScale).ColumnTypePrecisionScale(i)
}
func (r *timedRows) ColumnTypeScanType(i int) reflect.Type {
	return r.Rows.(driver.RowsColumnTypeScanType).ColumnTypeScanType(i)
}

var (
	pkgPrefix  = reflect.TypeFor[Timing]().PkgPath() + "."
	tableRe    = regexp.MustCompile(`(?is)\b(?:FROM|INTO|UPDATE)\s+([A-Za-z_][A-Za-z0-9_]*)`)
	argListRe  = regexp.MustCompile(`\?(\s*,\s*\?)+`)
	spaceRe    = regexp.MustCompile(`\s+`)
	funcSuffix = regexp.MustCompile(`(\.func\d+)+$|\[[^\]]*\]`)
	summaries  sync.Map // query -> "SELECT messages #1a2b"
	summaryN   int64
	summaryMu  sync.Mutex
)

// statementName: the outermost function of this package on the stack (the
// repository method the service called), then what the statement is.
func statementName(query string) string {
	pcs := make([]uintptr, 48)
	n := runtime.Callers(2, pcs)
	frames := runtime.CallersFrames(pcs[:n])
	caller := ""
	for {
		f, more := frames.Next()
		// (a test in this package calling a repository is not the caller)
		if fn := strings.TrimPrefix(f.Function, pkgPrefix); fn != f.Function && !strings.HasPrefix(fn, "Test") {
			if !strings.HasSuffix(f.File, "timing.go") {
				caller = f.Function
			}
		} else if caller != "" {
			break
		}
		if !more {
			break
		}
	}
	caller = strings.TrimPrefix(caller, pkgPrefix)
	caller = funcSuffix.ReplaceAllString(caller, "")
	caller = strings.NewReplacer("(*", "", ")", "").Replace(caller)
	if caller == "" {
		caller = "sqlite"
	}
	return caller + ": " + summary(query)
}

// summary: the statement's verb, its first table and a hash of its text
// (lists of placeholders folded, so "IN (?, ?)" and "IN (?, ?, ?)" match).
func summary(query string) string {
	if s, ok := summaries.Load(query); ok {
		return s.(string)
	}
	norm := strings.TrimSpace(spaceRe.ReplaceAllString(argListRe.ReplaceAllString(query, "?"), " "))
	verb, _, _ := strings.Cut(norm, " ")
	verb = strings.ToUpper(verb)
	s := verb
	if m := tableRe.FindStringSubmatch(norm); m != nil {
		s += " " + m[1]
	}
	if verb != "COMMIT" {
		h := fnv.New32a()
		h.Write([]byte(norm))
		s += fmt.Sprintf(" #%04x", h.Sum32()&0xffff)
	}
	summaryMu.Lock()
	if summaryN < 5000 { // (statements built at run time stay bounded)
		summaries.Store(query, s)
		summaryN++
	}
	summaryMu.Unlock()
	return s
}
