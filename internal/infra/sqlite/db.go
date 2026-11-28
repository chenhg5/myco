// Package sqlite implements myco's repositories on SQLite (pure Go driver,
// no cgo, so myco stays a single static binary).
package sqlite

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/myco-ai/myco/internal/domain"
)

const schema = `
-- Account quota snapshots are scoped to the agent owner and authenticated machine.
CREATE TABLE IF NOT EXISTS account_quotas (
 owner TEXT NOT NULL,
 machine TEXT NOT NULL,
 provider TEXT NOT NULL,
 profile TEXT NOT NULL,
 snapshot TEXT NOT NULL,
 PRIMARY KEY(owner,machine,provider,profile)
);

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
-- Handovers (v0.19.23): the session behind a node replaced by another.
CREATE TABLE IF NOT EXISTS agent_handovers (
  id              TEXT PRIMARY KEY,
  agent           TEXT NOT NULL,
  at              INTEGER NOT NULL,
  by_who          TEXT NOT NULL DEFAULT '',
  note            TEXT NOT NULL DEFAULT '',
  from_binding    TEXT NOT NULL DEFAULT '{}',
  from_machine_id TEXT NOT NULL DEFAULT '',
  from_runtime    TEXT NOT NULL DEFAULT '{}',
  to_binding      TEXT NOT NULL DEFAULT '{}',
  to_machine_id   TEXT NOT NULL DEFAULT '',
  to_runtime      TEXT NOT NULL DEFAULT '{}',
  state           TEXT NOT NULL,
  done_at         INTEGER NOT NULL DEFAULT 0,
  open_tasks      TEXT NOT NULL DEFAULT '[]',
  back_to         TEXT NOT NULL DEFAULT '',
  expires_at      INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_agent_handovers ON agent_handovers(agent, at DESC);
CREATE TABLE IF NOT EXISTS handover_links (
  token_hash  TEXT PRIMARY KEY,
  handover_id TEXT NOT NULL,
  agent       TEXT NOT NULL,
  expires_at  INTEGER NOT NULL,
  used_at     INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS agent_retired_tokens (
  token_hash  TEXT PRIMARY KEY,
  agent       TEXT NOT NULL,
  handover_id TEXT NOT NULL,
  retired_at  INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_agent_retired_tokens ON agent_retired_tokens(agent, retired_at DESC);
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
CREATE TABLE IF NOT EXISTS automations (
  id           TEXT PRIMARY KEY,
  name         TEXT NOT NULL,
  owner        TEXT NOT NULL DEFAULT '',
  owner_id     TEXT NOT NULL DEFAULT '',
  from_agent   TEXT NOT NULL,
  to_agent     TEXT NOT NULL,
  project      TEXT NOT NULL DEFAULT '',
  body         TEXT NOT NULL,
  cron         TEXT NOT NULL DEFAULT '',
  tz           TEXT NOT NULL DEFAULT '',
  webhook_hash TEXT NOT NULL DEFAULT '',
  skip_if_busy INTEGER NOT NULL DEFAULT 0,
  enabled      INTEGER NOT NULL DEFAULT 1,
  created_at   INTEGER NOT NULL,
  updated_at   INTEGER NOT NULL,
  next_run_at  INTEGER NOT NULL DEFAULT 0,
  last_task    TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_automations_due ON automations(next_run_at) WHERE enabled = 1 AND next_run_at > 0;
CREATE TABLE IF NOT EXISTS automation_runs (
  id            TEXT PRIMARY KEY,
  automation_id TEXT NOT NULL,
  at            INTEGER NOT NULL,
  run_trigger   TEXT NOT NULL,
  outcome       TEXT NOT NULL,
  message_id    TEXT NOT NULL DEFAULT '',
  error         TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_automation_runs ON automation_runs(automation_id, at DESC);
CREATE TABLE IF NOT EXISTS workflows (
  id         TEXT PRIMARY KEY,
  name       TEXT NOT NULL UNIQUE,
  project    TEXT NOT NULL DEFAULT '',
  owner      TEXT NOT NULL DEFAULT '',
  owner_id   TEXT NOT NULL DEFAULT '',
  version    INTEGER NOT NULL,
  def        TEXT NOT NULL,
  enabled    INTEGER NOT NULL DEFAULT 1,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS workflow_versions (
  workflow_id TEXT NOT NULL,
  version     INTEGER NOT NULL,
  def         TEXT NOT NULL,
  by_name     TEXT NOT NULL DEFAULT '',
  created_at  INTEGER NOT NULL,
  PRIMARY KEY (workflow_id, version)
);
CREATE TABLE IF NOT EXISTS workflow_runs (
  id          TEXT PRIMARY KEY,
  workflow_id TEXT NOT NULL,
  name        TEXT NOT NULL,
  version     INTEGER NOT NULL,
  project     TEXT NOT NULL DEFAULT '',
  status      TEXT NOT NULL,
  inputs      TEXT NOT NULL DEFAULT '{}',
  owner       TEXT NOT NULL DEFAULT '',
  owner_id    TEXT NOT NULL DEFAULT '',
  by_name     TEXT NOT NULL DEFAULT '',
  run_trigger TEXT NOT NULL DEFAULT '',
  error       TEXT NOT NULL DEFAULT '',
  state       TEXT NOT NULL DEFAULT '{}',
  created_at  INTEGER NOT NULL,
  updated_at  INTEGER NOT NULL,
  ended_at    INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_workflow_runs ON workflow_runs(workflow_id, created_at DESC);
-- Cover the frequently refreshed workflow catalog counts without run payload reads.
CREATE INDEX IF NOT EXISTS idx_workflow_runs_counts ON workflow_runs(workflow_id, status);
CREATE INDEX IF NOT EXISTS idx_workflow_runs_active ON workflow_runs(status) WHERE status IN ('running', 'waiting');
CREATE TABLE IF NOT EXISTS run_steps (
  id         TEXT PRIMARY KEY,
  run_id     TEXT NOT NULL,
  step_id    TEXT NOT NULL,
  kind       TEXT NOT NULL,
  attempt    INTEGER NOT NULL,
  status     TEXT NOT NULL,
  target     TEXT NOT NULL DEFAULT '',
  message_id TEXT NOT NULL DEFAULT '',
  input      TEXT NOT NULL DEFAULT '',
  output     TEXT NOT NULL DEFAULT '',
  summary    TEXT NOT NULL DEFAULT '',
  error      TEXT NOT NULL DEFAULT '',
  retries    INTEGER NOT NULL DEFAULT 0,
  bounced    INTEGER NOT NULL DEFAULT 0,
  started_at INTEGER NOT NULL,
  ended_at   INTEGER NOT NULL DEFAULT 0,
  due_at     INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_run_steps ON run_steps(run_id, started_at);
CREATE INDEX IF NOT EXISTS idx_run_steps_message ON run_steps(message_id) WHERE message_id != '';
CREATE TABLE IF NOT EXISTS run_events (
  id     INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id TEXT NOT NULL,
  at     INTEGER NOT NULL,
  kind   TEXT NOT NULL,
  step   TEXT NOT NULL DEFAULT '',
  text   TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_run_events ON run_events(run_id, id);
CREATE TABLE IF NOT EXISTS superagents (
  member_id TEXT PRIMARY KEY,
  agent     TEXT NOT NULL,
  set_at    INTEGER NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_superagents_agent ON superagents(agent);
CREATE TABLE IF NOT EXISTS approvals (
  id           TEXT PRIMARY KEY,
  owner        TEXT NOT NULL,
  agent        TEXT NOT NULL,
  what         TEXT NOT NULL DEFAULT '',
  reason       TEXT NOT NULL DEFAULT '',
  method       TEXT NOT NULL,
  path         TEXT NOT NULL,
  body         BLOB,
  content_type TEXT NOT NULL DEFAULT '',
  status       TEXT NOT NULL,
  result       INTEGER NOT NULL DEFAULT 0,
  result_body  TEXT NOT NULL DEFAULT '',
  decided_by   TEXT NOT NULL DEFAULT '',
  created_at   INTEGER NOT NULL,
  decided_at   INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_approvals_owner ON approvals(owner, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_approvals_agent ON approvals(agent, created_at DESC);
CREATE TABLE IF NOT EXISTS run_evaluations (
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id    TEXT NOT NULL,
  step      TEXT NOT NULL DEFAULT '',
  name      TEXT NOT NULL,
  value     REAL NOT NULL,
  unit      TEXT NOT NULL DEFAULT '',
  direction TEXT NOT NULL DEFAULT '',
  note      TEXT NOT NULL DEFAULT '',
  by_name   TEXT NOT NULL DEFAULT '',
  at        INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_run_evaluations ON run_evaluations(run_id, id);
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
CREATE TABLE IF NOT EXISTS pages (
  id         TEXT PRIMARY KEY,
  name       TEXT NOT NULL,
  project    TEXT NOT NULL DEFAULT '',
  publisher  TEXT NOT NULL DEFAULT '',
  owner      TEXT NOT NULL DEFAULT '',
  latest     INTEGER NOT NULL DEFAULT 0,
  live       INTEGER NOT NULL DEFAULT 0,
  port       INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS page_versions (
  page    TEXT NOT NULL,
  n       INTEGER NOT NULL,
  blob    TEXT NOT NULL,
  sha256  TEXT NOT NULL DEFAULT '',
  size    INTEGER NOT NULL DEFAULT 0,
  entry   TEXT NOT NULL DEFAULT 'index.html',
  files   INTEGER NOT NULL DEFAULT 0,
  by_name TEXT NOT NULL DEFAULT '',
  at      INTEGER NOT NULL,
  PRIMARY KEY (page, n)
);
CREATE TABLE IF NOT EXISTS page_state (
  page       TEXT PRIMARY KEY,
  state      TEXT NOT NULL DEFAULT '',
  version    INTEGER NOT NULL DEFAULT 0,
  updated_by TEXT NOT NULL DEFAULT '',
  updated_at INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS page_records (
  page       TEXT NOT NULL,
  collection TEXT NOT NULL,
  id         TEXT NOT NULL,
  data       TEXT NOT NULL,
  version    INTEGER NOT NULL,
  updated_by TEXT NOT NULL DEFAULT '',
  updated_at INTEGER NOT NULL,
  pos        INTEGER NOT NULL,
  PRIMARY KEY (page, collection, id)
);
-- Forward-only, idempotent indexes for record order and common task queries.
CREATE INDEX IF NOT EXISTS idx_page_records_order ON page_records(page, collection, pos, id);
CREATE INDEX IF NOT EXISTS idx_page_records_n ON page_records(page, collection, (json_type(data,'$.n') IS NULL), json_extract(data,'$.n') DESC, pos, id);
CREATE INDEX IF NOT EXISTS idx_page_records_status ON page_records(page, collection, json_extract(data,'$.status'));
CREATE INDEX IF NOT EXISTS idx_page_records_assignee_status ON page_records(page, collection, json_extract(data,'$.assignee'), json_extract(data,'$.status'));
CREATE TABLE IF NOT EXISTS page_changes (
  seq        INTEGER PRIMARY KEY AUTOINCREMENT,
  page       TEXT NOT NULL,
  collection TEXT NOT NULL,
  record     TEXT NOT NULL,
  op         TEXT NOT NULL,
  data       TEXT NOT NULL DEFAULT '',
  version    INTEGER NOT NULL DEFAULT 0,
  by_name    TEXT NOT NULL DEFAULT '',
  at         INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_page_changes ON page_changes(page, seq);
CREATE TABLE IF NOT EXISTS templates (
  id          TEXT PRIMARY KEY,
  slug        TEXT NOT NULL UNIQUE,
  name        TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  kind        TEXT NOT NULL DEFAULT 'custom',
  latest      TEXT NOT NULL DEFAULT '',
  publisher   TEXT NOT NULL DEFAULT '',
  owner       TEXT NOT NULL DEFAULT '',
  created_at  INTEGER NOT NULL,
  updated_at  INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS template_versions (
  template TEXT NOT NULL,
  version  TEXT NOT NULL,
  blob     TEXT NOT NULL,
  sha256   TEXT NOT NULL DEFAULT '',
  size     INTEGER NOT NULL DEFAULT 0,
  files    INTEGER NOT NULL DEFAULT 0,
  manifest TEXT NOT NULL,
  guide    TEXT NOT NULL DEFAULT '',
  by_name  TEXT NOT NULL DEFAULT '',
  at       INTEGER NOT NULL,
  PRIMARY KEY (template, version)
);
CREATE TABLE IF NOT EXISTS notices (
  seq         INTEGER PRIMARY KEY AUTOINCREMENT,
  id          TEXT NOT NULL UNIQUE,
  recipient   TEXT NOT NULL,
  key         TEXT NOT NULL,
  kind        TEXT NOT NULL,
  tier        TEXT NOT NULL,
  ref         TEXT NOT NULL DEFAULT '',
  target      TEXT NOT NULL DEFAULT '',
  project     TEXT NOT NULL DEFAULT '',
  actor       TEXT NOT NULL DEFAULT '',
  subject     TEXT NOT NULL DEFAULT '',
  message_id  TEXT NOT NULL DEFAULT '',
  announce    INTEGER NOT NULL DEFAULT 0,
  quiet       TEXT NOT NULL DEFAULT '',
  created_at  INTEGER NOT NULL,
  read_at     INTEGER NOT NULL DEFAULT 0,
  resolved_at INTEGER NOT NULL DEFAULT 0,
  UNIQUE (recipient, key)
);
CREATE INDEX IF NOT EXISTS idx_notices_recipient ON notices(recipient, seq);
CREATE INDEX IF NOT EXISTS idx_notices_open_ref ON notices(ref) WHERE resolved_at = 0;
CREATE UNIQUE INDEX IF NOT EXISTS idx_messages_idem ON messages(from_agent, idempotency_key) WHERE idempotency_key IS NOT NULL;
`

