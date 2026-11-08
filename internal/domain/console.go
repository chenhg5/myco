package domain

import (
	"context"
	"strings"
	"time"
)

// ConsoleToken is a revocable admin credential for one device (the Mac
// console app). It can do what the admin token can, except manage console
// tokens; the admin token itself never leaves the hub machine.
type ConsoleToken struct {
	ID         string
	Name       string
	TokenHash  string
	CreatedAt  time.Time
	LastUsedAt time.Time
	RevokedAt  time.Time
}

// Active reports whether the token still works.
func (t *ConsoleToken) Active() bool { return t.RevokedAt.IsZero() }

// ParseConsoleName validates a console token's device name.
func ParseConsoleName(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 64 {
		return "", Errf(CodeInvalidArgument, "console token name must be 1-64 characters").
			WithSuggestion("name it after the device, e.g. --name glenn-mac")
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return "", Errf(CodeInvalidArgument, "console token name has a control character")
		}
	}
	return s, nil
}

// ConsoleTokenRepository persists console tokens.
type ConsoleTokenRepository interface {
	Create(ctx context.Context, t *ConsoleToken) error
	Get(ctx context.Context, id string) (*ConsoleToken, error)
	GetByTokenHash(ctx context.Context, hash string) (*ConsoleToken, error)
	List(ctx context.Context) ([]*ConsoleToken, error)
	Save(ctx context.Context, t *ConsoleToken) error
}
