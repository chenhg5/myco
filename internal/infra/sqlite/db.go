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
	return db, nil
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
