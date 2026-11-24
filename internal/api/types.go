// Package api defines the hub's wire contract (JSON DTOs) shared by the HTTP
// server and the Go client. Adding optional fields is non-breaking; renaming
// or removing fields is a breaking change.
package api

import (
	"encoding/json"
	"github.com/myco-ai/myco/internal/remotedesktop"
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
	ID          string      `json:"id"`
	Resume      *ResumeInfo `json:"resume,omitempty"`
	Name        string      `json:"name"`
	DisplayName string      `json:"display_name,omitempty"`
	// Avatar is the face people chose: "look:<parts>" (drawn by clients)
	// or "pic:<hash>" (GET /v1/agents/{name}/avatar; the hash changes with
	// the picture). Empty: draw the default one from the id.
	Avatar  string   `json:"avatar,omitempty"`
	Aliases []string `json:"aliases,omitempty"`
	// State is working (has a task in progress or held), pending (tasks
	// queued, none started), idle or offline; Queued/InProgress/Held count
	// open messages addressed to it.
	State      string `json:"state,omitempty"`
	Queued     int    `json:"queued"`
	InProgress int    `json:"in_progress"`
	Held       int    `json:"held"`
	Asking     int    `json:"asking,omitempty"`
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
	InvitedBy  string     `json:"invited_by,omitempty"`
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
	// RunSettings are the model and effort chosen for its runner (absent
	// when none is chosen).
	RunSettings *RunSettings `json:"run_settings,omitempty"`
}

