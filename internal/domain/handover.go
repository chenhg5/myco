package domain

import (
	"context"
	"time"
)

// CodeReplaced: the credential belonged to a session that another session
// has taken over (a handover). Not retryable: the old session stops.
const CodeReplaced ErrorCode = "replaced"

// HandoverState is where a handover is.
type HandoverState string

const (
	// HandoverWaiting: a link was made; no session has taken it yet.
	HandoverWaiting HandoverState = "waiting"
	// HandoverDone: a session took the node over.
	HandoverDone HandoverState = "done"
	// HandoverExpired: the link expired unused.
	HandoverExpired HandoverState = "expired"
)

// HandoverLinkTTL is how long a take-over link works.
const HandoverLinkTTL = 24 * time.Hour

// HandoverTokenPrefix starts take-over link tokens.
const HandoverTokenPrefix = "myco_takeover_"

// MaxRetiredTokens is how many handed-over credentials an agent keeps for
// telling an old session it was replaced.
const MaxRetiredTokens = 20

// HandoverSide is one session of a handover: where it ran and on what.
type HandoverSide struct {
	Binding   Binding
	MachineID string
	Runtime   Runtime
}

// Handover is one change of the session behind a node: the node's name,
// conversations, projects and aliases stay; the session (binding, token,
// machine) is replaced. Records are only added, never edited after done.
type Handover struct {
	ID    string
	Agent AgentName
	At    time.Time
	// By is who started it: a member's name, "admin", or an agent's name.
	By   string
	Note string
	From HandoverSide
	To   HandoverSide
	// DoneAt is when a session took it (zero while waiting).
	DoneAt time.Time
	State  HandoverState
	// OpenTasks are the tasks that went back to the queue when it was taken.
	OpenTasks []string
	// Back is the handover this one undoes (switching back to the session
	// that handover replaced), "" otherwise.
	Back string
	// ExpiresAt is when its link stops working.
	ExpiresAt time.Time
}

// TakeOverInput is what a session brings to take a node over.
type TakeOverInput struct {
	LinkHash  string
	Binding   Binding
	TokenHash string // the new agent credential's hash
	Now       time.Time
}

// HandoverRepository stores handovers, their links and the credentials they
// retired.
type HandoverRepository interface {
	// Create stores a waiting handover and its one-time link.
	Create(ctx context.Context, h *Handover, linkHash string) error
	Get(ctx context.Context, id string) (*Handover, error)
	// List returns an agent's handovers, newest first.
	List(ctx context.Context, agent AgentName, limit int) ([]*Handover, error)
	// PeekLink returns the waiting handover of an unused, unexpired link.
	PeekLink(ctx context.Context, linkHash string, now time.Time) (*Handover, error)
	// TakeOver, in one transaction: checks the link; refuses a session bound
	// to another agent; retires the agent's current credential; binds the
	// new session with the new credential (machine and runtime cleared, run
	// settings' model and effort cleared when the harness changes); puts the
	// agent's open tasks back in the queue; revokes its resume link; marks
	// the link used and the handover done.
	TakeOver(ctx context.Context, in TakeOverInput) (*Handover, error)
	// Retired returns the handover that retired a credential.
	Retired(ctx context.Context, tokenHash string) (*Handover, error)
}
