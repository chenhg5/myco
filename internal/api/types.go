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
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	DisplayName  string     `json:"display_name,omitempty"`
	Aliases      []string   `json:"aliases,omitempty"`
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

// ProfileRequest changes cosmetic fields; omitted fields are unchanged and
// an empty display_name clears it.
type ProfileRequest struct {
	DisplayName *string `json:"display_name,omitempty"`
	Description *string `json:"description,omitempty"`
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
}

// CreateEnrollmentRequest is the body of POST /v1/enrollments.
type CreateEnrollmentRequest struct {
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
	GeneratedAt         time.Time      `json:"generated_at"`
	Viewer              Viewer         `json:"viewer"`
	OnlineWindowSeconds int            `json:"online_window_seconds"`
	Totals              OverviewTotals `json:"totals"`
	Agents              []AgentStatus  `json:"agents"`
	Recent              []Message      `json:"recent"`
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