type ResumeInfo struct {
	CreatedAt   time.Time  `json:"created_at"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	LastUsedEnv string     `json:"last_used_env,omitempty"`
}

type ResumeLink struct {
	Token   string     `json:"token,omitempty"`
	URL     string     `json:"url,omitempty"`
	Command string     `json:"command,omitempty"`
	Resume  ResumeInfo `json:"resume"`
}

type ResumeRequest struct {
	Token   string  `json:"token"`
	Binding Binding `json:"binding"`
}

// ProviderOption is an available channel on the runner's own computer; no secrets.
type ProviderOption struct {
	Ref   string `json:"ref"`
	Name  string `json:"name"`
	Host  string `json:"host,omitempty"`
	Model string `json:"model,omitempty"`
}

type EncryptedProvider struct {
	ID   string `json:"id"`
	From string `json:"from"`
	Body string `json:"body"`
}

// Runtime is what an agent runs on.
// RunnerSlots describes one runner process, shared across its workspaces.
// Queue contains only the reporting agent's waiting message IDs.
type RunnerSlots struct {
	MaxConcurrent int                `json:"max_concurrent"`
	InUse         int                `json:"in_use"`
	Waiting       int                `json:"waiting"`
	Queue         []RunnerQueuedTask `json:"queue,omitempty"`
	At            time.Time          `json:"at"`
}
type RunnerQueuedTask struct {
	MessageID string `json:"message_id"`
	Position  int    `json:"position"`
}

type Runtime struct {
	RunnerSlots *RunnerSlots `json:"runner_slots,omitempty"`
	CLI         string       `json:"cli,omitempty"`
	CLIVersion  string       `json:"cli_version,omitempty"`
	Model       string       `json:"model,omitempty"`
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
	// ProviderHost is only the effective endpoint host and port, never URL credentials or a path.
	ProviderHost string            `json:"provider_host,omitempty"`
	Providers    *[]ProviderOption `json:"providers,omitempty"`
	// Effort is the reasoning effort the CLI runs with; EffortSource: app
	// (the agent's run settings) or self (the agent reported it). From v0.19.5.
	Effort       string `json:"effort,omitempty"`
	EffortSource string `json:"effort_source,omitempty"`
	// MaxAccess is the highest access the agent's machine lets its owner set
	// from the app (run settings); a runner that predates it leaves it empty.
	MaxAccess string `json:"max_access,omitempty"`
	// Quota is the account's usage limits as the CLI last reported them.
	Quota *Quota `json:"quota,omitempty"`
	// Features are what the runner supports beyond the basics (e.g.
	// "changes", "ask_access"); apps show a feature's controls only when it
	// is listed. Absent from runners that predate it.
	Features []string `json:"features,omitempty"`
	// Away says why the runner went offline, sent as it stops: "stopped"
	// (it was stopped: by hand, its service, logging out or shutting down).
	// A runner that is running sends none. Env is where it runs when that
	// explains going offline: "wsl" (Windows shuts WSL down when idle).
	Away string `json:"away,omitempty"`
	Env  string `json:"env,omitempty"`
	// Models are the models its CLI offers on that computer, in the CLI's
	// own order (Codex: its models cache). Absent when the CLI does not
	// say (Claude Code: its aliases are fixed) or the runner predates it
	// (v0.21.1).
	Models []RuntimeModel `json:"models,omitempty"`
	// Project-level tool inventory only; user-level tools come from MachineAI.
	MCP          []MCPServer  `json:"mcp,omitempty"`
	Skills       []ToolSkill  `json:"skills,omitempty"`
	Plugins      []ToolPlugin `json:"plugins,omitempty"`
	ToolWarnings []string     `json:"tool_warnings,omitempty"`
}

// Tool inventory contains labels and remote hostnames only: never env,
// headers, argv, or URL paths and queries.
type MCPServer struct {
	Name      string `json:"name"`
	Transport string `json:"transport"`
	Host      string `json:"host,omitempty"`
	Scope     string `json:"scope"`
	Enabled   *bool  `json:"enabled,omitempty"`
}

type ToolSkill struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Scope       string `json:"scope"`
	ManagedBy   string `json:"managed_by,omitempty"`
}

type ToolPlugin struct {
	Name        string `json:"name"`
	Marketplace string `json:"marketplace,omitempty"`
	Enabled     bool   `json:"enabled"`
}

// RuntimeModel is a model a CLI offers: the id its --model takes, a
// display name, and the reasoning efforts it supports.
type RuntimeModel struct {
	ID      string   `json:"id"`
	Name    string   `json:"name,omitempty"`
	Efforts []string `json:"efforts,omitempty"`
}

// Quota is a CLI account's usage limits at one moment (from v0.19.5).
type Quota struct {
	// Source: claude or codex.
	Source  string        `json:"source"`
	At      time.Time     `json:"at"`
	Windows []QuotaWindow `json:"windows"`
}

// QuotaWindow is one limit: Used is the share used (0..1).
type QuotaWindow struct {
	// Name: five_hour, seven_day (Claude Code), primary, secondary (Codex).
	Name     string     `json:"name"`
	Label    string     `json:"label,omitempty"`
	Used     float64    `json:"used"`
	ResetsAt *time.Time `json:"resets_at,omitempty"`
	// Status: allowed, or why work is refused.
	Status string `json:"status,omitempty"`
}

// RunSettings choose how an agent's runner starts its CLI ("" follows the
// machine's own setting). PUT /v1/agents/{name}/run-settings (from v0.19.5).
type RunSettings struct {
	Provider          *string            `json:"provider,omitempty"`
	ProviderEncrypted *EncryptedProvider `json:"provider_encrypted,omitempty"`
	Model             string             `json:"model"`
	Effort            string             `json:"effort"`
	// MCPTools, when present in a request, changes whether configured MCP
	// tools may run without a prompt outside full access.
	MCPTools *bool `json:"mcp_tools,omitempty"`
	// Access: readonly, workspace or full ("" follows the machine). Only the
	// agent's owner may change it; left out of a request, it stays as it is.
	Access string `json:"access,omitempty"`
}

// RunSettingsPatch preserves every field omitted by an App update.
type RunSettingsPatch struct {
	Model             *string            `json:"model,omitempty"`
	Effort            *string            `json:"effort,omitempty"`
	Access            *string            `json:"access,omitempty"`
	MCPTools          *bool              `json:"mcp_tools,omitempty"`
	Provider          *string            `json:"provider,omitempty"`
	ProviderEncrypted *EncryptedProvider `json:"provider_encrypted,omitempty"`
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
	// Ask: a progress update asking the task's sender to decide; its body
	// is the question, then the choices as a numbered list ("1. label —
	// detail"). Asking, on a task: the id of its question still waiting for
	// an answer. Answers, on a steer: the question it answers. Whatever the
	// sender says while a question waits is its answer.
	Ask     bool   `json:"ask,omitempty"`
	Asking  string `json:"asking,omitempty"`
	Answers string `json:"answers,omitempty"`
	// Suggest, on a task: what its recipient thinks the sender may want to
	// say next (POST /v1/messages/{id}/suggest, v0.19.15+); the app offers
	// it in the input and Tab takes it.
	Suggest string `json:"suggest,omitempty"`
	// Automation, on a task an automation sent, is its id (v0.19.22+).
	Automation string `json:"automation,omitempty"`
	// Workflow, on a task a workflow sent, is its run's id; Result, on a
	// reply, is the structured result handed back with it (v0.21+).
	Workflow string          `json:"workflow,omitempty"`
	Result   json.RawMessage `json:"result,omitempty"`
	// GrantedBy, on a task, says why the hub let it past its recipient's
	// allow_from: "project:<name>" (a spawn request from an agent of a
	// project with a terminal on that computer; v0.21.2+).
	GrantedBy string `json:"granted_by,omitempty"`
	// E2E: the body is end-to-end encrypted (an envelope only the
	// message's readers can open).
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
	// Avatar sets the face: "" for the default one drawn from the id,
	// "look:<parts>" for a drawn one chosen in a client, or a picture as a
	// data: URL (PNG, JPEG, WebP or GIF, at most 192 KiB). Hubs before
	// avatars refuse it as an unknown field.
	Avatar *string `json:"avatar,omitempty"`
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

// SuggestRequest is the body of POST /v1/messages/{id}/suggest: what the
// task's sender may want to say next ("" clears it).
type SuggestRequest struct {
	Text string `json:"text"`
}

// ReplyRequest is the body of POST /v1/messages/{id}/reply.
type ReplyRequest struct {
	Body        string   `json:"body"`
	Attachments []string `json:"attachments,omitempty"`
	// Result (replies only, v0.21+): a JSON object, the structured result a
	// workflow step reads. Older hubs refuse the field.
	Result json.RawMessage `json:"result,omitempty"`
	// Ask (progress only): the update asks the task's sender to decide, and
	// the task waits (held) until they answer. Older hubs refuse the field.
	Ask bool `json:"ask,omitempty"`
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
	// Workspace names this workspace on a hub that serves several (its
	// address ends in /w/<name>); empty for the hub's default workspace.
	Workspace string `json:"workspace,omitempty"`
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
	// Superagent: the agent that joins through this link becomes the
	// caller's superagent (a person's link; v0.21.5+, older hubs ignore it).
	Superagent bool `json:"superagent,omitempty"`
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
	Agent      Agent  `json:"agent"`
	State      string `json:"state"` // working | pending | idle | offline
	Queued     int    `json:"queued"`
	InProgress int    `json:"in_progress"`
	Held       int    `json:"held"`
	// Asking counts held tasks waiting on a question to their sender
	// (myco message ask); they are in Held too. Absent on older hubs.
	Asking int `json:"asking,omitempty"`
	// Stale counts tasks it acked that show no life: it is away, or none
	// was worked on lately (5 minutes for a runner, 30 for a session). They
	// are open but not in InProgress, and do not make it "working".
	// StaleTasks are their ids. Absent on older hubs.
	Stale      int       `json:"stale,omitempty"`
	StaleTasks []string  `json:"stale_tasks,omitempty"`
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
	// TakeOver, from the hub only (POST /v1/agents/{name}/handover): a
	// take-over link token. The new session takes that node over instead of
	// joining as a new agent; Brief is its first prompt (the handover note).
	// ResumeSession continues that session (a switch back) instead of
	// starting one. Machines older than the hub's handover support are
	// refused by the hub before it sends this.
	TakeOver      string `json:"take_over,omitempty"`
	Brief         string `json:"brief,omitempty"`
	ResumeSession string `json:"resume_session,omitempty"`
	// JoinLink, instead of starting a session, adds this machine to another
	// workspace of the same hub server with that workspace's join link
	// (<hub>/w/<name>/j/<token>): it reuses the machine's way to the server,
	// so nothing is run by hand there. The machine only follows links to
	// the server it is already connected to through this node.
	JoinLink string `json:"join_link,omitempty"`
	// Version, with CLI UpgradeCLI: the release to install ("" = latest).
	Version string `json:"version,omitempty"`
}

// HubUpgrade is POST /v1/admin/upgrade: the hub brings its own myco to a
// release and restarts (v0.20.29). Via says how: "service" (a root
// service of the server does it, installed by install-hub.sh or `sudo
// myco upgrade`) or "self" (the hub replaces its binary and its service
// manager starts it again). Done in the background: watch /v1/health's
// server_version.
// Backups is GET /v1/admin/backups (from v0.21.12): the backups this hub
// keeps of its own data (its database and files), newest first. A
// workspace of a server keeps its own. Keep is how many it keeps; it makes
// one a day by itself (Daily) when it has none from the last day.
type Backups struct {
	Backups []Backup `json:"backups"`
	Keep    int      `json:"keep"`
	Daily   bool     `json:"daily"`
	// Restore is how to put one back (on the server, with the hub stopped).
	Restore string `json:"restore,omitempty"`
}

// Backup is one backup: download it with GET /v1/admin/backups/{name}.
type Backup struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

type HubUpgrade struct {
	From string `json:"from"`
	To   string `json:"to"`
	Via  string `json:"via"`
}

// UpgradeCLI, as a spawn request's CLI, asks the machine node to bring the
// machine's myco to a release (Version, or the latest) and restart its
// runner (SpawnResult.Upgrade). Machines older than it refuse it as an
// unknown CLI.
const UpgradeCLI = "upgrade"

// MachineUpgrade is a machine's answer to an upgrade request.
type MachineUpgrade struct {
	// From and To are its myco before and after ("" To: it was current).
	From string `json:"from"`
	To   string `json:"to,omitempty"`
	// Restarting: its runner restarts in a few seconds to run To; false
	// when it runs under no service it can restart (Note says so).
	Restarting bool   `json:"restarting"`
	Note       string `json:"note,omitempty"`
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
	// Joined is the workspace URL a join_link request added the machine to,
	// and MachineNode its machine node there.
	Joined      string `json:"joined,omitempty"`
	MachineNode string `json:"machine_node,omitempty"`
	// Info answers a request with CLI InfoCLI: what the machine allows and
	// has (older machines refuse it as an unknown CLI). Reply carries the
	// same as JSON text, which hubs older than Info pass on.
	Info *MachineInfo `json:"info,omitempty"`
	// Upgrade answers a request with CLI UpgradeCLI (also as JSON in Reply).
	Upgrade *MachineUpgrade `json:"upgrade,omitempty"`
}

// InfoCLI, as a spawn request's CLI, asks the machine node about its
// machine instead of starting anything (SpawnResult.Info). It rides on the
// spawn request so that hubs pass it on unchanged.
const InfoCLI = "info"

// MachineInfo is what a machine tells its owner: where sessions may start,
// the agent CLIs on it, and its recent Claude Code and Codex sessions.
type MachineInfo struct {
	// Dirs are the folders sessions may start in, as configured
	// ("~/code/x"); ["~"] when the machine does not limit them.
	Dirs []string `json:"dirs"`
	// NewDir is where a new conversation's folder goes when none is given
	// (a missing folder directly inside it is made): "~/myco" when allowed.
	NewDir string `json:"new_dir"`
	// MaxAccess is the highest access a session there may get.
	MaxAccess string           `json:"max_access"`
	AIs       []MachineAI      `json:"ais"`
	Sessions  []MachineSession `json:"sessions"`
	Home      string           `json:"home,omitempty"`
}

// MachineAI is an agent CLI on the machine.
type MachineAI struct {
	CLI  string `json:"cli"`
	Name string `json:"name"`
	// State: missing, signed_out, ready, or unknown (installed; signed in
	// or not cannot be told).
	State   string `json:"state"`
	Version string `json:"version,omitempty"`
	// Install and Login are commands to run on that machine.
	Install string `json:"install,omitempty"`
	Login   string `json:"login,omitempty"`
	// Models are the models it offers there (Codex, v0.21.1+).
	Models       []RuntimeModel `json:"models,omitempty"`
	MCP          []MCPServer    `json:"mcp,omitempty"`
	Skills       []ToolSkill    `json:"skills,omitempty"`
	Plugins      []ToolPlugin   `json:"plugins,omitempty"`
	ToolWarnings []string       `json:"tool_warnings,omitempty"`
}

// MachineSession is a recent Claude Code or Codex session on the machine;
// a copy of it joins with a spawn request (ForkFrom: its id, Dir: its
// folder).
type MachineSession struct {
	CLI        string    `json:"cli"`
	SessionID  string    `json:"session_id"`
	Workdir    string    `json:"workdir"`
	LastActive time.Time `json:"last_active"`
	Title      string    `json:"title,omitempty"`
	// Joined names the agent it already is in this workspace; JoinedElsewhere
	// says it joined another workspace.
	Joined          string `json:"joined,omitempty"`
	JoinedElsewhere bool   `json:"joined_elsewhere,omitempty"`
	// Auto: a program ran it (claude -p, codex exec, myco), not a person.
	Auto bool `json:"auto,omitempty"`
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
	CoordinatorInvites bool            `json:"coordinator_invites"`
	Name               string          `json:"name"`
	Title              string          `json:"title"`
	Goal               string          `json:"goal,omitempty"`
	Brief              string          `json:"brief,omitempty"`
	Links              []ProjectLink   `json:"links"`
	Visibility         string          `json:"visibility"` // members | team
	Owner              string          `json:"owner,omitempty"`
	OwnerID            string          `json:"owner_id,omitempty"`
	Archived           bool            `json:"archived,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
	Members            []ProjectMember `json:"members"`
	Docs               []ProjectDoc    `json:"docs"`
	// Memory is the project's memory document, with its body (empty when
	// nobody added to it yet), on GET /v1/projects/{name} from v0.21.10;
	// nil from older hubs, which have no project memory.
	Memory *ProjectDoc `json:"memory,omitempty"`
	// Machines are the computers (ids) its coordinators may start agents
	// on (v0.21.2+; absent on older hubs).
	Machines []string `json:"machines"`
	// Groups are the groups its members are sorted into, in display order
	// (from v0.21.34). Hubs that have groups always send the key ([] when
	// there are none); it is absent from older hubs, which is how a client
	// tells whether the hub supports groups. omitzero keeps it absent when
	// a project from an older hub is passed on.
	Groups []string `json:"groups,omitzero"`
	// What the caller may do: edit the content (owner, admins,
	// coordinators), manage it (people: roles, visibility, archive, delete).
	CanEdit   bool `json:"can_edit"`
	CanManage bool `json:"can_manage"`
}

// ProjectMachine is a computer of the workspace, as a project sees it:
// whether its coordinators may start agents there (GET
// /v1/projects/{name}/machines).
type ProjectMachine struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	OS        string `json:"os,omitempty"`
	Online    bool   `json:"online"`
	CanSpawn  bool   `json:"can_spawn"`
	SpawnNode string `json:"spawn_node,omitempty"`
	Allowed   bool   `json:"allowed"`
}

// ProjectMachineList answers GET /v1/projects/{name}/machines.
type ProjectMachineList struct {
	Project  string           `json:"project"`
	Machines []ProjectMachine `json:"machines"`
}

// ProjectLink points at material kept elsewhere.
type ProjectLink struct {
	Title string `json:"title,omitempty"`
	URL   string `json:"url"`
}

// ProjectMember is a node in a project. Agent is its card when the caller
// may see the node (a private node of someone else shows only its name).
type ProjectMember struct {
	Name string `json:"name"`
	Role string `json:"role"` // member | coordinator
	Note string `json:"note,omitempty"`
	// Group is its group in this project ("" or absent: ungrouped).
	Group   string    `json:"group,omitempty"`
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
	// Groups replaces the project's groups (the full list, in order);
	// members of a group left out become ungrouped. MemberGroups moves
	// members (agent → group, "" = ungrouped) into groups of the resulting
	// list. Both are applied together, in one transaction, by those who
	// edit the project (from v0.21.34; older hubs reject them, so send them
	// only when the project has the groups key).
	Groups       *[]string         `json:"groups,omitempty"`
	MemberGroups map[string]string `json:"member_groups,omitempty"`
}

// ProjectMemberRequest is the body of PUT /v1/projects/{name}/members/{agent}.
type ProjectMemberRequest struct {
	Role *string `json:"role,omitempty"`
	Note *string `json:"note,omitempty"`
	// Group sets its group: one of the project's groups, "" takes it out
	// (from v0.21.34; send it only to hubs that have groups).
	Group *string `json:"group,omitempty"`
}

// ProjectDocRequest is the body of PUT /v1/projects/{name}/docs/{doc}.
type ProjectDocRequest struct {
	Body string `json:"body"`
}

// ProjectMemory is the name of the project document that holds the
// project's memory (from v0.21.10).
const ProjectMemory = "memory"

// ProjectMemoryRequest is POST /v1/projects/{name}/memory: add one entry
// to the project's memory (from v0.21.10). The answer is the memory
// document, with its body.
type ProjectMemoryRequest struct {
	Text string `json:"text"`
}

// HiddenList is GET /v1/console/hidden: the messages the caller took out of
// their own view (oldest first). Nothing is deleted.
type HiddenList struct {
	IDs []string `json:"ids"`
}

// HiddenUpdate is POST /v1/console/hidden: messages to hide and to show
// again (500 at most in all); the answer is the HiddenList.
type HiddenUpdate struct {
	Hide []string `json:"hide,omitempty"`
	Show []string `json:"show,omitempty"`
}

// VoiceSettings is GET and PUT /v1/console/voice: what the caller (a
// person) set for voice chats, kept on the hub so every device of theirs
// has it. It holds API keys: the hub hands it only to that person's own
// devices and never logs it. UpdatedAt is when the person changed it, by
// the device they changed it on: the newest wins (a PUT older than what
// the hub holds changes nothing, and the answer is what it holds).
type VoiceSettings struct {
	// The model the voice thinks with: an OpenAI-compatible chat API.
	LLMBaseURL string `json:"llm_base_url,omitempty"`
	LLMModel   string `json:"llm_model,omitempty"`
	LLMKey     string `json:"llm_key,omitempty"`
	// The speech service it speaks with ("doubao", ...), the voice chosen
	// there and how fast it talks (-50..100, 0 is normal).
	TTSProvider string `json:"tts_provider,omitempty"`
	TTSSpeaker  string `json:"tts_speaker,omitempty"`
	TTSRate     int    `json:"tts_rate,omitempty"`
	// The speech services' keys, by service.
	TTSKeys map[string]string `json:"tts_keys,omitempty"`
	// Zero on GET when nothing was ever set.
	UpdatedAt time.Time `json:"updated_at"`
}

// Workspace is one workspace of a hub that serves several: its own members,
// agents, projects and messages, at URL (<hub>/w/<name>). The hub itself is
// the default workspace and is not listed.
type Workspace struct {
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
	// OwnerLink, in the answer to creating it, is a one-time app link that
	// signs in as its owner (works once, for 24 hours).
	OwnerLink *PairingLink `json:"owner_link,omitempty"`
}

// WorkspaceList answers GET /v1/server/workspaces.
type WorkspaceList struct {
	Workspaces []Workspace `json:"workspaces"`
}

// CreateWorkspaceRequest is POST /v1/server/workspaces.
type CreateWorkspaceRequest struct {
	Name   string `json:"name"`
	DryRun bool   `json:"dry_run,omitempty"`
}

// Automations (v0.19.22+): an automation sends a task, as its owner, to one
// agent when its schedule comes round or its webhook is called. The hub
// runs it, so it fires while the owner's computers are off.

// AutomationSchedule is a 5-field cron (minute hour day-of-month month
// day-of-week; numbers, *, a-b, a,b, */n; 0 and 7 are Sunday) in an IANA
// zone ("" = UTC). Runs are at least 10 minutes apart.
type AutomationSchedule struct {
	Cron string `json:"cron"`
	TZ   string `json:"tz,omitempty"`
}

// AutomationRun is one time an automation ran.
type AutomationRun struct {
	ID           string    `json:"id"`
	AutomationID string    `json:"automation_id"`
	At           time.Time `json:"at"`
	// schedule | webhook | manual
	Trigger string `json:"trigger"`
	// sent | skipped | failed
	Outcome   string `json:"outcome"`
	MessageID string `json:"message_id,omitempty"`
	// Why it was skipped or failed (short, English).
	Error string `json:"error,omitempty"`
	// What a webhook call carried (hubs from v0.20.18; webhook runs only).
	Request *WebhookRequest `json:"request,omitempty"`
	// WorkflowRun is the run a workflow automation started (v0.21+).
	WorkflowRun string `json:"workflow_run,omitempty"`
}

// WebhookRequest is what a webhook call carried: its content type, the
// headers safe to show (never credentials, cookies or addresses) and its
// body. A list of runs leaves the body out (size says how big it is);
// GET /v1/console/automations/{id}/runs/{run} has it.
type WebhookRequest struct {
	ContentType string            `json:"content_type,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Body        string            `json:"body,omitempty"`
	Size        int               `json:"size"`
}

// Automation is GET /v1/console/automations' item.
type Automation struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Owner string `json:"owner"`
	// From is the address its tasks are sent from ("@console:<name>").
	From    string `json:"from"`
	To      string `json:"to"`
	Project string `json:"project,omitempty"`
	// Body is the message; "{{payload}}" is replaced by a webhook's body.
	Body     string              `json:"body"`
	Schedule *AutomationSchedule `json:"schedule,omitempty"`
	Webhook  bool                `json:"webhook"`
	// WebhookURL is where to POST (the secret is not in it).
	WebhookURL string         `json:"webhook_url,omitempty"`
	SkipIfBusy bool           `json:"skip_if_busy"`
	Enabled    bool           `json:"enabled"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	NextRunAt  *time.Time     `json:"next_run_at,omitempty"`
	LastRun    *AutomationRun `json:"last_run,omitempty"`
	// Workflow (v0.21+): the workflow it starts instead of sending a task
	// (its id and name), with the run's inputs (templates over {{payload}}).
	Workflow     string            `json:"workflow,omitempty"`
	WorkflowName string            `json:"workflow_name,omitempty"`
	Inputs       map[string]string `json:"inputs,omitempty"`
}

// AutomationRequest is POST /v1/console/automations (create) and PATCH
// /v1/console/automations/{id} (update: absent fields stay as they are).
type AutomationRequest struct {
	Name          *string             `json:"name,omitempty"`
	To            *string             `json:"to,omitempty"`
	Project       *string             `json:"project,omitempty"`
	Body          *string             `json:"body,omitempty"`
	Schedule      *AutomationSchedule `json:"schedule,omitempty"`
	ClearSchedule bool                `json:"clear_schedule,omitempty"`
	// Webhook turns the webhook on (a new secret) or off.
	Webhook *bool `json:"webhook,omitempty"`
	// RotateSecret gives the webhook a new secret; the old one stops working.
	RotateSecret bool  `json:"rotate_secret,omitempty"`
	SkipIfBusy   *bool `json:"skip_if_busy,omitempty"`
	Enabled      *bool `json:"enabled,omitempty"`
	// Workflow (v0.21+): a workflow's id or name to start instead of
	// sending a task ("" goes back to sending one); Inputs replace the
	// run's inputs.
	Workflow *string           `json:"workflow,omitempty"`
	Inputs   map[string]string `json:"inputs,omitempty"`
}

// AutomationResponse answers create and update. WebhookSecret is set only
// when a secret was just made (shown once).
type AutomationResponse struct {
	Automation    Automation `json:"automation"`
	WebhookSecret string     `json:"webhook_secret,omitempty"`
}

// AutomationList is GET /v1/console/automations.
type AutomationList struct {
	Automations []Automation `json:"automations"`
}

// AutomationDetail is GET /v1/console/automations/{id}: newest runs first.
type AutomationDetail struct {
	Automation Automation      `json:"automation"`
	Runs       []AutomationRun `json:"runs"`
}

// WebhookResponse answers POST /hooks/{id}.
type WebhookResponse struct {
	Run AutomationRun `json:"run"`
}

// Page is a web page published to the hub (from v0.19.24): GET /v1/pages,
// GET /v1/pages/{id}, POST /v1/pages.
type Page struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Project   string `json:"project,omitempty"`
	Publisher string `json:"publisher"`
	// Version is the latest version's number.
	Version int `json:"version"`
	// Entry is the file the latest version opens on.
	Entry string `json:"entry"`
	// URL is the page's address on the hub (<hub>/pages/<id>); the App
	// opens such links in its page panel.
	URL       string        `json:"url"`
	Versions  []PageVersion `json:"versions"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
	// CanManage: the caller may publish versions of it and delete it.
	CanManage bool `json:"can_manage"`
	// Editors is who may change its records and shared state (hubs from
	// v0.22.2): "managers" only those who manage it; empty everyone who sees
	// it. ReadOnly: the caller may not (their writes are refused).
	Editors  string `json:"editors,omitempty"`
	ReadOnly bool   `json:"read_only,omitempty"`
	// Live: a server on its publisher's computer (on Port there), forwarded
	// while `myco page serve` runs; Online says whether it is now.
	Live   bool `json:"live,omitempty"`
	Port   int  `json:"port,omitempty"`
	Online bool `json:"online,omitempty"`
	// Template: the template it was made from (hubs from v0.20.5).
	Template *PageTemplate `json:"template,omitempty"`
}

