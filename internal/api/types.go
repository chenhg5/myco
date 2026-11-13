// Package api defines the hub's wire contract (JSON DTOs) shared by the HTTP
// server and the Go client. Adding optional fields is non-breaking; renaming
// or removing fields is a breaking change.
package api

import (
	"encoding/json"
	"time"
)

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
	StatusText  string     `json:"status_text,omitempty"`
	StatusAt    *time.Time `json:"status_at,omitempty"`
	Description string     `json:"description"`
	// Capabilities are the names of Skills (older clients read these).
	Capabilities []string `json:"capabilities"`
	// Skills: what others can pick this agent for (hubs before v0.18.21
	// leave it out; read Capabilities then).
	Skills []Skill `json:"skills,omitempty"`
	// Visibility: "team" (everyone on the hub) or "private" (only its owner
	// and the owner's agents). Absent on hubs before v0.18.22: team.
	Visibility string     `json:"visibility,omitempty"`
	Labels     []string   `json:"labels"`
	Binding    Binding    `json:"binding"`
	Delivery   Delivery   `json:"delivery"`
	AllowFrom  []string   `json:"allow_from"`
	EnrolledBy string     `json:"enrolled_by,omitempty"`
	Online     bool       `json:"online"`
	CreatedAt  time.Time  `json:"created_at"`
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`
	// Kind is agent (an AI session), shell (exec/shell node), human, or
	// machine (a machine's node for starting sessions: not shown as a node).
	Kind string `json:"kind,omitempty"`
	// Owner is the member who added it (their name); OwnerID their id.
	Owner   string `json:"owner,omitempty"`
	OwnerID string `json:"owner_id,omitempty"`
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
	// Owner is the member whose machine it is (name); OwnerID their id.
	Owner   string `json:"owner,omitempty"`
	OwnerID string `json:"owner_id,omitempty"`
}

// MachineList is the body of GET /v1/machines.
type MachineList struct {
	Machines []Machine `json:"machines"`
}

type MachineRemoveResult struct {
	Machine Machine  `json:"machine"`
	Removed []string `json:"removed"`
	DryRun  bool     `json:"dry_run"`
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
	ID             string `json:"id"`
	From           string `json:"from"`
	To             string `json:"to"`
	ThreadID       string `json:"thread_id"`
	ReplyTo        string `json:"reply_to,omitempty"`
	After          string `json:"after,omitempty"`
	Type           string `json:"type"`
	Body           string `json:"body"`
	Status         string `json:"status"`
	Attempts       int    `json:"attempts"`
	Error          string `json:"error,omitempty"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
	BroadcastID    string `json:"broadcast_id,omitempty"`
	// Project is the project its thread belongs to.
	Project string `json:"project,omitempty"`
	// FromOwner is the person behind the sender: the member who wrote it in
	// the app, or the sending agent's owner (member name).
	FromOwner string `json:"from_owner,omitempty"`
	// FromDisplay is the sending node's display name, when it has one: what
	// people call it (inbox pulls, from v0.18.36). Address it by From.
	FromDisplay string       `json:"from_display,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
	// E2E: the body is end-to-end encrypted (an envelope only the
	// message's readers can open; see docs/design/e2e-encryption.md).
	E2E bool `json:"e2e,omitempty"`
	// Unreadable says why a client could not decrypt an end-to-end
	// encrypted body (set by the client, never by the hub).
	Unreadable string     `json:"unreadable,omitempty"`
	LeaseUntil *time.Time `json:"lease_until,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	ActiveAt   *time.Time `json:"active_at,omitempty"`
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
// Skill is one kind of work an agent can be asked to do.
type Skill struct {
	Name     string   `json:"name"`
	Summary  string   `json:"summary,omitempty"`
	Examples []string `json:"examples,omitempty"`
}

// VisibilityRequest is the body of PUT /v1/agents/{name}/visibility.
type VisibilityRequest struct {
	Visibility string `json:"visibility"`
}

