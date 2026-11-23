package api

import (
	"encoding/json"
	"time"
)

// Agent actions (v0.21.21): an agent asks to act on a target (a computer)
// through a provider the target's App runs (the optional Remote Desktop
// component, say). The target's owner approves a grant: what the agent may
// do there, for how long. The agent then runs actions as jobs the target
// carries out; what they make (a screenshot) is a private, short-lived
// artifact. The hub checks grants and relays jobs; it never reads their
// arguments or results. Contract: context/design/app-agent-actions.md in
// the maintainers' repository.

// ActionResource is a private, explicitly enabled resource on a controller
// machine. Its immutable binding freezes the actual target; display metadata
// comes from the trusted host, never from the requesting agent.
type ActionResource struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
	Binding  string `json:"binding"`
	Name     string `json:"name,omitempty"`
	Summary  string `json:"summary,omitempty"`
}

// ActionTarget is where actions run.
type ActionTarget struct {
	Kind     string          `json:"kind"` // "machine" | "page"
	ID       string          `json:"id"`
	Resource *ActionResource `json:"resource,omitempty"`
}

// ActionDescriptor is one action a provider offers.
type ActionDescriptor struct {
	Name         string          `json:"name"` // provider namespace, e.g. remote.desktop.observe
	Version      int             `json:"version"`
	Title        string          `json:"title"`
	Permission   string          `json:"permission"` // e.g. remote.desktop.view
	InputSchema  json.RawMessage `json:"input_schema,omitempty"`
	OutputSchema json.RawMessage `json:"output_schema,omitempty"`
}

// ActionDescriptorList answers GET /v1/me/action-grants/{id}/actions.
type ActionDescriptorList struct {
	Actions []ActionDescriptor `json:"actions"`
}

// ActionGrantRequest is POST /v1/me/action-grants.
type ActionGrantRequest struct {
	Target      ActionTarget `json:"target"`
	Permissions []string     `json:"permissions"`
	Reason      string       `json:"reason"`
	MessageID   string       `json:"message_id,omitempty"` // the task it is for; the grant ends with it
	PageID      string       `json:"page_id,omitempty"`    // context only, not authority
	TTLSeconds  int          `json:"ttl_seconds,omitempty"`
}

// Action grant states.
const (
	GrantPending = "pending"
	GrantActive  = "active"
	GrantDenied  = "denied"
	GrantExpired = "expired"
	GrantRevoked = "revoked"
	GrantEnded   = "ended"
)

// ActionGrant is what an agent may do on a target, approved by its owner.
// Its ID is a reference, not a credential: only the agent that asked can
// use it.
type ActionGrant struct {
	ID          string       `json:"id"`
	Agent       string       `json:"agent"`
	Owner       string       `json:"owner"` // the member who decides (the computer's owner)
	Target      ActionTarget `json:"target"`
	TargetName  string       `json:"target_name,omitempty"`
	Permissions []string     `json:"permissions"`
	Requested   []string     `json:"requested,omitempty"`
	Reason      string       `json:"reason,omitempty"`
	MessageID   string       `json:"message_id,omitempty"`
	ApprovalID  string       `json:"approval_id"`
	State       string       `json:"state"`
	EndReason   string       `json:"end_reason,omitempty"`
	CreatedAt   time.Time    `json:"created_at"`
	ExpiresAt   *time.Time   `json:"expires_at,omitempty"`
	EndedAt     *time.Time   `json:"ended_at,omitempty"`
}

// ActionGrantList answers GET /v1/me/action-grants and
// /v1/console/action-grants.
type ActionGrantList struct {
	Grants []ActionGrant `json:"grants"`
}

// ActionGrantNarrow is the optional body of approving an action_grant
// approval: less than was asked (never more).
type ActionGrantNarrow struct {
	Permissions []string `json:"permissions,omitempty"`
	TTLSeconds  int      `json:"ttl_seconds,omitempty"`
}