// PageTemplate says which template a page was made from, and its data's
// structure (the manifest's collections).
type PageTemplate struct {
	ID      string                        `json:"id"`
	Slug    string                        `json:"slug"`
	Name    string                        `json:"name"`
	Version string                        `json:"version"`
	Data    map[string]TemplateCollection `json:"data,omitempty"`
	// Permissions it asked for (send, download).
	Permissions []string `json:"permissions,omitempty"`
	// OS and Net: the computer's capabilities and the hosts it declares
	// (hubs from v0.22.1).
	OS  []string `json:"os,omitempty"`
	Net []string `json:"net,omitempty"`
	// Latest is the app's newest version, when newer than the page's
	// (hubs from v0.20.7): POST /v1/pages/{id}/upgrade takes it.
	Latest string `json:"latest,omitempty"`
}

// PageVersion is one publish of a page.
type PageVersion struct {
	N      int       `json:"n"`
	Entry  string    `json:"entry"`
	Files  int       `json:"files"`
	Size   int64     `json:"size"`
	SHA256 string    `json:"sha256"`
	By     string    `json:"by"`
	At     time.Time `json:"at"`
}

// PageList is GET /v1/pages.
type PageList struct {
	Pages []Page `json:"pages"`
}

// PageState is a page's shared state (GET/PUT /v1/pages/{id}/state, from
// v0.19.24): JSON people (through the page's myco bridge) and agents read
// and write. State is null before anyone wrote; Version counts writes.
type PageState struct {
	State     json.RawMessage `json:"state"`
	Version   int             `json:"version"`
	UpdatedBy string          `json:"updated_by,omitempty"`
	UpdatedAt *time.Time      `json:"updated_at,omitempty"`
}

