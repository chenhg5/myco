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
	// PullTypes pulls only messages of the given types (the runner watches
	// for steers while a task runs).
	PullTypes(ctx context.Context, limit int, wait time.Duration, types []string) ([]api.Message, error)
	Ack(ctx context.Context, id string) error
	Complete(ctx context.Context, id string) error
	Reply(ctx context.Context, id, body string, dry bool) (*api.Message, error)
	Fail(ctx context.Context, id, reason string, dry bool) (*api.Message, error)
	Release(ctx context.Context, id string, delay time.Duration, dry bool) (*api.Message, error)
	DownloadFile(ctx context.Context, id, dest string) (*api.File, string, error)
	GetMessage(ctx context.Context, id string) (*api.Message, error)
}

// ProjectReader reads a project for the prompt (a hub client has it; hubs
// before v0.18.25 answer not found, and the prompt goes without it).
type ProjectReader interface {
	GetProject(ctx context.Context, name string, bodies bool) (*api.Project, error)
}

// SessionStore resolves which harness session a message runs in. The key is
// chosen by the runner's SessionScope ("agent" for the agent's single bound
// session, or a thread ID).
type SessionStore interface {
	Get(key string) string
	Put(key, sessionID string) error
}

// SessionScope decides how messages map to harness sessions.
type SessionScope string

const (
	// ScopeAgent: the agent is one session; every message resumes it.
	ScopeAgent SessionScope = "agent"
	// ScopeThread: one session per myco thread.
	ScopeThread SessionScope = "thread"
)

// LocalFile is an attachment downloaded for the agent (Error set if not).
type LocalFile struct {
	Attachment api.Attachment
	Path       string
	Error      string
}

// PromptBuilder renders the text handed to the harness for a message.
type PromptBuilder interface {
	Build(self string, m api.Message, files []LocalFile) string
}
