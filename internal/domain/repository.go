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
	// Mutate loads message id, applies fn and persists the result atomically.
	Mutate(ctx context.Context, id string, fn MutateFunc) (*Message, error)
	// ClaimDue atomically selects up to limit due messages for recipient
	// (oldest first), applies claim to each and persists them. It returns
	// the messages for which claim returned true.
	ClaimDue(ctx context.Context, recipient AgentName, now time.Time, limit int, claim func(*Message) bool) ([]*Message, error)
}
