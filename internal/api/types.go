// Package api defines the hub's wire contract (JSON DTOs) shared by the HTTP
// server and the Go client. Adding optional fields is non-breaking; renaming
// or removing fields is a breaking change.
package api

import "time"

// Version of the wire contract, exposed at GET /v1/health.
const Version = "v1"

// Binding ties an agent to one session of one harness in one environment.
type Binding struct {
	Harness     string `json:"harness,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
	Environment string `json:"environment,omitempty"`
	Workdir     string `json:"workdir,omitempty"`
}

// Delivery configures how the hub wakes the agent. Secret is write-only:
// accepted on registration, never returned (HasSecret reports it is set).
type Delivery struct {
	Mode      string            `json:"mode"`
	URL       string            `json:"url,omitempty"`
	Secret    string            `json:"secret,omitempty"`
	HasSecret bool              `json:"has_secret,omitempty"`
	Params    map[string]string `json:"params,omitempty"`
}

// Agent is the public view of a registered agent.
type Agent struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name,omitempty"`
	Aliases     []string `json:"aliases,omitempty"`
	// State is working (has a task in progress or held), pending (tasks
	// queued, none started), idle or offline; Queued/InProgress/Held count
	// open messages addressed to it.
	State      string `json:"state,omitempty"`
	Queued     int    `json:"queued"`
	InProgress int    `json:"in_progress"`
	Held       int    `json:"held"`
	// StatusText is the agent's public progress line (myco agent status set).
	StatusText   string     `json:"status_text,omitempty"`
	StatusAt     *time.Time `json:"status_at,omitempty"`
	Description  string     `json:"description"`
	Capabilities []string   `json:"capabilities"`
	Labels       []string   `json:"labels"`
	Binding      Binding    `json:"binding"`
	Delivery     Delivery   `json:"delivery"`
	AllowFrom    []string   `json:"allow_from"`
	EnrolledBy   string     `json:"enrolled_by,omitempty"`
	Online       bool       `json:"online"`
	CreatedAt    time.Time  `json:"created_at"`
	LastSeenAt   *time.Time `json:"last_seen_at,omitempty"`
	// Kind is agent (an AI session), shell (exec/shell node), human, or
	// machine (a machine's node for starting sessions: not shown as a node).
	Kind string `json:"kind,omitempty"`
	// MachineID is the machine it runs on (also "env:<host>" for machines
	// whose runners predate machine reporting).
	MachineID string `json:"machine_id,omitempty"`
	// Runtime is its CLI and model, as its runner reports them.
	Runtime *Runtime `json:"runtime,omitempty"`
}

// Runtime is what an agent runs on.
type Runtime struct {
	CLI        string `json:"cli,omitempty"`
	CLIVersion string `json:"cli_version,omitempty"`
	Model      string `json:"model,omitempty"`
	// ModelSource: configured (the agent's runner.model), cli-config (the
	// CLI's own settings), observed (seen in its last turn), or default.
	ModelSource string `json:"model_source,omitempty"`
	Access      string `json:"access,omitempty"`
	// Provider is the model provider's display name (never its key);
	// ProviderSource: pinned (chosen for this agent), machine (the
	// machine's current setting), env_file (from runner.env_file) or
	// missing (the pinned provider no longer exists; the machine's is used).
	Provider       string `json:"provider,omitempty"`
	ProviderSource string `json:"provider_source,omitempty"`
}

// Machine is a computer, server or cloud environment agents run on.
type Machine struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Hostname    string     `json:"hostname,omitempty"`
	OS          string     `json:"os,omitempty"`
	Arch        string     `json:"arch,omitempty"`
	MycoVersion string     `json:"myco_version,omitempty"`
	Online      bool       `json:"online"`
	LastSeenAt  *time.Time `json:"last_seen_at,omitempty"`
	// CanSpawn: its machine node is online, so sessions can be started on it.
	CanSpawn bool `json:"can_spawn"`
	// Legacy: its runners predate machine reporting (grouped by host name).
	Legacy bool     `json:"legacy,omitempty"`
	Agents []string `json:"agents"`
}

// MachineList is the body of GET /v1/machines.
type MachineList struct {
	Machines []Machine `json:"machines"`
}

