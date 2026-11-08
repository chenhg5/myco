// Package app contains myco's use cases. It orchestrates the domain model
// through ports (interfaces) and knows nothing about HTTP or SQL.
package app

import (
	"context"
	"time"
)

// Notifier wakes up waiters (long-polls) when something changes on a topic.
// The in-process implementation lives in infra/notify; a multi-instance hub
// could swap in Redis/NATS without touching use cases.
type Notifier interface {
	Publish(topic string)
	// Subscribe returns a channel closed on the next Publish to topic and a
	// cancel func that must be called when the waiter gives up.
	Subscribe(topic string) (<-chan struct{}, func())
}

// IDGenerator creates unique, prefixed identifiers.
type IDGenerator interface {
	New(prefix string) string
}

// TokenIssuer creates agent credentials and hashes presented tokens.
type TokenIssuer interface {
	Issue() (plain, hash string, err error)
	Hash(plain string) string
}

// Clock abstracts time for deterministic tests.
type Clock interface {
	Now() time.Time
}

// SystemClock is the real clock.
type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

func inboxTopic(agent string) string { return "inbox:" + agent }
func replyTopic(msgID string) string { return "reply:" + msgID }

// wait blocks until ch fires, the timer elapses or ctx is done.
// It returns true only if ch fired.
func wait(ctx context.Context, ch <-chan struct{}, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ch:
		return true
	case <-t.C:
	case <-ctx.Done():
	}
	return false
}

// sleepUntil waits d (if positive) or until ctx is done.
func sleepUntil(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	wait(ctx, nil, d)
	return ctx.Err()
}