// SkillsRequest is the body of PUT /v1/me/skills and
// /v1/agents/{name}/skills: the complete new list ([] clears it).
type SkillsRequest struct {
	Skills []Skill `json:"skills"`
}

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
	After          string   `json:"after,omitempty"`
	IdempotencyKey string   `json:"idempotency_key,omitempty"`
	Attachments    []string `json:"attachments,omitempty"`
	// Project puts a new thread in a project (name); a message in an
	// existing thread belongs to the thread's project. Hubs before v0.18.26
	// ignore it.
	Project string `json:"project,omitempty"`
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
	// Since echoes updated_since when the hub honoured it (from v0.18.35):
	// Messages are then only those that changed after it. Absent: the
	// whole list (also what an older hub answers).
	Since int64 `json:"since,omitempty"`
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
	ID string `json:"id"`
	// Owner is the member who created it (id); what joins through it is theirs.
	OwnerID      string     `json:"owner_id,omitempty"`
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
	// AgentGuide: the join script and guide end with the agent's last step
	// (myco join), and JoinURL + "/" + a project name joins that project
	// too. Older hubs leave it out; give their agents the steps yourself.
	AgentGuide bool `json:"agent_guide,omitempty"`
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
	// ForkFrom copies an existing session (of the same CLI, on that machine)
	// into the new agent instead of starting an empty one: it keeps the
	// original's context, and the original is left untouched.
	ForkFrom string `json:"fork_from,omitempty"`
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

// CleanupRequest is the body of POST /v1/agents/cleanup.
type CleanupRequest struct {
	// OfflineDays: remove nodes offline for at least this many days (>= 1).
	OfflineDays int `json:"offline_days"`
}

// CleanupResponse lists the nodes removed (or, with dry_run, that would be):
// only those the caller may manage.
type CleanupResponse struct {
	Agents  []Agent `json:"agents"`
	Removed bool    `json:"removed"`
	DryRun  bool    `json:"dry_run,omitempty"`
}

// Peer is another node a node exchanged messages with
// (GET /v1/console/agents/{name}/peers).
type Peer struct {
	Agent    string   `json:"agent"`
	Messages int      `json:"messages"`
	Last     *Message `json:"last,omitempty"`
}

// PeerList is the body of GET /v1/console/agents/{name}/peers.
type PeerList struct {
	Peers []Peer `json:"peers"`
}

// Exchange is the body of GET /v1/console/exchanges: messages between two
// nodes, newest first; more says older ones exist (page with before).
type Exchange struct {
	Messages []Message `json:"messages"`
	More     bool      `json:"more"`
	// Since: as in MessageList. With it, Messages are the changes after it,
	// oldest change first, and More means there are more changes.
	Since int64 `json:"since,omitempty"`
}

// RecentItem is one row of GET /v1/console/recent: a thread, or the
// exchange between an agent (or the console) and a shell node.
type RecentItem struct {
	// Kind is "thread" or "pair".
	Kind     string `json:"kind"`
	ThreadID string `json:"thread_id"`
	// Other and Node: a pair's two sides (Node is the shell node).
	Other string `json:"other,omitempty"`
	Node  string `json:"node,omitempty"`
	// Participants, in order of first appearance.
	Participants []string  `json:"participants"`
	UpdatedAt    time.Time `json:"updated_at"`
	Messages     int       `json:"messages"`
	First        Message   `json:"first"`
	Last         Message   `json:"last"`
	// Task is the newest task, Answer its reply or failure event.
	Task   *Message `json:"task,omitempty"`
	Answer *Message `json:"answer,omitempty"`
	// Open is the newest task not finished yet; it can be older than Task
	// (a task stuck behind a newer one that finished). Hubs before v0.18.40
	// leave it out.
	Open *Message `json:"open,omitempty"`
	// State: "queued", "working", "done", "failed" or "" (no task). A shell
	// command that exited non-zero is "done" here; its exit code is in
	// Answer's body.
	State string `json:"state,omitempty"`
	// Project is the project the thread belongs to.
	Project string `json:"project,omitempty"`
}

// TraceEvent is one step of the run that handled a task: what the agent
// said, thought and did. Data is the step (harness.Event): kind, text,
// tool, id, input, output, failed, and usage figures.
type TraceEvent struct {
	Seq  int             `json:"seq"`
	Kind string          `json:"kind"`
	Data json.RawMessage `json:"data"`
	At   time.Time       `json:"at,omitempty"`
}

// TraceUpload is the body of POST /v1/messages/{id}/trace (the recipient's
// runner, while it runs the task).
type TraceUpload struct {
	Events []TraceEvent `json:"events"`
}