// MachineReport is the body of PUT /v1/me/machine (sent by runners).
type MachineReport struct {
	// Fingerprint is a hash of the machine's own stable ID (never the ID).
	Fingerprint string `json:"fingerprint"`
	Name        string `json:"name,omitempty"`
	Hostname    string `json:"hostname,omitempty"`
	OS          string `json:"os,omitempty"`
	Arch        string `json:"arch,omitempty"`
	MycoVersion string `json:"myco_version,omitempty"`
}

// RenameMachineRequest is the body of PATCH /v1/machines/{id}.
type RenameMachineRequest struct {
	Name string `json:"name"`
}

// Message is the public view of a message.
type Message struct {
	ID             string       `json:"id"`
	From           string       `json:"from"`
	To             string       `json:"to"`
	ThreadID       string       `json:"thread_id"`
	ReplyTo        string       `json:"reply_to,omitempty"`
	Type           string       `json:"type"`
	Body           string       `json:"body"`
	Status         string       `json:"status"`
	Attempts       int          `json:"attempts"`
	Error          string       `json:"error,omitempty"`
	IdempotencyKey string       `json:"idempotency_key,omitempty"`
	BroadcastID    string       `json:"broadcast_id,omitempty"`
	Attachments    []Attachment `json:"attachments,omitempty"`
	LeaseUntil     *time.Time   `json:"lease_until,omitempty"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

// RegisterRequest is the body of POST /v1/agents.
type RegisterRequest struct {
	Name         string   `json:"name"`
	DisplayName  string   `json:"display_name,omitempty"`
	Description  string   `json:"description,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	AllowFrom    []string `json:"allow_from,omitempty"`
	Labels       []string `json:"labels,omitempty"`
	Binding      Binding  `json:"binding,omitempty"`
	Delivery     Delivery `json:"delivery,omitempty"`
	RotateToken  bool     `json:"rotate_token,omitempty"`
}

// LabelsRequest adds/removes labels (a bare key in Remove drops all its values).
type LabelsRequest struct {
	Add    []string `json:"add,omitempty"`
	Remove []string `json:"remove,omitempty"`
}

// BroadcastRequest is the body of POST /v1/broadcasts.
type BroadcastRequest struct {
	Selector       string   `json:"selector"`
	Type           string   `json:"type,omitempty"`
	Body           string   `json:"body"`
	IdempotencyKey string   `json:"idempotency_key,omitempty"`
	IncludeSelf    bool     `json:"include_self,omitempty"`
	Attachments    []string `json:"attachments,omitempty"`
}

// BroadcastSkip is a matching agent that was not messaged.
type BroadcastSkip struct {
	Agent  string `json:"agent"`
	Reason string `json:"reason"`
}

// BroadcastResponse lists the per-recipient messages of a broadcast.
type BroadcastResponse struct {
	BroadcastID string          `json:"broadcast_id"`
	Messages    []Message       `json:"messages"`
	Skipped     []BroadcastSkip `json:"skipped"`
	Duplicate   bool            `json:"duplicate"`
	DryRun      bool            `json:"dry_run,omitempty"`
}

// BroadcastDelivery is one recipient's message and its answer, if any.
type BroadcastDelivery struct {
	Message Message  `json:"message"`
	Reply   *Message `json:"reply,omitempty"`
}

// BroadcastStatus is returned by GET /v1/broadcasts/{id}.
type BroadcastStatus struct {
	BroadcastID string              `json:"broadcast_id"`
	Total       int                 `json:"total"`
	Answered    int                 `json:"answered"`
	Deliveries  []BroadcastDelivery `json:"deliveries"`
}

// BindRequest is the body of PUT /v1/me/binding (empty fields unchanged).
type BindRequest = Binding

// PendingResponse is returned by GET /v1/inbox/wait.
type PendingResponse struct {
	Pending  []Message `json:"pending"`
	TimedOut bool      `json:"timed_out"`
}

// RegisterResponse returns the agent and, once, its token.
type RegisterResponse struct {
	Agent   Agent  `json:"agent"`
	Token   string `json:"token,omitempty"`
	Created bool   `json:"created"`
	DryRun  bool   `json:"dry_run,omitempty"`
}

// SendRequest is the body of POST /v1/messages.
type SendRequest struct {
	To             string   `json:"to"`
	Type           string   `json:"type,omitempty"`
	Body           string   `json:"body"`
	ThreadID       string   `json:"thread_id,omitempty"`
	IdempotencyKey string   `json:"idempotency_key,omitempty"`
	Attachments    []string `json:"attachments,omitempty"`
}

