package domain

import (
	"context"
	"time"
)

// A superagent is a person's own agent with the App's powers in its
// workspace (docs/design/chief-of-staff.md). Its owner names it; the hub
// then lets its agent token through the App's routes, each allowed, held
// for its owner's approval, or refused. One per member in a workspace.

// DefaultSuperagentName is the name a superagent made in the App gets.
const DefaultSuperagentName = "Shroom"

// SuperagentRepository keeps which agent is whose superagent.
type SuperagentRepository interface {
	// Of returns member's superagent ("" when none).
	Of(ctx context.Context, member string) (AgentName, error)
	// OwnerOf returns the member whose superagent agent is ("" when it is
	// none's).
	OwnerOf(ctx context.Context, agent AgentName) (string, error)
	// Set names agent member's superagent (replacing any before);
	// Clear takes it away.
	Set(ctx context.Context, member string, agent AgentName, at time.Time) error
	Clear(ctx context.Context, member string) error
}

// Approval states.
const (
	ApprovalPending  = "pending"
	ApprovalDone     = "done"     // approved and carried out (Result says how it went)
	ApprovalDenied   = "denied"   // its owner said no
	ApprovalExpired  = "expired"  // nobody decided in time
	ApprovalWithdrew = "withdrew" // the agent is no longer a superagent
)

// ApprovalTTL is how long a request waits for its owner.
const ApprovalTTL = 24 * time.Hour

// Approval is a request a superagent made that its owner must approve
// first: kept as it came, carried out once when approved.
type Approval struct {
	ID          string
	Owner       string // member id
	Agent       AgentName
	What        string // what it does, in words ("删除项目 old")
	Reason      string // why, as the agent said
	Method      string
	Path        string // with its query
	Body        []byte
	ContentType string
	Status      string
	Result      int    // the HTTP status it got when carried out
	ResultBody  string // the start of its answer
	DecidedBy   string
	CreatedAt   time.Time
	DecidedAt   time.Time
}

// ApprovalRepository stores approvals.
type ApprovalRepository interface {
	Create(ctx context.Context, a *Approval) error
	Get(ctx context.Context, id string) (*Approval, error)
	Save(ctx context.Context, a *Approval) error
	// List returns an owner's approvals (all states when status is ""),
	// newest first.
	List(ctx context.Context, owner, status string, limit int) ([]*Approval, error)
	// ListByAgent returns those an agent asked for, newest first.
	ListByAgent(ctx context.Context, agent AgentName, limit int) ([]*Approval, error)
}