// Open opens (and migrates) a SQLite database. Use ":memory:" for tests.
func Open(path string) (*sql.DB, error) { return open(path, nil) }

func open(path string, t *Timing) (*sql.DB, error) {
	dsn := path
	if path != ":memory:" {
		dsn = "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	}
	db := openDB(dsn, t)
	// A single writer connection serializes transactions and keeps
	// ":memory:" databases shared across the pool.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema + agentForkSchema + usageSchema + projectInvitationSchema + pageAttachmentSchema + watchSchema + goalSchema + modelProviderSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	if err := addColumns(db, "agents", agentColumnsV2); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	if err := addColumns(db, "enrollments", [][2]string{{"labels", "TEXT NOT NULL DEFAULT '[]'"},
		{"kind", "TEXT NOT NULL DEFAULT ''"}, {"parent_id", "TEXT NOT NULL DEFAULT ''"}, {"machine", "TEXT NOT NULL DEFAULT ''"},
		{"owner", "TEXT NOT NULL DEFAULT ''"},
		// A link whose agent becomes its owner's superagent (v0.21.5).
		{"superagent", "INTEGER NOT NULL DEFAULT 0"}, {"invitation_id", "TEXT NOT NULL DEFAULT ''"}}); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	// Live pages (a dev server forwarded): from the same release as pages.
	// Typed approvals (v0.21.21): what kind of request, and its payload.
	if err := addColumns(db, "approvals", [][2]string{{"kind", "TEXT NOT NULL DEFAULT ''"}, {"payload", "TEXT NOT NULL DEFAULT ''"}}); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	if err := addColumns(db, "pages", [][2]string{{"live", "INTEGER NOT NULL DEFAULT 0"}, {"port", "INTEGER NOT NULL DEFAULT 0"},
		{"template", "TEXT NOT NULL DEFAULT ''"}, {"template_version", "TEXT NOT NULL DEFAULT ''"}, {"manifest", "TEXT NOT NULL DEFAULT ''"},
		// Who may change its records ('' everyone who sees it, managers; v0.22.2).
		{"editors", "TEXT NOT NULL DEFAULT ''"}}); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	if err := addColumns(db, "messages", [][2]string{{"broadcast_id", "TEXT NOT NULL DEFAULT ''"}, {"attachments", "TEXT NOT NULL DEFAULT '[]'"},
		{"from_owner", "TEXT NOT NULL DEFAULT ''"}, {"project", "TEXT NOT NULL DEFAULT ''"}, {"after", "TEXT NOT NULL DEFAULT ''"},
		// Questions to a task's sender: the question, the task waiting on it,
		// the steer answering it.
		{"ask", "INTEGER NOT NULL DEFAULT 0"}, {"asking", "TEXT NOT NULL DEFAULT ''"}, {"answers", "TEXT NOT NULL DEFAULT ''"},
		{"hold_until", "TEXT NOT NULL DEFAULT ''"}, {"hold_wake", "TEXT NOT NULL DEFAULT ''"}, {"hold_recovered", "INTEGER NOT NULL DEFAULT 0"},
		// What a task's recipient suggests its sender says next.
		{"suggest", "TEXT NOT NULL DEFAULT ''"},
		// When its recipient last showed it is working on it (ms).
		{"active_at", "INTEGER NOT NULL DEFAULT 0"},
		// Exact first acknowledgement for queue statistics; old rows remain unknown.
		{"first_acked_at", "INTEGER NOT NULL DEFAULT -1"},
		// The automation that sent it (v0.19.22).
		{"automation", "TEXT NOT NULL DEFAULT ''"},
		// The workflow run that sent it, and a reply's structured result
		// (v0.21).
		{"workflow", "TEXT NOT NULL DEFAULT ''"}, {"result", "TEXT NOT NULL DEFAULT ''"},
		// Why the hub admitted it past its recipient's allow list (v0.21.2).
		{"granted_by", "TEXT NOT NULL DEFAULT ''"}}); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	for table, cols := range map[string][][2]string{
		"machines": {{"owner", "TEXT NOT NULL DEFAULT ''"}},
		// The computers a project's coordinators may start agents on (v0.21.2).
		// Member groups (JSON list, in display order; v0.21.34).
		"projects": {{"files_max", "INTEGER NOT NULL DEFAULT 0"}, {"bytes_max", "INTEGER NOT NULL DEFAULT 0"}, {"coordinator_automations", "INTEGER NOT NULL DEFAULT 0"}, {"coordinator_invites", "INTEGER NOT NULL DEFAULT 0"}, {"machines", "TEXT NOT NULL DEFAULT '[]'"}, {"member_groups", "TEXT NOT NULL DEFAULT '[]'"}},
		// A member's group in its project ('' = ungrouped).
		"project_members": {{"group_name", "TEXT NOT NULL DEFAULT ''"}},
		"console_tokens":  {{"member_id", "TEXT NOT NULL DEFAULT ''"}},
		"project_docs":    {{"content_type", "TEXT NOT NULL DEFAULT ''"}, {"size", "INTEGER NOT NULL DEFAULT 0"}, {"blob", "TEXT NOT NULL DEFAULT ''"}},
		// A template's icon (v0.20.7).
		"template_versions": {{"icon", "TEXT NOT NULL DEFAULT ''"}},
		// What a webhook call carried (JSON; '' for other runs).
		"automation_runs": {{"request", "TEXT NOT NULL DEFAULT ''"},
			// The workflow run a workflow automation started (v0.21).
			{"workflow_run", "TEXT NOT NULL DEFAULT ''"}},
		// A run step's input (JSON; v0.21).
		"run_steps": {{"payload", "TEXT NOT NULL DEFAULT ''"},
			// Send-backs, the tasks sent, and what they cost (v0.21.1).
			{"bounces", "INTEGER NOT NULL DEFAULT 0"}, {"tasks", "TEXT NOT NULL DEFAULT ''"}, {"usage", "TEXT NOT NULL DEFAULT ''"}},
		// A workflow an automation starts, and the run's inputs (v0.21).
		"automations": {{"project_scoped", "INTEGER NOT NULL DEFAULT 0"}, {"starts_at", "INTEGER NOT NULL DEFAULT 0"}, {"ends_at", "INTEGER NOT NULL DEFAULT 0"}, {"once_only", "INTEGER NOT NULL DEFAULT 0"}, {"disabled_reason", "TEXT NOT NULL DEFAULT ''"}, {"created_by", "TEXT NOT NULL DEFAULT ''"}, {"approved_by", "TEXT NOT NULL DEFAULT ''"}, {"workflow", "TEXT NOT NULL DEFAULT ''"}, {"inputs", "TEXT NOT NULL DEFAULT ''"}},
	} {
		if err := addColumns(db, table, cols); err != nil {
			db.Close()
			return nil, fmt.Errorf("migrate: %w", err)
		}
	}
	if err := migrateQuotaAccounts(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate quota accounts: %w", err)
	}
	if err := migrateMessageCoordination(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	for _, ddl := range []string{
		`CREATE INDEX IF NOT EXISTS idx_agents_session ON agents(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_task_stats ON messages(created_at, id, from_agent, to_agent, project, status, first_acked_at) WHERE type='task'`,
		`CREATE INDEX IF NOT EXISTS idx_messages_stats_reply ON messages(reply_to, type, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_stats_thread ON messages(thread_id, from_agent, type, created_at, to_agent)`,
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
		// Per-agent recent lists take a bounded page from each direction
		// before loading message bodies, ordered by update rather than creation.
		`CREATE INDEX IF NOT EXISTS idx_messages_from_updated ON messages(from_agent, updated_at DESC, id)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_to_updated ON messages(to_agent, updated_at DESC, id)`,
		// Refresh lists and thread membership checks read metadata without
		// fetching large message bodies from the table.
		`CREATE INDEX IF NOT EXISTS idx_messages_recent ON messages(updated_at DESC, id, thread_id, from_agent, to_agent, project)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_thread_parties ON messages(thread_id, updated_at DESC, id, from_agent, to_agent, project)`,
	} {
		if _, err := db.Exec(ddl); err != nil {
			db.Close()
			return nil, fmt.Errorf("migrate: %w", err)
		}
	}
	if err := migrateTaskMonitor(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate task monitor: %w", err)
	}
	if t != nil {
		t.live.Store(true) // (from here on: the migrations are not timed)
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
	{"invited_by", "TEXT NOT NULL DEFAULT ''"},
}

// addColumns adds missing columns (idempotent, forward-only migrations).
func addColumns(db schemaConnection, table string, cols [][2]string) error {
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