// ActionInvocation is POST /v1/me/action-jobs.
type ActionInvocation struct {
	GrantID        string          `json:"grant_id"`
	Action         string          `json:"action"`
	Version        int             `json:"version"`
	Args           json.RawMessage `json:"args,omitempty"`
	IdempotencyKey string          `json:"idempotency_key"`
}

// Action job states.
const (
	JobQueued    = "queued"
	JobRunning   = "running"
	JobSucceeded = "succeeded"
	JobFailed    = "failed"
	JobCancelled = "cancelled"
)

// ActionError is why a job failed. Code "outcome_unknown" means it may or
// may not have happened (observe before going on); Applied is how many of
// an input's events the computer confirmed before it failed.
type ActionError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
	Applied   *int   `json:"applied,omitempty"`
}

// ActionArtifact is a private, short-lived file a job made (a screenshot):
// GET /v1/me/action-artifacts/{id}, by the grant's agent only.
type ActionArtifact struct {
	ID        string    `json:"id"`
	MediaType string    `json:"media_type"`
	Bytes     int       `json:"bytes"`
	ExpiresAt time.Time `json:"expires_at"`
}

// ActionJob is one action run on a grant.
type ActionJob struct {
	ID              string           `json:"id"`
	GrantID         string           `json:"grant_id"`
	Action          string           `json:"action"`
	Version         int              `json:"version"`
	State           string           `json:"state"`
	Result          json.RawMessage  `json:"result,omitempty"`
	Error           *ActionError     `json:"error,omitempty"`
	Artifacts       []ActionArtifact `json:"artifacts,omitempty"`
	CancelRequested bool             `json:"cancel_requested,omitempty"`
	CreatedAt       time.Time        `json:"created_at"`
	FinishedAt      *time.Time       `json:"finished_at,omitempty"`
}

// ActionHostRegister is POST /v1/console/action-hosts: the trusted App on
// a computer offering a provider's actions there.
type ActionHostRegister struct {
	Resource   *ActionResource    `json:"resource,omitempty"`
	MachineID  string             `json:"machine_id"`
	Provider   string             `json:"provider"`
	Generation string             `json:"generation"`
	Actions    []ActionDescriptor `json:"actions"`
}

// ActionHost is a registered host.
type ActionHost struct {
	Resource   *ActionResource `json:"resource,omitempty"`
	ID         string          `json:"id"`
	MachineID  string          `json:"machine_id"`
	Provider   string          `json:"provider"`
	Generation string          `json:"generation"`
	LastSeenAt time.Time       `json:"last_seen_at"`
}

// ActionHostJob is a job handed to a host, with the grant it runs under
// (checked again on the host before anything native happens).
type ActionHostJob struct {
	Job   ActionJob       `json:"job"`
	Args  json.RawMessage `json:"args,omitempty"`
	Grant ActionGrant     `json:"grant"`
}

// ActionHostJobs answers GET /v1/console/action-hosts/{id}/jobs.
type ActionHostJobs struct {
	Jobs []ActionHostJob `json:"jobs"`
}

// ActionHostJobState answers GET /v1/console/action-jobs/{id}: whether the
// host should stop a job it is running (checked again before and while it
// does anything native).
type ActionHostJobState struct {
	Job            ActionJob  `json:"job"`
	GrantState     string     `json:"grant_state"`
	GrantExpiresAt *time.Time `json:"grant_expires_at,omitempty"`
	Stop           bool       `json:"stop"`
}

// ActionJobComplete is POST /v1/console/action-jobs/{id}/complete.
type ActionJobComplete struct {
	HostID      string          `json:"host_id"`
	Generation  string          `json:"generation"`
	State       string          `json:"state"` // succeeded | failed | cancelled
	Result      json.RawMessage `json:"result,omitempty"`
	Error       *ActionError    `json:"error,omitempty"`
	ArtifactIDs []string        `json:"artifact_ids,omitempty"`
}

// ActionTargetList contains enabled private resources of the agent's owner.
type ActionTargetList struct {
	Targets []ActionTarget `json:"targets"`
}