// PageStateRequest is PUT /v1/pages/{id}/state: the new state, and the
// version it was based on (0 when there was none). A 409 answers with the
// latest in "latest".
type PageStateRequest struct {
	State     json.RawMessage `json:"state"`
	IfVersion int             `json:"if_version"`
}

// PageStateConflict is PUT /v1/pages/{id}/state's 409.
type PageStateConflict struct {
	Error
	Latest PageState `json:"latest"`
}

// Pin is one thing a conversation keeps at hand (置顶), above its messages
// in the App: a web address (link), a file, or one of its messages. Both
// parties of the conversation see the same pins; either adds them.
type Pin struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"` // link, file or message
	Title string `json:"title"`
	// Target is the address (link) or the message's id (message).
	Target string      `json:"target,omitempty"`
	File   *Attachment `json:"file,omitempty"`
	// AddedBy is who pinned it: an agent's name or a console sender.
	AddedBy string    `json:"added_by"`
	At      time.Time `json:"at"`
}

// PinList is GET /v1/pins?with=X (and ?a=Y for a console looking at two
// agents' conversation): the conversation's pins in their order.
type PinList struct {
	A    string `json:"a"`
	With string `json:"with"`
	Pins []Pin  `json:"pins"`
	Max  int    `json:"max"`
}

