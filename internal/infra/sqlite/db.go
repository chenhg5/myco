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
CREATE TABLE IF NOT EXISTS agent_resumes (
  agent         TEXT PRIMARY KEY,
  token_hash    TEXT NOT NULL UNIQUE,
  created_at    INTEGER NOT NULL,
  last_used_at  INTEGER NOT NULL DEFAULT 0,
  last_used_env TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS messages (
  id              TEXT PRIMARY KEY,
  from_agent      TEXT NOT NULL,
  to_agent        TEXT NOT NULL,
  thread_id       TEXT NOT NULL,
  reply_to        TEXT NOT NULL DEFAULT '',
  after           TEXT NOT NULL DEFAULT '',
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
CREATE TABLE IF NOT EXISTS aliases (
  name       TEXT PRIMARY KEY,
  target     TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_aliases_target ON aliases(target);
CREATE TABLE IF NOT EXISTS machines (
  id               TEXT PRIMARY KEY,
  fingerprint_hash TEXT NOT NULL UNIQUE,
  name             TEXT NOT NULL DEFAULT '',
  hostname         TEXT NOT NULL DEFAULT '',
  os               TEXT NOT NULL DEFAULT '',
  arch             TEXT NOT NULL DEFAULT '',
  myco_version     TEXT NOT NULL DEFAULT '',
  created_at       INTEGER NOT NULL,
  last_seen_at     INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS console_tokens (
  id           TEXT PRIMARY KEY,
  name         TEXT NOT NULL,
  token_hash   TEXT NOT NULL UNIQUE,
  created_at   INTEGER NOT NULL,
  last_used_at INTEGER NOT NULL DEFAULT 0,
  revoked_at   INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS console_pairings (
  id         TEXT PRIMARY KEY,
  code_hash  TEXT NOT NULL UNIQUE,
  member_id  TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL,
  used_at    INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS members (
  id           TEXT PRIMARY KEY,
  name         TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL DEFAULT '',
  role         TEXT NOT NULL,
  created_at   INTEGER NOT NULL,
  removed_at   INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS audit_log (
  seq        INTEGER PRIMARY KEY AUTOINCREMENT,
  id         TEXT NOT NULL UNIQUE,
  at         INTEGER NOT NULL,
  actor      TEXT NOT NULL DEFAULT '',
  actor_name TEXT NOT NULL DEFAULT '',
  actor_kind TEXT NOT NULL DEFAULT '',
  action     TEXT NOT NULL,
  target     TEXT NOT NULL DEFAULT '',
  detail     TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_audit_at ON audit_log(at);
CREATE TABLE IF NOT EXISTS e2e_keys (
  kind       TEXT NOT NULL,
  id         TEXT NOT NULL,
  enc_key    TEXT NOT NULL,
  sign_key   TEXT NOT NULL,
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (kind, id)
);
CREATE TABLE IF NOT EXISTS hub_settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS projects (
  id         TEXT PRIMARY KEY,
  name       TEXT NOT NULL UNIQUE,
  title      TEXT NOT NULL DEFAULT '',
  goal       TEXT NOT NULL DEFAULT '',
  brief      TEXT NOT NULL DEFAULT '',
  links      TEXT NOT NULL DEFAULT '[]',
  visibility TEXT NOT NULL DEFAULT 'members',
  owner      TEXT NOT NULL DEFAULT '',
  archived   INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS project_members (
  project  TEXT NOT NULL,
  agent    TEXT NOT NULL,
  role     TEXT NOT NULL DEFAULT 'member',
  note     TEXT NOT NULL DEFAULT '',
  added_by TEXT NOT NULL DEFAULT '',
  added_at INTEGER NOT NULL,
  PRIMARY KEY (project, agent)
);
CREATE INDEX IF NOT EXISTS idx_project_members_agent ON project_members(agent);
CREATE TABLE IF NOT EXISTS message_traces (
  message TEXT NOT NULL,
  seq     INTEGER NOT NULL,
  kind    TEXT NOT NULL,
  data    TEXT NOT NULL,
  at      INTEGER NOT NULL,
  PRIMARY KEY (message, seq)
);
CREATE INDEX IF NOT EXISTS idx_message_traces_at ON message_traces(at);
CREATE TABLE IF NOT EXISTS project_docs (
  project    TEXT NOT NULL,
  name       TEXT NOT NULL,
  body       TEXT NOT NULL DEFAULT '',
  content_type TEXT NOT NULL DEFAULT '',
  size       INTEGER NOT NULL DEFAULT 0,
  blob       TEXT NOT NULL DEFAULT '',
  updated_by TEXT NOT NULL DEFAULT '',
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (project, name)
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
	if err := addColumns(db, "enrollments", [][2]string{{"labels", "TEXT NOT NULL DEFAULT '[]'"},
		{"kind", "TEXT NOT NULL DEFAULT ''"}, {"parent_id", "TEXT NOT NULL DEFAULT ''"}, {"machine", "TEXT NOT NULL DEFAULT ''"},
		{"owner", "TEXT NOT NULL DEFAULT ''"}}); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	if err := addColumns(db, "messages", [][2]string{{"broadcast_id", "TEXT NOT NULL DEFAULT ''"}, {"attachments", "TEXT NOT NULL DEFAULT '[]'"},
		{"from_owner", "TEXT NOT NULL DEFAULT ''"}, {"project", "TEXT NOT NULL DEFAULT ''"}, {"after", "TEXT NOT NULL DEFAULT ''"},
		// Questions to a task's sender: the question, the task waiting on it,
		// the steer answering it.
		{"ask", "INTEGER NOT NULL DEFAULT 0"}, {"asking", "TEXT NOT NULL DEFAULT ''"}, {"answers", "TEXT NOT NULL DEFAULT ''"}}); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	for table, cols := range map[string][][2]string{
		"machines":       {{"owner", "TEXT NOT NULL DEFAULT ''"}},
		"console_tokens": {{"member_id", "TEXT NOT NULL DEFAULT ''"}},
		"project_docs":   {{"content_type", "TEXT NOT NULL DEFAULT ''"}, {"size", "INTEGER NOT NULL DEFAULT 0"}, {"blob", "TEXT NOT NULL DEFAULT ''"}},
	} {
		if err := addColumns(db, table, cols); err != nil {
			db.Close()
			return nil, fmt.Errorf("migrate: %w", err)
		}
	}
	for _, ddl := range []string{
		`CREATE INDEX IF NOT EXISTS idx_agents_session ON agents(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_broadcast ON messages(broadcast_id) WHERE broadcast_id != ''`,
		`CREATE INDEX IF NOT EXISTS idx_messages_project ON messages(project, thread_id) WHERE project != ''`,
		`CREATE INDEX IF NOT EXISTS idx_messages_after ON messages(after) WHERE after != ''`,
		// Messages an agent sent (its inbox is idx_messages_inbox): peers and
		// exchanges between two nodes.
		`CREATE INDEX IF NOT EXISTS idx_messages_from ON messages(from_agent, to_agent, created_at)`,
		// Open messages only (see openStatus in messages.go): inboxes, due
		// scans and counters read this instead of the whole history.
		`CREATE INDEX IF NOT EXISTS idx_messages_open ON messages(to_agent, status, created_at) WHERE status IN ('queued','delivered','acked','held')`,
		// The newest messages (the overview's recent list).
		`CREATE INDEX IF NOT EXISTS idx_messages_updated ON messages(updated_at DESC, id)`,
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
	{"display_name", "TEXT NOT NULL DEFAULT ''"},
	{"status_text", "TEXT NOT NULL DEFAULT ''"},
	{"status_at", "INTEGER NOT NULL DEFAULT 0"},
	// Skills (JSON); empty for agents stored before them: their
	// capabilities stand in as bare skills.
	{"skills", "TEXT NOT NULL DEFAULT '[]'"},
	// team / private; "" (agents from before) is team.
	{"visibility", "TEXT NOT NULL DEFAULT ''"},
	// v0.18.24 only (coordinators are now a project role); unused.
	{"coordinator", "INTEGER NOT NULL DEFAULT 0"},
	// Reported by runners (v0.16.9): the machine the agent runs on, and
	// its CLI and model.
	{"machine_id", "TEXT NOT NULL DEFAULT ''"},
	{"runtime", "TEXT NOT NULL DEFAULT '{}'"},
	// Members (v0.18): the person who added the agent.
	{"owner", "TEXT NOT NULL DEFAULT ''"},
	// Faces people chose: the avatar word, and the picture
	// when it is one.
	{"avatar", "TEXT NOT NULL DEFAULT ''"},
	{"avatar_type", "TEXT NOT NULL DEFAULT ''"},
	{"avatar_data", "BLOB"},
	// The model and reasoning effort chosen for the agent's runner (v0.19.5).
	{"run_settings", "TEXT NOT NULL DEFAULT '{}'"},
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
