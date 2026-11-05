package domain

import (
	"context"
	"strings"
	"time"
	"unicode"
)

// Alias is a re-pointable address: messages sent to the alias go to Target.
// Names stay immutable (they are referenced by messages, allow-lists, file
// grants and profiles); aliases give role-style addresses ("reviewer") that
// can move to another agent without touching anyone's scripts. Aliases and
// agent names share one namespace. Only the hub admin manages aliases, so an
// agent cannot claim a role address to intercept others' messages.
type Alias struct {
	Name      AgentName
	Target    AgentName
	CreatedAt time.Time
	UpdatedAt time.Time
}

// AliasRepository persists aliases.
type AliasRepository interface {
	// Set creates or re-points an alias.
	Set(ctx context.Context, a *Alias) error
	Get(ctx context.Context, name AgentName) (*Alias, error)
	Delete(ctx context.Context, name AgentName) error
	List(ctx context.Context) ([]*Alias, error)
	// DeleteForTarget removes every alias pointing at target.
	DeleteForTarget(ctx context.Context, target AgentName) error
}

const maxDisplayName = 80

// ParseDisplayName validates a human-friendly display name (any script,
// no control characters, at most 80 bytes); "" clears it.
func ParseDisplayName(s string) (string, error) {
	s = strings.TrimSpace(s)
	if len(s) > maxDisplayName {
		return "", Errf(CodeInvalidArgument, "display name longer than %d bytes", maxDisplayName)
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return "", Errf(CodeInvalidArgument, "display name contains control characters")
		}
	}
	return s, nil
}