// PinCreate is POST /v1/pins: pin something to the conversation between A
// (the caller when empty: the agent itself, or the console's sender) and
// With. The same thing pinned again is not added twice.
type PinCreate struct {
	A      string      `json:"a,omitempty"`
	With   string      `json:"with"`
	Kind   string      `json:"kind"`
	Title  string      `json:"title,omitempty"`
	Target string      `json:"target,omitempty"`
	File   *Attachment `json:"file,omitempty"`
}

// PinUpdate is PATCH /v1/pins/{id}: a new title.
type PinUpdate struct {
	A     string `json:"a,omitempty"`
	With  string `json:"with"`
	Title string `json:"title"`
}

// PinOrder is PUT /v1/pins/order: the pins in this order (those not named
// keep theirs, after); the answer is the PinList.
type PinOrder struct {
	A    string   `json:"a,omitempty"`
	With string   `json:"with"`
	IDs  []string `json:"ids"`
}

// PageRecord is one record of a page's data (hubs from v0.20.5): a JSON
// object in a named collection, with a version that counts its writes.
type PageRecord struct {
	Collection string          `json:"collection"`
	ID         string          `json:"id"`
	Data       json.RawMessage `json:"data"`
	Version    int             `json:"version"`
	UpdatedBy  string          `json:"updated_by,omitempty"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

// PageRecords is GET /v1/pages/{id}/data: the records (in their order) and
// the page's latest change, to watch from.
type PageRecords struct {
	Records []PageRecord `json:"records"`
	Seq     int64        `json:"seq"`
}

// PageRecordWrite is PUT /v1/pages/{id}/data/{collection}/{record} and
// POST /v1/pages/{id}/data/{collection} (a new record, its id made up):
// IfVersion is the version read (0: it must be new); leave it out to write
// whatever it is. A conflict answers 409 with the latest in "latest".
type PageRecordWrite struct {
	Data      json.RawMessage `json:"data"`
	IfVersion *int            `json:"if_version,omitempty"`
}

// PageRecordConflict is a 409 on a record write: the error and the record
// as it is now.
type PageRecordConflict struct {
	Error
	Latest PageRecord `json:"latest"`
}

// PageChange is one change of a page's data.
type PageChange struct {
	Seq        int64           `json:"seq"`
	Collection string          `json:"collection"`
	Record     string          `json:"record"`
	Op         string          `json:"op"`
	Data       json.RawMessage `json:"data,omitempty"`
	Version    int             `json:"version"`
	By         string          `json:"by,omitempty"`
	At         time.Time       `json:"at"`
}

// PageChanges is GET /v1/pages/{id}/changes?after=&wait=: what changed
// after a point, oldest first; Seq is where to go on from. Reset: the feed
// no longer goes back that far; list the records again.
type PageChanges struct {
	Changes []PageChange `json:"changes"`
	Seq     int64        `json:"seq"`
	Reset   bool         `json:"reset,omitempty"`
}

// Template is one template in the workspace's library (hubs from v0.20.5).
type TemplateLocale struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

type Template struct {
	Locales     map[string]TemplateLocale `json:"locales,omitempty"`
	ID          string                    `json:"id"`
	Slug        string                    `json:"slug"`
	Name        string                    `json:"name"`
	Description string                    `json:"description,omitempty"`
	Kind        string                    `json:"kind"`
	// Category is its shelf: office, dev, tools, life, creative, game or other
	// (hubs from v0.20.6; empty before, and when the manifest has none).
	Category string `json:"category,omitempty"`
	// Icon is its icon, SVG markup (hubs from v0.20.7): show its shape,
	// e.g. as a CSS mask, never as HTML.
	Icon        string                        `json:"icon,omitempty"`
	Version     string                        `json:"version"`
	Publisher   string                        `json:"publisher"`
	Data        map[string]TemplateCollection `json:"data,omitempty"`
	Permissions []string                      `json:"permissions,omitempty"`
	// OS and Net: the computer's capabilities and the hosts it declares
	// (hubs from v0.22.1).
	OS     []string `json:"os,omitempty"`
	Net    []string `json:"net,omitempty"`
	Export []string `json:"export,omitempty"`
	// Guide is its agent's guide (agent.md): how to fill its data.
	Guide string `json:"guide,omitempty"`
	// Official: one of myco's, in every workspace (hubs from v0.20.5).
	Official bool `json:"official,omitempty"`
	// Shots are its screenshots (hubs from v0.21.5): pictures in its
	// folder's shots/, each with its dark one when there is one; fetch them
	// with GET /v1/templates/{id}/files/{path}. A workspace's own templates
	// list theirs when got one by one, not in the list.
	Shots     []TemplateShot    `json:"shots,omitempty"`
	Versions  []TemplateVersion `json:"versions"`
	CanManage bool              `json:"can_manage"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// TemplateShot is one screenshot of a template: paths in its folder.
type TemplateShot struct {
	Src  string `json:"src"`
	Dark string `json:"dark,omitempty"`
}

// AppSync is what POST /v1/templates/sync found: how many official apps
// the hub offers, which came new or newer, where they come from (built in,
// kept from an earlier fetch, the "apps" release), and a fetch error.
type AppSync struct {
	Apps    int        `json:"apps"`
	Updated []string   `json:"updated,omitempty"`
	Source  string     `json:"source"`
	At      *time.Time `json:"fetched_at,omitempty"`
	Error   string     `json:"error,omitempty"`
}

// TemplateCollection is a collection of a template's data.
type TemplateCollection struct {
	Fields map[string]string `json:"fields"`
	Single bool              `json:"single,omitempty"`
	// Write "managers": only those who manage the page change it (v0.22.2).
	Write string `json:"write,omitempty"`
}

// PageUpdate changes a page's settings (PATCH /v1/pages/{id}; hubs from
// v0.22.2): who may change its records, "everyone" or "managers".
type PageUpdate struct {
	Editors *string `json:"editors,omitempty"`
}

// TemplateVersion is one publish of a template.
type TemplateVersion struct {
	Version string    `json:"version"`
	Files   int       `json:"files"`
	Size    int64     `json:"size"`
	By      string    `json:"by"`
	At      time.Time `json:"at"`
}

// TemplateList is GET /v1/templates.
type TemplateList struct {
	Templates []Template `json:"templates"`
}

// TemplateUse is POST /v1/templates/{id}/use: make a page from it.
type TemplateUse struct {
	Name    string `json:"name,omitempty"`
	Project string `json:"project,omitempty"`
	Version string `json:"version,omitempty"`
	// Sample starts it with the template's sample data.
	Sample bool `json:"sample,omitempty"`
}

// TemplateUsed is what POST /v1/templates/{id}/use answers: the page, and
// the template (its guide says how to fill the page's data).
type TemplateUsed struct {
	Page     Page     `json:"page"`
	Template Template `json:"template"`
}

// Workflows (v0.21+): graphs the hub walks, handing each step to an agent,
// a computer or a person.

// Workflow is a stored workflow. Definition is its JSON (on get and apply).
type Workflow struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Title       string          `json:"title,omitempty"`
	Tags        []string        `json:"tags,omitempty"`
	Group       string          `json:"group,omitempty"`
	Description string          `json:"description,omitempty"`
	Project     string          `json:"project,omitempty"`
	Owner       string          `json:"owner,omitempty"`
	Version     int             `json:"version"`
	Enabled     bool            `json:"enabled"`
	Steps       int             `json:"steps"`
	Active      int             `json:"active"`
	LastRun     *WorkflowRun    `json:"last_run,omitempty"`
	Definition  json.RawMessage `json:"definition,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// WorkflowList answers GET /v1/workflows.
type WorkflowList struct {
	Workflows []Workflow `json:"workflows"`
}

// WorkflowApplyRequest is the body of POST /v1/workflows (create, or a
// new version of the one with the same name) and /v1/workflows/validate.
type WorkflowApplyRequest struct {
	Definition json.RawMessage `json:"definition"`
}

// WorkflowApplyResponse answers POST /v1/workflows.
type WorkflowApplyResponse struct {
	Workflow Workflow `json:"workflow"`
	Created  bool     `json:"created"`
	Changed  bool     `json:"changed"`
	Warnings []string `json:"warnings,omitempty"`
	DryRun   bool     `json:"dry_run,omitempty"`
}

// WorkflowCheck answers POST /v1/workflows/validate.
type WorkflowCheck struct {
	Valid    bool     `json:"valid"`
	Problems []string `json:"problems,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// WorkflowUpdate is the body of PATCH /v1/workflows/{ref}.
type WorkflowUpdate struct {
	Enabled *bool `json:"enabled,omitempty"`
}

// WorkflowStart is the body of POST /v1/workflows/{ref}/runs.
type WorkflowStart struct {
	Inputs map[string]any `json:"inputs,omitempty"`
}

// WorkflowPreview answers a dry-run start: what the first step would get.
type WorkflowPreview struct {
	Inputs map[string]any `json:"inputs"`
	Step   string         `json:"step"`
	Target string         `json:"target"`
	Body   string         `json:"body"`
	DryRun bool           `json:"dry_run"`
}

// WorkflowRun is one run. Steps, Events and Definition come with GET
// /v1/workflow-runs/{id}; Current (the steps open now) always.
type WorkflowRun struct {
	ID         string          `json:"id"`
	WorkflowID string          `json:"workflow_id"`
	Name       string          `json:"name"`
	Version    int             `json:"version"`
	Project    string          `json:"project,omitempty"`
	Status     string          `json:"status"` // running waiting succeeded failed cancelled
	Inputs     map[string]any  `json:"inputs,omitempty"`
	Owner      string          `json:"owner,omitempty"`
	By         string          `json:"by,omitempty"`
	Trigger    string          `json:"trigger,omitempty"`
	Error      string          `json:"error,omitempty"`
	Current    []string        `json:"current,omitempty"`
	Steps      []RunStep       `json:"steps,omitempty"`
	Events     []RunEvent      `json:"events,omitempty"`
	Definition json.RawMessage `json:"definition,omitempty"`
	// DurationMS is from start to end, or to now (v0.21.1+).
	DurationMS int64 `json:"duration_ms"`
	// Metrics, Artifacts and Evaluations come with GET
	// /v1/workflow-runs/{id} (v0.21.1+).
	Metrics     *RunMetrics     `json:"metrics,omitempty"`
	Artifacts   []RunArtifact   `json:"artifacts,omitempty"`
	Evaluations []RunEvaluation `json:"evaluations,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	EndedAt     *time.Time      `json:"ended_at,omitempty"`
}

// Usage is what agents' runs cost: tokens and money, as their CLIs report
// them.
type Usage struct {
	InputTokens  int64   `json:"input_tokens,omitempty"`
	OutputTokens int64   `json:"output_tokens,omitempty"`
	CostUSD      float64 `json:"cost_usd,omitempty"`
}

// RunMetrics are a run's runtime facts (v0.21.1+). Active is the time an
// agent or computer step was at work; waiting, the time a decision waited
// for someone while nothing else was.
type RunMetrics struct {
	DurationMS int64  `json:"duration_ms"`
	ActiveMS   int64  `json:"active_ms"`
	WaitingMS  int64  `json:"waiting_ms"`
	Steps      int    `json:"steps"`
	Done       int    `json:"done"`
	Failed     int    `json:"failed"`
	Attempts   int    `json:"attempts"`
	Retries    int    `json:"retries"`
	Bounces    int    `json:"bounces"`
	Usage      *Usage `json:"usage,omitempty"`
}

// RunArtifact is a file an agent handed back on a step's task (v0.21.1+).
// Download it from /v1/files/{file_id} (or the App's files route).
type RunArtifact struct {
	Step        string    `json:"step"`
	StepRun     string    `json:"step_run"`
	Try         int       `json:"try"`
	Kind        string    `json:"kind"` // reply progress
	Message     string    `json:"message"`
	From        string    `json:"from"`
	At          time.Time `json:"at"`
	FileID      string    `json:"file_id"`
	Name        string    `json:"name"`
	Size        int64     `json:"size"`
	ContentType string    `json:"content_type,omitempty"`
}

// RunEvaluation is a score given to a run or one of its steps (v0.21.1+).
type RunEvaluation struct {
	ID        int64     `json:"id"`
	Run       string    `json:"run"`
	Step      string    `json:"step,omitempty"`
	Name      string    `json:"name"`
	Value     float64   `json:"value"`
	Unit      string    `json:"unit,omitempty"`
	Direction string    `json:"direction,omitempty"` // higher lower
	Note      string    `json:"note,omitempty"`
	By        string    `json:"by,omitempty"`
	At        time.Time `json:"at"`
}

// RunEvaluate is the body of POST /v1/workflow-runs/{id}/evaluations.
type RunEvaluate struct {
	Step      string  `json:"step,omitempty"`
	Name      string  `json:"name"`
	Value     float64 `json:"value"`
	Unit      string  `json:"unit,omitempty"`
	Direction string  `json:"direction,omitempty"`
	Note      string  `json:"note,omitempty"`
}

// StepLogEntry is a line of a step's log.
type StepLogEntry struct {
	At time.Time `json:"at"`
	// Kind: task progress reply failure event stdout stderr trace.
	Kind        string       `json:"kind"`
	Sub         string       `json:"sub,omitempty"`
	Try         int          `json:"try,omitempty"`
	From        string       `json:"from,omitempty"`
	Text        string       `json:"text,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
}

// StepLog answers GET /v1/workflow-runs/{id}/steps/{step}/log (v0.21.1+).
type StepLog struct {
	Run     string         `json:"run"`
	Step    RunStep        `json:"step"`
	Entries []StepLogEntry `json:"entries"`
	Clipped bool           `json:"clipped,omitempty"`
}

// Dist summarizes values: durations in ms, or scores.
type Dist struct {
	N   int     `json:"n"`
	Avg float64 `json:"avg"`
	Min float64 `json:"min"`
	P50 float64 `json:"p50"`
	P90 float64 `json:"p90"`
	P95 float64 `json:"p95"`
	Max float64 `json:"max"`
}

// WorkflowStepStats are one step's figures in WorkflowStats.
type WorkflowStepStats struct {
	Step         string     `json:"step"`
	Title        string     `json:"title,omitempty"`
	Kind         string     `json:"kind"`
	Runs         int        `json:"runs"`
	Done         int        `json:"done"`
	Failed       int        `json:"failed"`
	Retries      int        `json:"retries"`
	Bounces      int        `json:"bounces"`
	Duration     Dist       `json:"duration_ms"`
	Usage        *Usage     `json:"usage,omitempty"`
	LastError    string     `json:"last_error,omitempty"`
	LastErrorRun string     `json:"last_error_run,omitempty"`
	LastErrorAt  *time.Time `json:"last_error_at,omitempty"`
}

// WorkflowRunPoint is a run in WorkflowStats.
type WorkflowRunPoint struct {
	ID         string    `json:"id"`
	Status     string    `json:"status"`
	Version    int       `json:"version"`
	CreatedAt  time.Time `json:"created_at"`
	DurationMS int64     `json:"duration_ms"`
	Error      string    `json:"error,omitempty"`
	FailedStep string    `json:"failed_step,omitempty"`
	CostUSD    float64   `json:"cost_usd,omitempty"`
}

// WorkflowVersionStats are one version's figures in WorkflowStats.
type WorkflowVersionStats struct {
	Version   int  `json:"version"`
	Runs      int  `json:"runs"`
	Succeeded int  `json:"succeeded"`
	Failed    int  `json:"failed"`
	Cancelled int  `json:"cancelled"`
	Duration  Dist `json:"duration_ms"`
}

// WorkflowEvalStats summarize a score over runs (each run's latest).
type WorkflowEvalStats struct {
	Name      string  `json:"name"`
	Unit      string  `json:"unit,omitempty"`
	Direction string  `json:"direction,omitempty"`
	Values    Dist    `json:"values"`
	Last      float64 `json:"last"`
	LastRun   string  `json:"last_run"`
}

// WorkflowStats answers GET /v1/workflows/{ref}/stats (v0.21.1+): counts
// over all runs; distributions over the latest Sample runs.
type WorkflowStats struct {
	Workflow    string                 `json:"workflow"`
	Name        string                 `json:"name"`
	Total       int                    `json:"total"`
	ByStatus    map[string]int         `json:"by_status"`
	SuccessRate float64                `json:"success_rate"`
	Sample      int                    `json:"sample"`
	Version     int                    `json:"version,omitempty"`
	Duration    Dist                   `json:"duration_ms"`
	Active      Dist                   `json:"active_ms"`
	Waiting     Dist                   `json:"waiting_ms"`
	Usage       *Usage                 `json:"usage,omitempty"`
	CostPerRun  float64                `json:"cost_per_run,omitempty"`
	Steps       []WorkflowStepStats    `json:"steps"`
	Versions    []WorkflowVersionStats `json:"versions"`
	Evaluations []WorkflowEvalStats    `json:"evaluations"`
	Recent      []WorkflowRunPoint     `json:"recent"`
	Failures    []WorkflowRunPoint     `json:"failures"`
}

// RunCompareSide is one run in a RunComparison.
type RunCompareSide struct {
	Run     WorkflowRun `json:"run"`
	Metrics RunMetrics  `json:"metrics"`
}

// RunCompareStep is a step's figures in one run of a comparison.
type RunCompareStep struct {
	Status     string `json:"status"`
	Runs       int    `json:"runs"`
	Attempts   int    `json:"attempts"`
	DurationMS int64  `json:"duration_ms"`
	Usage      *Usage `json:"usage,omitempty"`
	Summary    string `json:"summary,omitempty"`
	Error      string `json:"error,omitempty"`
}

// RunCompareStepDelta compares a step in two runs.
type RunCompareStepDelta struct {
	Step      string          `json:"step"`
	Title     string          `json:"title,omitempty"`
	A         *RunCompareStep `json:"a,omitempty"`
	B         *RunCompareStep `json:"b,omitempty"`
	FasterIn  string          `json:"faster_in,omitempty"` // a b
	DeltaMS   int64           `json:"delta_ms"`
	DeltaCost float64         `json:"delta_cost_usd,omitempty"`
}

// RunCompareEval compares a score in two runs.
type RunCompareEval struct {
	Name      string   `json:"name"`
	Unit      string   `json:"unit,omitempty"`
	Direction string   `json:"direction,omitempty"`
	A         *float64 `json:"a,omitempty"`
	B         *float64 `json:"b,omitempty"`
	Delta     float64  `json:"delta"`
	Better    string   `json:"better,omitempty"` // a b
}

// RunComparison answers GET /v1/workflow-runs/compare?a=&b= (v0.21.1+).
type RunComparison struct {
	A           RunCompareSide        `json:"a"`
	B           RunCompareSide        `json:"b"`
	SameVersion bool                  `json:"same_version"`
	Steps       []RunCompareStepDelta `json:"steps"`
	Evaluations []RunCompareEval      `json:"evaluations"`
}

// RunStep is one start of a step in a run.
type RunStep struct {
	ID        string `json:"id"`
	Step      string `json:"step"`
	Title     string `json:"title,omitempty"`
	Kind      string `json:"kind"` // agent computer human
	Attempt   int    `json:"attempt"`
	Status    string `json:"status"` // running waiting done failed cancelled
	Target    string `json:"target,omitempty"`
	MessageID string `json:"message_id,omitempty"`
	Input     string `json:"input,omitempty"`
	// Payload is the step's input as JSON (the run's inputs, the step
	// before's result, an edge's map, or a join's object by step).
	Payload any            `json:"payload,omitempty"`
	Output  map[string]any `json:"output,omitempty"`
	Summary string         `json:"summary,omitempty"`
	Error   string         `json:"error,omitempty"`
	Retries int            `json:"retries,omitempty"`
	Bounced bool           `json:"bounced,omitempty"`
	Options []string       `json:"options,omitempty"`
	// v0.21.1+: send-backs, how many times its task went out, the tasks,
	// how long it took (to now while open) and what its agent's runs cost.
	Bounces    int        `json:"bounces,omitempty"`
	Attempts   int        `json:"attempts,omitempty"`
	Tasks      []string   `json:"tasks,omitempty"`
	DurationMS int64      `json:"duration_ms"`
	Usage      *Usage     `json:"usage,omitempty"`
	StartedAt  time.Time  `json:"started_at"`
	EndedAt    *time.Time `json:"ended_at,omitempty"`
	DueAt      *time.Time `json:"due_at,omitempty"`
}

// RunEvent is a line of a run's log.
type RunEvent struct {
	At   time.Time `json:"at"`
	Kind string    `json:"kind"`
	Step string    `json:"step,omitempty"`
	Text string    `json:"text"`
}

// WorkflowRunList answers GET /v1/workflow-runs.
type WorkflowRunList struct {
	Runs []WorkflowRun `json:"runs"`
}

// WorkflowRetry is the body of POST /v1/workflow-runs/{id}/retry.
type WorkflowRetry struct {
	From string `json:"from,omitempty"`
}

// WorkflowDecide is the body of POST /v1/workflow-runs/{id}/decide.
type WorkflowDecide struct {
	Step   string `json:"step"`
	Choice string `json:"choice"`
	Note   string `json:"note,omitempty"`
}

// WorkflowDecision is a decision waiting for someone.
type WorkflowDecision struct {
	Run  WorkflowRun `json:"run"`
	Step RunStep     `json:"step"`
}

// WorkflowDecisions answers GET /v1/workflow-decisions.
type WorkflowDecisions struct {
	Decisions []WorkflowDecision `json:"decisions"`
}

// Superagents (v0.21.1+): a person's own agent with the App's powers in
// its workspace.

// Superagent answers GET /v1/console/superagent: the caller's superagent
// (none: Agent absent) and how many of its requests wait for them.
type Superagent struct {
	Agent   *Agent `json:"agent,omitempty"`
	Pending int    `json:"pending,omitempty"`
}

// SuperagentSet is the body of PUT /v1/console/superagent.
type SuperagentSet struct {
	Agent string `json:"agent"`
}

// MeSuperagent answers GET /v1/me/superagent: whether the calling agent is
// a superagent, and whose.
type MeSuperagent struct {
	Superagent bool   `json:"superagent"`
	Owner      string `json:"owner,omitempty"`
}

// Approval is a superagent's request held for its owner.
type Approval struct {
	ID     string `json:"id"`
	Agent  string `json:"agent"`
	What   string `json:"what"`
	Reason string `json:"reason,omitempty"`
	Method string `json:"method"`
	Path   string `json:"path"`
	Body   string `json:"body,omitempty"`
	// Kind is "" for a superagent's held request (Method/Path/Body), or a
	// typed request any agent may make ("action_grant": Payload is
	// {"grant_id": …}; approving may narrow it with an
	// ActionGrantNarrow body). v0.21.21.
	Kind       string          `json:"kind,omitempty"`
	Payload    json.RawMessage `json:"payload,omitempty"`
	Status     string          `json:"status"` // pending done denied expired withdrew
	Result     int             `json:"result,omitempty"`
	ResultBody string          `json:"result_body,omitempty"`
	DecidedBy  string          `json:"decided_by,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
	ExpiresAt  time.Time       `json:"expires_at"`
	DecidedAt  *time.Time      `json:"decided_at,omitempty"`
}

// ApprovalList answers GET /v1/console/approvals and /v1/me/approvals.
type ApprovalList struct {
	Approvals []Approval `json:"approvals"`
}

// Remote desktop signaling is optional. Media never travels in these DTOs.
type RemoteDevice = remotedesktop.Device
type RemoteSession = remotedesktop.Session
type RemoteOffer = remotedesktop.Offer
type RemoteDescription = remotedesktop.Description

// Notice is something a person should hear about (GET /v1/console/notices,
// hub v0.21.21+): an agent asking them, a decision waiting, a failure, an
// answer. Announce says whether devices announce it (a banner, a sound, a
// push), decided once by the hub from the person's preferences; Quiet says
// why not ("tier", "muted", "paused", "quiet_hours", "off").
type Notice struct {
	Seq  int64  `json:"seq"`
	ID   string `json:"id"`
	Kind string `json:"kind"`
	// Tier: needs_you, results or activity.
	Tier string `json:"tier"`
	Key  string `json:"key"`
	Ref  string `json:"ref,omitempty"`
	// Target is where it leads: "agent:<name>", "project:<name>",
	// "workflow:<id>?run=<run>", "superagent".
	Target    string     `json:"target,omitempty"`
	Project   string     `json:"project,omitempty"`
	Actor     string     `json:"actor,omitempty"`
	Subject   string     `json:"subject,omitempty"`
	MessageID string     `json:"message_id,omitempty"`
	Announce  bool       `json:"announce"`
	Quiet     string     `json:"quiet,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	ReadAt    *time.Time `json:"read_at,omitempty"`
	// ResolvedAt: it no longer waits for the person.
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
	// Message is the message it is about, when there is one (sealed as
	// stored on an end-to-end encrypted hub).
	Message *Message `json:"message,omitempty"`
}

// NoticeFeed is a page of a person's notices. Read after Last next time;
// Badge counts what still needs them (open needs_you notices).
type NoticeFeed struct {
	Notices []Notice `json:"notices"`
	Last    int64    `json:"last"`
	Badge   int      `json:"badge"`
}

// NoticeReadRequest is the body of POST /v1/console/notices/read: mark as
// seen what leads to Target ("" for all), up to Through (0 for all).
type NoticeReadRequest struct {
	Target  string `json:"target,omitempty"`
	Through int64  `json:"through,omitempty"`
}

// NoticeReadResponse says how many notices changed.
type NoticeReadResponse struct {
	Changed int `json:"changed"`
}

// NoticePrefs is what a person chose about being told
// (GET/PUT /v1/console/notice-prefs). Tiers overrides the defaults
// (needs_you on, results and activity off); Quiet hours are minutes from
// midnight in Zone (IANA); Mutes maps a scope ("agent:<name>",
// "project:<name>", "workflow:<id>", "superagent") to when the mute ends
// (the zero time: never).
type NoticePrefs struct {
	Off         bool                 `json:"off,omitempty"`
	Tiers       map[string]bool      `json:"tiers,omitempty"`
	Quiet       *NoticeQuiet         `json:"quiet,omitempty"`
	PausedUntil *time.Time           `json:"paused_until,omitempty"`
	Mutes       map[string]time.Time `json:"mutes,omitempty"`
	UpdatedAt   *time.Time           `json:"updated_at,omitempty"`
}

// NoticeQuiet is a span of quiet hours.
type NoticeQuiet struct {
	From int    `json:"from"`
	To   int    `json:"to"`
	Zone string `json:"zone,omitempty"`
}
