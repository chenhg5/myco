// Package runner turns an agent CLI into a myco network participant: it
// pulls the agent's inbox, invokes a harness per message and sends results
// back. It depends only on the Hub and SessionStore interfaces below plus
// harness.Harness, so each piece can be swapped or faked independently.
package runner

import (
	"context"
	"time"

	"github.com/chenhg5/myco/internal/api"
)

// Hub is the subset of the hub API the runner needs.
type Hub interface {
	Pull(ctx context.Context, limit int, wait time.Duration) ([]api.Message, error)
	Ack(ctx context.Context, id string) error
	Complete(ctx context.Context, id string) error
	Reply(ctx context.Context, id, body string, dry bool) (*api.Message, error)
	Fail(ctx context.Context, id, reason string, dry bool) (*api.Message, error)
	GetMessage(ctx context.Context, id string) (*api.Message, error)
}

// SessionStore maps myco threads to harness session IDs so follow-ups in a
// thread resume the same agent conversation.
type SessionStore interface {
	Get(threadID string) string
	Put(threadID, sessionID string) error
}

// PromptBuilder renders the text handed to the harness for a message.
type PromptBuilder interface {
	Build(self string, m api.Message) string
}