// Attachment is a file referenced by a message.
type Attachment struct {
	FileID      string `json:"file_id"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
	ContentType string `json:"content_type,omitempty"`
}

// File is the metadata of an uploaded file.
type File struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Size        int64      `json:"size"`
	SHA256      string     `json:"sha256"`
	ContentType string     `json:"content_type,omitempty"`
	Owner       string     `json:"owner"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

// SendResponse returns the queued (or previously queued) message.
type SendResponse struct {
	Message   Message `json:"message"`
	Duplicate bool    `json:"duplicate"`
	DryRun    bool    `json:"dry_run,omitempty"`
	// ViaAlias is set when the message was addressed to an alias.
	ViaAlias string `json:"via_alias,omitempty"`
}

// StatusRequest is the body of PUT /v1/me/status ("" clears).
type StatusRequest struct {
	Text string `json:"text"`
}

// ProfileRequest changes cosmetic fields; omitted fields are unchanged and
// an empty display_name clears it.
type ProfileRequest struct {
	DisplayName *string `json:"display_name,omitempty"`
	Description *string `json:"description,omitempty"`
	// AllowFrom replaces who may send tasks/notes (admin only; [] = anyone,
	// refused for exec nodes). The agent's token is not changed.
	AllowFrom *[]string `json:"allow_from,omitempty"`
	// AcceptExecRisk confirms that an exec node with an empty allow_from
	// should run commands from every agent (stored as "*").
	AcceptExecRisk bool `json:"accept_exec_risk,omitempty"`
}

// Alias is a re-pointable address for an agent.
type Alias struct {
	Alias     string    `json:"alias"`
	Target    string    `json:"target"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AliasRequest is the body of PUT /v1/aliases/{alias}.
type AliasRequest struct {
	Target string `json:"target"`
}

// AliasList wraps aliases.
type AliasList struct {
	Aliases []Alias `json:"aliases"`
}

// PullRequest is the body of POST /v1/inbox/pull.
type PullRequest struct {
	Limit       int `json:"limit,omitempty"`
	WaitSeconds int `json:"wait_seconds,omitempty"`
	// Types, if set, only pulls messages of these types (e.g. ["steer"]).
	Types []string `json:"types,omitempty"`
	// Via is "runner" when an agent's runner pulls; such pulls get nothing
	// while a session of the agent keeps `inbox wait` running.
	Via string `json:"via,omitempty"`
}

// MessageList wraps a list of messages.
type MessageList struct {
	Messages []Message `json:"messages"`
}

// AgentList wraps a list of agents.
type AgentList struct {
	Agents []Agent `json:"agents"`
}

// ReplyRequest is the body of POST /v1/messages/{id}/reply.
type ReplyRequest struct {
	Body        string   `json:"body"`
	Attachments []string `json:"attachments,omitempty"`
}

// ReleaseRequest is the body of POST /v1/messages/{id}/release.
type ReleaseRequest struct {
	DelaySeconds int `json:"delay_seconds"`
	// Reason "in_use" tells the hub the agent's session was busy (e.g. open
	// in a terminal), which the console shows with a way to fix it.
	Reason string `json:"reason,omitempty"`
}

// FailRequest is the body of POST /v1/messages/{id}/fail.
type FailRequest struct {
	Reason string `json:"reason"`
}

// MessageResult wraps a single message result of a state change.
type MessageResult struct {
	Message Message `json:"message"`
	DryRun  bool    `json:"dry_run,omitempty"`
}

// WaitResponse is returned by GET /v1/messages/{id}/reply. Reply is nil when
// the wait timed out.
type WaitResponse struct {
	Reply    *Message `json:"reply"`
	TimedOut bool     `json:"timed_out"`
	// Progress lists the task's progress updates newer than progress_since
	// (only when that parameter is given).
	Progress []Message `json:"progress,omitempty"`
}

// Error is the JSON error body for every non-2xx response.
type Error struct {
	Code       string `json:"error"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
	Retryable  bool   `json:"retryable"`
}

// Health is returned by GET /v1/health.
type Health struct {
	OK      bool   `json:"ok"`
	Version string `json:"version"`
	// PublicURL is the hub's URL as clients see it (e.g. its tunnel), when known.
	PublicURL string `json:"public_url,omitempty"`
	// ServerVersion is the hub's myco build (Version is the wire contract).
	// Hubs older than v0.16.6 leave it out.
	ServerVersion string `json:"server_version,omitempty"`
	// LatestVersion is the newest published release the hub knows of (its
	// daily background check; empty when disabled or not checked yet), and
	// UpdateAvailable says it is newer than ServerVersion.
	LatestVersion   string `json:"latest_version,omitempty"`
	UpdateAvailable bool   `json:"update_available,omitempty"`
}

// Enrollment is the public view of an enrollment (the token is never shown
// again after creation).
type Enrollment struct {
	ID           string     `json:"id"`
	NamePrefix   string     `json:"name_prefix"`
	Harness      string     `json:"harness,omitempty"`
	AllowFrom    []string   `json:"allow_from"`
	Capabilities []string   `json:"capabilities"`
	Labels       []string   `json:"labels"`
	Note         string     `json:"note,omitempty"`
	MaxUses      int        `json:"max_uses"`
	Uses         int        `json:"uses"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	RevokedAt    *time.Time `json:"revoked_at,omitempty"`
	Active       bool       `json:"active"`
	CreatedAt    time.Time  `json:"created_at"`
	// Kind is "" (plain), "link" (join link) or "machine" (a machine that
	// joined through link ParentID).
	Kind     string `json:"kind,omitempty"`
	ParentID string `json:"parent_id,omitempty"`
	Machine  string `json:"machine,omitempty"`
}

// CreateEnrollmentRequest is the body of POST /v1/enrollments.
type CreateEnrollmentRequest struct {
	Kind         string   `json:"kind,omitempty"`
	NamePrefix   string   `json:"name_prefix"`
	Harness      string   `json:"harness,omitempty"`
	AllowFrom    []string `json:"allow_from,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Labels       []string `json:"labels,omitempty"`
	Note         string   `json:"note,omitempty"`
	MaxUses      int      `json:"max_uses,omitempty"`
	TTLSeconds   int64    `json:"ttl_seconds,omitempty"`
}

// CreateEnrollmentResponse returns the enrollment and, once, its token.
type CreateEnrollmentResponse struct {
	Enrollment Enrollment `json:"enrollment"`
	Token      string     `json:"token,omitempty"`
	DryRun     bool       `json:"dry_run,omitempty"`
	// JoinURL is set for links: the page/script a machine opens to join.
	JoinURL string `json:"join_url,omitempty"`
}

// JoinRequest is the body of POST /v1/join (authenticated with a link).
type JoinRequest struct {
	Machine string `json:"machine"`
}

// JoinResponse gives a machine its own enrollment token (shown once).
type JoinResponse struct {
	Machine    string   `json:"machine"`
	Token      string   `json:"token,omitempty"`
	Created    bool     `json:"created"`
	NamePrefix string   `json:"name_prefix"`
	Harness    string   `json:"harness,omitempty"`
	AllowFrom  []string `json:"allow_from"`
	Labels     []string `json:"labels"`
	DryRun     bool     `json:"dry_run,omitempty"`
}

// EnrollmentList wraps a list of enrollments.
type EnrollmentList struct {
	Enrollments []Enrollment `json:"enrollments"`
}

// EnrollRequest is the body of POST /v1/enroll (authenticated with an
// enrollment token).
type EnrollRequest struct {
	Name         string   `json:"name,omitempty"`
	DisplayName  string   `json:"display_name,omitempty"`
	Description  string   `json:"description,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Labels       []string `json:"labels,omitempty"`
	Binding      Binding  `json:"binding"`
}

// Viewer says whose permissions an overview was built with.
type Viewer struct {
	Kind string `json:"kind"` // agent | admin
	Name string `json:"name,omitempty"`
}

// AgentStatus is one agent with live counters (GET /v1/overview).
type AgentStatus struct {
	Agent      Agent     `json:"agent"`
	State      string    `json:"state"` // working | pending | idle | offline
	Queued     int       `json:"queued"`
	InProgress int       `json:"in_progress"`
	Held       int       `json:"held"`
	Current    []Message `json:"current,omitempty"`
	// Receiving is how the agent gets messages right now: "session" (a
	// session keeps `inbox wait` running), "runner" (a runner wakes the
	// session per message) or empty (nothing is receiving).
	Receiving string `json:"receiving,omitempty"`
	// SessionInUse: the runner recently found the session in use elsewhere
	// and handed messages back instead of interrupting it.
	SessionInUse bool `json:"session_in_use,omitempty"`
}

// OverviewTotals counts agents by state.
type OverviewTotals struct {
	Agents  int `json:"agents"`
	Working int `json:"working"`
	Pending int `json:"pending"`
	Idle    int `json:"idle"`
	Offline int `json:"offline"`
}

// Overview is a network snapshot for dashboards.
type Overview struct {
	// Machines are only in the admin view.
	Machines            []Machine      `json:"machines,omitempty"`
	GeneratedAt         time.Time      `json:"generated_at"`
	Viewer              Viewer         `json:"viewer"`
	OnlineWindowSeconds int            `json:"online_window_seconds"`
	Totals              OverviewTotals `json:"totals"`
	Agents              []AgentStatus  `json:"agents"`
	Recent              []Message      `json:"recent"`
}

// SpawnRequest asks a machine node (harness "machine") to start a new agent
// session in a directory on its machine and join it to the network. It is
// sent as the body of a task; see `myco agent spawn`.
type SpawnRequest struct {
	// CLI is the agent CLI to start: claude, codex, or a preset (pi, kiro, ...).
	CLI string `json:"cli"`
	// Dir is the working directory on the node's machine ("~/..." allowed).
	Dir string `json:"dir"`
	// Name is the new agent's name; the machine's prefix is added when missing.
	Name        string   `json:"name,omitempty"`
	DisplayName string   `json:"display_name,omitempty"`
	Description string   `json:"description,omitempty"`
	Labels      []string `json:"labels,omitempty"`
	// Access is the session's permission level (default workspace); the
	// machine caps it (default cap: workspace).
	Access string `json:"access,omitempty"`
	Model  string `json:"model,omitempty"`
	// Prompt is extra first instructions for the new session.
	Prompt string `json:"prompt,omitempty"`
	// AllowFrom, for a shell node (CLI "exec"), names the agents that may
	// send it commands. Only the hub admin can add shell nodes: the hub
	// turns this into a one-use exec enrollment token (EnrollToken), which
	// the machine joins the node with.
	AllowFrom   []string `json:"allow_from,omitempty"`
	EnrollToken string   `json:"enroll_token,omitempty"`
}

// SpawnResult is a machine node's answer to a SpawnRequest.
type SpawnResult struct {
	Agent     string `json:"agent,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	CLI       string `json:"cli,omitempty"`
	Workdir   string `json:"workdir,omitempty"`
	Access    string `json:"access,omitempty"`
	// Reply is the new session's answer to its first turn.
	Reply string `json:"reply,omitempty"`
	// Denied: the machine refused the request (policy); Error says why.
	Denied bool   `json:"denied,omitempty"`
	Error  string `json:"error,omitempty"`
}

// ExecResult is the reply body of an exec node (harness "exec").
type ExecResult struct {
	ExitCode   int    `json:"exit_code"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	DurationMS int64  `json:"duration_ms"`
	// Truncated: stdout or stderr exceeded the node's max_output.
	Truncated bool `json:"truncated,omitempty"`
	// TimedOut: the command was killed at the node's timeout (exit 124).
	TimedOut bool `json:"timed_out,omitempty"`
	// Denied: the node refused to run it (exit 126); Error says why.
	Denied bool   `json:"denied,omitempty"`
	Error  string `json:"error,omitempty"`
}

// ConsoleToken is a per-device admin credential for the console app (the
// plain token is only in CreateConsoleTokenResponse).
type ConsoleToken struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Active     bool       `json:"active"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

// CreateConsoleTokenRequest is the body of POST /v1/console/tokens.
type CreateConsoleTokenRequest struct {
	Name string `json:"name"`
}

// CreateConsoleTokenResponse returns the token record and, once, the token.
type CreateConsoleTokenResponse struct {
	ConsoleToken ConsoleToken `json:"console_token"`
	Token        string       `json:"token"`
}

// ConsoleTokenList is the body of GET /v1/console/tokens.
type ConsoleTokenList struct {
	Tokens []ConsoleToken `json:"tokens"`
}

// Conversation is a thread the console (the administrator, from the app)
// takes part in.
type Conversation struct {
	ThreadID  string    `json:"thread_id"`
	Agent     string    `json:"agent,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
	First     Message   `json:"first"`
	Last      Message   `json:"last"`
	Messages  int       `json:"messages"`
}

// ConversationList is the body of GET /v1/console/conversations.
type ConversationList struct {
	Conversations []Conversation `json:"conversations"`
}

// SteerRequest is the body of POST /v1/console/messages/{id}/steer.
type SteerRequest struct {
	Body string `json:"body"`
}
