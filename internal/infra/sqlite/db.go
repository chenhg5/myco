// Package sqlite implements myco's repositories on SQLite (pure Go driver,
// no cgo, so myco stays a single static binary).
package sqlite

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/chenhg5/myco/internal/domain"
)

const schema = `
CREATE TABLE IF NOT EXISTS agents (
  id           TEXT PRIMARY KEY,
  name         TEXT NOT NULL UNIQUE,
  description  TEXT NOT NULL DEFAULT '',
  capabilities TEXT NOT NULL DEFAULT '[]',
  delivery     TEXT NOT NULL DEFAULT 'pull',
  allow_from   TEXT NOT NULL DEFAULT '[]',
  token_hash   TEXT NOT NULL UNIQUE,
  created_at   INTEGER NOT NULL,
  last_seen_at INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS messages (
  id              TEXT PRIMARY KEY,
  from_agent      TEXT NOT NULL,
  to_agent        TEXT NOT NULL,
  thread_id       TEXT NOT NULL,
  reply_to        TEXT NOT NULL DEFAULT '',
  type            TEXT NOT NULL,
  body            TEXT NOT NULL,
  status          TEXT NOT NULL,
  attempts        INTEGER NOT NULL DEFAULT 0,
  lease_until     INTEGER NOT NULL DEFAULT 0,
  error           TEXT NOT NULL DEFAULT '',
  idempotency_key TEXT,
  created_at      INTEGER NOT NULL,
  updated_at      INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_messages_inbox  ON messages(to_agent, status, created_at);
CREATE INDEX IF NOT EXISTS idx_messages_thread ON messages(thread_id, created_at);
CREATE INDEX IF NOT EXISTS idx_messages_reply  ON messages(reply_to);
CREATE TABLE IF NOT EXISTS enrollments (
  id            TEXT PRIMARY KEY,
  token_hash    TEXT NOT NULL UNIQUE,
  name_prefix   TEXT NOT NULL,
  harness       TEXT NOT NULL DEFAULT '',
  allow_from    TEXT NOT NULL DEFAULT '[]',
  capabilities  TEXT NOT NULL DEFAULT '[]',
  note          TEXT NOT NULL DEFAULT '',
  max_uses      INTEGER NOT NULL DEFAULT 0,
  uses          INTEGER NOT NULL DEFAULT 0,
  expires_at    INTEGER NOT NULL DEFAULT 0,
  revoked_at    INTEGER NOT NULL DEFAULT 0,
  created_at    INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS files (
  id           TEXT PRIMARY KEY,
  name         TEXT NOT NULL,
  size         INTEGER NOT NULL,
  sha256       TEXT NOT NULL,
  content_type TEXT NOT NULL DEFAULT '',
  owner        TEXT NOT NULL,
  created_at   INTEGER NOT NULL,
  expires_at   INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_files_expires ON files(expires_at);
CREATE TABLE IF NOT EXISTS file_access (
  file_id TEXT NOT NULL,
  agent   TEXT NOT NULL,
  PRIMARY KEY (file_id, agent)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_messages_idem ON messages(from_agent, idempotency_key) WHERE idempotency_key IS NOT NULL;
`

// Open opens (and migrates) a SQLite database. Use ":memory:" for tests.
func Open(path string) (*sql.DB, error) {
	dsn := path
	if path != ":memory:" {
		dsn = "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// A single writer connection serializes transactions and keeps
	// ":memory:" databases shared across the pool.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	if err := addColumns(db, "agents", agentColumnsV2); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	if err := addColumns(db, "enrollments", [][2]string{{"labels", "TEXT NOT NULL DEFAULT '[]'"}}); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	if err := addColumns(db, "messages", [][2]string{{"broadcast_id", "TEXT NOT NULL DEFAULT ''"}, {"attachments", "TEXT NOT NULL DEFAULT '[]'"}}); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	for _, ddl := range []string{
		`CREATE INDEX IF NOT EXISTS idx_agents_session ON agents(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_broadcast ON messages(broadcast_id) WHERE broadcast_id != ''`,
	} {
		if _, err := db.Exec(ddl); err != nil {
			db.Close()
			return nil, fmt.Errorf("migrate: %w", err)
		}
	}
	return db, nil
}

// agentColumnsV2 were added in phase 2 (session binding + push delivery).
var agentColumnsV2 = [][2]string{
	{"harness", "TEXT NOT NULL DEFAULT ''"},
	{"session_id", "TEXT NOT NULL DEFAULT ''"},
	{"environment", "TEXT NOT NULL DEFAULT ''"},
	{"workdir", "TEXT NOT NULL DEFAULT ''"},
	{"delivery_url", "TEXT NOT NULL DEFAULT ''"},
	{"delivery_secret", "TEXT NOT NULL DEFAULT ''"},
	{"delivery_params", "TEXT NOT NULL DEFAULT '{}'"},
	{"enrolled_by", "TEXT NOT NULL DEFAULT ''"},
	{"labels", "TEXT NOT NULL DEFAULT '[]'"},
}

// addColumns adds missing columns (idempotent, forward-only migrations).
func addColumns(db *sql.DB, table string, cols [][2]string) error {
	rows, err := db.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return err
		}
		have[n] = true
	}
	rows.Close()
	for _, c := range cols {
		if !have[c[0]] {
			if _, err := db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + c[0] + ` ` + c[1]); err != nil {
				return err
			}
		}
	}
	return nil
}

func ms(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

func fromMS(v int64) time.Time {
	if v == 0 {
		return time.Time{}
	}
	return time.UnixMilli(v).UTC()
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

func internal(err error) error {
	if err == nil {
		return nil
	}
	return domain.Errf(domain.CodeInternal, "storage: %v", err)
}