// Trace is GET /v1/messages/{id}/trace and /v1/console/messages/{id}/trace:
// the steps after ?after=<seq>, and the task's status (a finished task's
// trace is complete).
type Trace struct {
	Message string       `json:"message"`
	Status  string       `json:"status"`
	Events  []TraceEvent `json:"events"`
}

// RecentList is the body of GET /v1/console/recent; more says older rows
// exist (page with before = the last row's updated_at in unix ms).
type RecentList struct {
	Items []RecentItem `json:"items"`
	More  bool         `json:"more"`
}

// SteerRequest is the body of POST /v1/console/messages/{id}/steer.
type SteerRequest struct {
	Body        string   `json:"body"`
	Attachments []string `json:"attachments,omitempty"`
}

type SteersResponse struct {
	Steers []Message `json:"steers"`
}

// Member is a person using the hub (GET /v1/members).
type Member struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	DisplayName string    `json:"display_name,omitempty"`
	Role        string    `json:"role"` // owner | admin | member
	CreatedAt   time.Time `json:"created_at"`
	Removed     bool      `json:"removed,omitempty"`
}

// MemberList wraps members.
type MemberList struct {
	Members []Member `json:"members"`
}

// InviteRequest is the body of POST /v1/members.
type InviteRequest struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name,omitempty"`
	Role        string `json:"role,omitempty"` // admin | member (default)
}

// InviteResponse returns the new member and, once, their app link.
type InviteResponse struct {
	Member Member `json:"member"`
	// Link opens the hub in the invitee's myco app as them (keep it private).
	Link string `json:"link"`
	// Command joins as them without the app: their agent runs it on their
	// machine (curl -fsSL <link> | bash). Same one-time code as Link.
	Command string `json:"command"`
}

// MemberUpdateRequest is the body of PATCH /v1/members/{member}.
type MemberUpdateRequest struct {
	Name        string  `json:"name,omitempty"`
	DisplayName *string `json:"display_name,omitempty"`
	Role        string  `json:"role,omitempty"`
}

// OwnerRequest gives an agent or machine to a member (name or id).
type OwnerRequest struct {
	Owner string `json:"owner"`
}

// Whoami is GET /v1/console/me: the person behind a console token.
type Whoami struct {
	Member *Member `json:"member,omitempty"` // nil for the hub's admin token
	Admin  bool    `json:"admin,omitempty"`  // the hub's admin token
	// Device is this console token (this app on this device).
	Device *ConsoleToken `json:"device,omitempty"`
	// CanManage: owner or admin (manage members and every node).
	CanManage bool `json:"can_manage"`
}

// AuditEntry is one administrative action.
type AuditEntry struct {
	ID        string    `json:"id"`
	At        time.Time `json:"at"`
	Actor     string    `json:"actor"`      // member, agent or "admin"
	ActorKind string    `json:"actor_kind"` // member | admin | agent
	Action    string    `json:"action"`
	Target    string    `json:"target,omitempty"`
	Detail    string    `json:"detail,omitempty"`
}

// AuditList wraps audit entries, newest first.
type AuditList struct {
	Entries []AuditEntry `json:"entries"`
}

// PairRequest is the body of POST /v1/console/pair: a device trading a
// one-time pairing code (from an app link) for its own console token.
type PairRequest struct {
	Code   string `json:"code"`
	Device string `json:"device"`
}

// PairResponse returns the device's console token (once) and who it is.
type PairResponse struct {
	Token  string       `json:"token"`
	Device ConsoleToken `json:"device"`
	Member *Member      `json:"member,omitempty"`
}

// PairingLink is a one-time app link (POST /v1/console/me/pairings).
type PairingLink struct {
	Link      string    `json:"link"`
	ExpiresAt time.Time `json:"expires_at"`
	// Command uses the same one-time code on the command line instead of
	// the app (myco member accept on another machine).
	Command string `json:"command"`
}

// DeviceList is GET /v1/console/me/devices: the member's devices.
type DeviceList struct {
	Devices []ConsoleToken `json:"devices"`
}

// KeyHolder is one identity that can read a message, with its public keys.
type KeyHolder struct {
	// Who: an agent's name, or "@console:<member>" for a member's device.
	Who string `json:"who"`
	// Role: recipient, sender or owner (in GET /v1/keys/readers).
	Role string `json:"role,omitempty"`
	// Device names a member's device ("" for agents).
	Device  string `json:"device,omitempty"`
	EncKey  string `json:"enc_key"`
	SignKey string `json:"sign_key"`
}

