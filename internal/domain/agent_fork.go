package domain

import (
	"context"
	"time"
)

// AgentFork is trusted lifecycle provenance. Mutable agent labels never grant
// fork or retirement authority. IDs bind identities even when names are reused.
type AgentFork struct {
	ID               string      `json:"id"`
	NamePrefix       string      `json:"name_prefix,omitempty"`
	Batch            string      `json:"batch"`
	SourceVisibility Visibility  `json:"source_visibility,omitempty"`
	SourceAllowFrom  []AgentName `json:"source_allow_from,omitempty"`
	SourceID         string      `json:"source_id"`
	ForkOf           string      `json:"fork_of"`
	CLI              string      `json:"cli"`
	SourceSession    string      `json:"source_session"`
	SourceDir        string      `json:"source_dir"`
	NodeID           string      `json:"node_id"`
	Node             string      `json:"node"`
	MachineID        string      `json:"machine_id"`
	Owner            string      `json:"owner_id"`
	Name             string      `json:"name"`
	AgentID          string      `json:"agent_id,omitempty"`
	ForkedBy         string      `json:"forked_by"`
	CreatorID        string      `json:"-"`
	ForTask          string      `json:"for_task,omitempty"`
	Worktree         bool        `json:"worktree"`
	Force            bool        `json:"force,omitempty"`
	Access           string      `json:"access,omitempty"`
	Model            string      `json:"model,omitempty"`
	State            string      `json:"state"` // planned, ready, retiring, retired
	MessageID        string      `json:"message_id,omitempty"`
	Finishing        []string    `json:"finishing,omitempty"`
	RetireAttempt    int         `json:"retire_attempt,omitempty"`
	RetireMessageID  string      `json:"retire_message_id,omitempty"`
	Workdir          string      `json:"workdir,omitempty"`
	Repository       string      `json:"repository,omitempty"`
	Branch           string      `json:"branch,omitempty"`
	Now              bool        `json:"now,omitempty"`
	CreatedAt        time.Time   `json:"created_at"`
	Error            string      `json:"error,omitempty"`
}

type ForkFinalization struct {
	Fork   *AgentFork
	Closed []*Message
	Events []*Message
}

// ForkRepository serializes reservations with claims and retirement decisions.
type ForkRepository interface {
	CreateBatch(context.Context, []*AgentFork, int) ([]*AgentFork, error)
	Get(context.Context, string) (*AgentFork, error)
	List(context.Context) ([]*AgentFork, error)
	Mutate(context.Context, string, func(*AgentFork) error) (*AgentFork, error)
	// Idle is true only when no queued/open task exists and the last exchange
	// is older than cutoff. Presence polling never counts as task activity.
	Idle(context.Context, *AgentFork, time.Time) (bool, error)
	ExpireIdle(context.Context, string, time.Time) (*AgentFork, error)
	BeginRetirement(context.Context, string, bool) (*AgentFork, error)
	Busy(context.Context, *AgentFork) (bool, error)
	Activate(context.Context, string, string, string, string) (*AgentFork, error)
	Finalize(context.Context, string, time.Time, func() string) (*ForkFinalization, error)
}
