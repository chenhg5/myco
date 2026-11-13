package domain

import (
	"context"
	"time"
)

// ResumeCredential is metadata for an agent's reusable recovery credential.
// Its plaintext is never persisted.
type ResumeCredential struct {
	Agent       AgentName
	TokenHash   string
	CreatedAt   time.Time
	LastUsedAt  time.Time
	LastUsedEnv string
}

type ResumeRepository interface {
	Get(ctx context.Context, agent AgentName) (*ResumeCredential, error)
	GetByTokenHash(ctx context.Context, hash string) (*ResumeCredential, error)
	Put(ctx context.Context, credential *ResumeCredential) error
	Delete(ctx context.Context, agent AgentName) error
	// Use atomically replaces the agent's binding and login token, and records
	// use of the recovery credential.
	Use(ctx context.Context, hash string, binding Binding, agentTokenHash string, at time.Time) (AgentName, error)
}