// Readers is GET /v1/keys/readers: who may read a message from one side to
// the other, and the sides with no key (the message then stays plain).
type Readers struct {
	E2E     bool        `json:"e2e"`
	Readers []KeyHolder `json:"readers"`
	Missing []string    `json:"missing,omitempty"`
}

// KeyList is GET /v1/keys?who=a,b: each identity's keys.
type KeyList struct {
	E2E  bool                   `json:"e2e"`
	Keys map[string][]KeyHolder `json:"keys"`
}

// PutKeysRequest is PUT /v1/me/keys and PUT /v1/console/me/keys.
type PutKeysRequest struct {
	EncKey  string `json:"enc_key"`
	SignKey string `json:"sign_key"`
}

// HubSettings is GET/PUT /v1/hub/settings.
type HubSettings struct {
	// E2E: messages are end-to-end encrypted (the owner turns it on).
	E2E bool `json:"e2e"`
}

// Project is a piece of work several agents and people do together.
type Project struct {
	Name       string          `json:"name"`
	Title      string          `json:"title"`
	Goal       string          `json:"goal,omitempty"`
	Brief      string          `json:"brief,omitempty"`
	Links      []ProjectLink   `json:"links"`
	Visibility string          `json:"visibility"` // members | team
	Owner      string          `json:"owner,omitempty"`
	OwnerID    string          `json:"owner_id,omitempty"`
	Archived   bool            `json:"archived,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
	Members    []ProjectMember `json:"members"`
	Docs       []ProjectDoc    `json:"docs"`
	// What the caller may do: edit the content (owner, admins,
	// coordinators), manage it (people: roles, visibility, archive, delete).
	CanEdit   bool `json:"can_edit"`
	CanManage bool `json:"can_manage"`
}

// ProjectLink points at material kept elsewhere.
type ProjectLink struct {
	Title string `json:"title,omitempty"`
	URL   string `json:"url"`
}

// ProjectMember is a node in a project. Agent is its card when the caller
// may see the node (a private node of someone else shows only its name).
type ProjectMember struct {
	Name    string    `json:"name"`
	Role    string    `json:"role"` // member | coordinator
	Note    string    `json:"note,omitempty"`
	AddedBy string    `json:"added_by,omitempty"`
	AddedAt time.Time `json:"added_at"`
	Agent   *Agent    `json:"agent,omitempty"`
}

// ProjectDoc is a shared document; Body only when asked for (?docs=body,
// or GET …/docs/{name}).
type ProjectDoc struct {
	Name      string    `json:"name"`
	Size      int       `json:"size"`
	UpdatedBy string    `json:"updated_by,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body,omitempty"`
	// ContentType is set for uploaded files ("" for documents written as
	// text). Binary is true when the content is not text: read it with GET
	// /v1/projects/{name}/files/{file} (from v0.18.29).
	ContentType string `json:"content_type,omitempty"`
	Binary      bool   `json:"binary,omitempty"`
}

// ProjectList wraps projects.
type ProjectList struct {
	Projects []Project `json:"projects"`
}

// CreateProjectRequest is the body of POST /v1/projects.
type CreateProjectRequest struct {
	Name       string `json:"name"`
	Title      string `json:"title,omitempty"`
	Goal       string `json:"goal,omitempty"`
	Visibility string `json:"visibility,omitempty"`
}

// UpdateProjectRequest is the body of PATCH /v1/projects/{name}; absent
// fields stay. Visibility and Archived are for people managing it.
type UpdateProjectRequest struct {
	Title      *string        `json:"title,omitempty"`
	Goal       *string        `json:"goal,omitempty"`
	Brief      *string        `json:"brief,omitempty"`
	Links      *[]ProjectLink `json:"links,omitempty"`
	Visibility *string        `json:"visibility,omitempty"`
	Archived   *bool          `json:"archived,omitempty"`
}

// ProjectMemberRequest is the body of PUT /v1/projects/{name}/members/{agent}.
type ProjectMemberRequest struct {
	Role *string `json:"role,omitempty"`
	Note *string `json:"note,omitempty"`
}

// ProjectDocRequest is the body of PUT /v1/projects/{name}/docs/{doc}.
type ProjectDocRequest struct {
	Body string `json:"body"`
}
