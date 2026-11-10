package domain

import (
	"context"
	"time"
)

// AgentRepository persists agents. Implementations return CodeNotFound and
// CodeConflict domain errors where applicable.
type AgentRepository interface {
	Create(ctx context.Context, a *Agent) error
	Save(ctx context.Context, a *Agent) error
	Get(ctx context.Context, name AgentName) (*Agent, error)
	GetByTokenHash(ctx context.Context, hash string) (*Agent, error)
	List(ctx context.Context) ([]*Agent, error)
	Delete(ctx context.Context, name AgentName) error
	Touch(ctx context.Context, name AgentName, at time.Time) error
	// FindBySession returns the agent bound to (harness, sessionID).
	FindBySession(ctx context.Context, harness, sessionID string) (*Agent, error)
	// SetMachine and SetRuntime record what the agent's runner reports.
	SetMachine(ctx context.Context, name AgentName, machineID string) error
	SetRuntime(ctx context.Context, name AgentName, rt Runtime) error
}

// EnrollmentRepository persists enrollments.
type EnrollmentRepository interface {
	Create(ctx context.Context, e *Enrollment) error
	Get(ctx context.Context, id string) (*Enrollment, error)
	GetByTokenHash(ctx context.Context, hash string) (*Enrollment, error)
	List(ctx context.Context) ([]*Enrollment, error)
	Save(ctx context.Context, e *Enrollment) error
	// ConsumeUse atomically increments Uses if a use is still available.
	ConsumeUse(ctx context.Context, id string) error
}

// MutateFunc changes a loaded message and may return new messages that must
// be stored atomically with the change.
type MutateFunc func(m *Message) (created []*Message, err error)

// MessageRepository persists messages. Mutations go through Mutate/ClaimDue so
// that state transitions stay in the domain while atomicity stays in storage.
type MessageRepository interface {
	// Create stores a new message. A duplicate (from, idempotency key)
	// returns CodeConflict.
	Create(ctx context.Context, m *Message) error
	Get(ctx context.Context, id string) (*Message, error)
	FindByIdempotencyKey(ctx context.Context, from AgentName, key string) (*Message, error)
	// FindReply returns the reply or failure event answering messageID.
	FindReply(ctx context.Context, messageID string) (*Message, error)
	ListThread(ctx context.Context, threadID string) ([]*Message, error)
	// ListBroadcast returns the messages of a broadcast, oldest first.
	ListBroadcast(ctx context.Context, broadcastID string) ([]*Message, error)
	// OpenCounts returns, per recipient, how many messages are in each open
	// status (queued, delivered, acked, held).
	OpenCounts(ctx context.Context) (map[AgentName]map[Status]int, error)
	// ListOpen returns open (not queued, not closed) messages of recipient.
	ListOpen(ctx context.Context, recipient AgentName, limit int) ([]*Message, error)
	// ListRecent returns the most recently updated messages; with agent set,
	// only those it sent or received.
	ListRecent(ctx context.Context, agent AgentName, limit int) ([]*Message, error)
	// ListRecentThreads returns threads matching f, most recently updated
	// first (see RecentFilter).
	ListRecentThreads(ctx context.Context, f RecentFilter) ([]ThreadRef, error)
	// ListRecentPairs returns the exchanges with the nodes in f.Nodes, one
	// per (other party, node), most recently updated first.
	ListRecentPairs(ctx context.Context, f RecentFilter) ([]PairRef, error)
	// ListPeers returns the agents agent exchanged messages with (consoles
	// excluded), most recent first, with how many messages and the newest.
	ListPeers(ctx context.Context, agent AgentName, limit int) ([]PeerRef, error)
	// ListBetween returns messages between agents a and b in either
	// direction, newest first; before (unix ms, 0 = now) pages back.
	ListBetween(ctx context.Context, a, b AgentName, before int64, limit int) ([]*Message, error)
	// CreateMany stores several new messages atomically.
	CreateMany(ctx context.Context, ms []*Message) error
	// ListDue returns (without claiming) up to limit messages deliverable to
	// recipient at now, oldest first.
	ListDue(ctx context.Context, recipient AgentName, now time.Time, limit int) ([]*Message, error)
	// DueRecipients lists agents that have deliverable messages at now.
	DueRecipients(ctx context.Context, now time.Time) ([]AgentName, error)
	// Mutate loads message id, applies fn and persists the result atomically.
	Mutate(ctx context.Context, id string, fn MutateFunc) (*Message, error)
	// ClaimDue atomically selects up to limit due messages for recipient
	// (oldest first), applies claim to each and persists them. It returns
	// the messages for which claim returned true.
	// With types set, only messages of those types are considered.
	ClaimDue(ctx context.Context, recipient AgentName, now time.Time, limit int, types []MessageType, claim func(*Message) bool) ([]*Message, error)
}

// ThreadRef is a thread and when it was last updated.
// PeerRef is another agent an agent exchanged messages with.
type PeerRef struct {
	Agent    AgentName
	Messages int
	Last     *Message
}

// RecentFilter narrows ListRecentThreads and ListRecentPairs. For the name
// lists, nil means no condition and an empty, non-nil list matches nothing.
type RecentFilter struct {
	// SenderPrefix keeps threads with a message from or to a name starting
	// with it (e.g. "@console:").
	SenderPrefix string
	// Involve keeps threads and pairs one of these names takes part in (what
	// a viewer may read).
	Involve []AgentName
	// Match keeps threads and pairs one of these names takes part in (a
	// filter the viewer chose).
	Match []AgentName
	// Nodes are the nodes whose exchanges are listed per pair (shell nodes):
	// threads with them are left out of ListRecentThreads.
	Nodes []AgentName
	// Skip are names whose threads are left out of ListRecentThreads
	// (machine nodes).
	Skip []AgentName
	// Query keeps threads and pairs with a message whose body, sender or
	// recipient contains it.
	Query string
	// Project keeps the threads and exchanges of this project.
	Project string
	// AllPairs: ListRecentPairs lists every pair of names that exchanged
	// messages (one conversation per pair), not only those with Nodes.
	AllPairs bool
	// Before (unix ms, 0 = now) pages back; Limit caps the result.
	Before int64
	Limit  int
}

// PairRef is the exchange between an agent (or console) and a node.
type PairRef struct {
	Other     AgentName
	Node      AgentName
	Messages  int
	UpdatedAt time.Time
}

type ThreadRef struct {
	ID        string
	UpdatedAt time.Time
}
