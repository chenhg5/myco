// Package api defines the hub's wire contract (JSON DTOs) shared by the HTTP
// server and the Go client. Adding optional fields is non-breaking; renaming
// or removing fields is a breaking change.
package api

import "time"

// Version of the wire contract, exposed at GET /v1/health.
const Version = "v1"

// Agent is the public view of a registered agent.
type Agent struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Description  string     `json:"description"`
	Capabilities []string   `json:"capabilities"`
	Delivery     string     `json:"delivery"`
	AllowFrom    []string   `json:"allow_from"`
	Online       bool       `json:"online"`
	CreatedAt    time.Time  `json:"created_at"`
	LastSeenAt   *time.Time `json:"last_seen_at,omitempty"`
}

// Message is the public view of a message.
type Message struct {
	ID             string     `json:"id"`
	From           string     `json:"from"`
	To             string     `json:"to"`
	ThreadID       string     `json:"thread_id"`
	ReplyTo        string     `json:"reply_to,omitempty"`
	Type           string     `json:"type"`
	Body           string     `json:"body"`
	Status         string     `json:"status"`
	Attempts       int        `json:"attempts"`
	Error          string     `json:"error,omitempty"`
	IdempotencyKey string     `json:"idempotency_key,omitempty"`
	LeaseUntil     *time.Time `json:"lease_until,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// RegisterRequest is the body of POST /v1/agents.
type RegisterRequest struct {
	Name         string   `json:"name"`
	Description  string   `json:"description,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	AllowFrom    []string `json:"allow_from,omitempty"`
	Delivery     string   `json:"delivery,omitempty"`
	RotateToken  bool     `json:"rotate_token,omitempty"`
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
	To             string `json:"to"`
	Type           string `json:"type,omitempty"`
	Body           string `json:"body"`
	ThreadID       string `json:"thread_id,omitempty"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

// SendResponse returns the queued (or previously queued) message.
type SendResponse struct {
	Message   Message `json:"message"`
	Duplicate bool    `json:"duplicate"`
	DryRun    bool    `json:"dry_run,omitempty"`
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
	Body string `json:"body"`
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
