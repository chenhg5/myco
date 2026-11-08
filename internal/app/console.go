package app

import (
	"context"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/domain"
)

// ConsoleTokenPrefix marks console tokens, so they are easy to recognise
// (and to tell apart from agent tokens) in logs and secret scanners.
const ConsoleTokenPrefix = "myco_console_"

// consoleTouchEvery limits last-used writes to one per token per minute.
const consoleTouchEvery = time.Minute

// ConsoleService manages console tokens: per-device admin credentials for
// the console app, revocable one by one, so the hub's admin token never has
// to leave the hub machine.
type ConsoleService struct {
	repo   domain.ConsoleTokenRepository
	tokens TokenIssuer
	ids    IDGenerator
	clock  Clock
}

// NewConsoleService wires the console token use cases.
func NewConsoleService(repo domain.ConsoleTokenRepository, tokens TokenIssuer, ids IDGenerator, clock Clock) *ConsoleService {
	return &ConsoleService{repo: repo, tokens: tokens, ids: ids, clock: clock}
}

// Create issues a console token for a device. The plain token is returned
// only here.
func (s *ConsoleService) Create(ctx context.Context, name string) (*domain.ConsoleToken, string, error) {
	name, err := domain.ParseConsoleName(name)
	if err != nil {
		return nil, "", err
	}
	plain, _, err := s.tokens.Issue()
	if err != nil {
		return nil, "", err
	}
	plain = ConsoleTokenPrefix + strings.TrimPrefix(plain, "myco_")
	t := &domain.ConsoleToken{ID: s.ids.New("con"), Name: name, TokenHash: s.tokens.Hash(plain), CreatedAt: s.clock.Now()}
	if err := s.repo.Create(ctx, t); err != nil {
		return nil, "", err
	}
	return t, plain, nil
}

// List returns all console tokens, oldest first.
func (s *ConsoleService) List(ctx context.Context) ([]*domain.ConsoleToken, error) {
	return s.repo.List(ctx)
}

// Revoke disables a console token. Revoking twice is fine.
func (s *ConsoleService) Revoke(ctx context.Context, id string) (*domain.ConsoleToken, error) {
	t, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if t.Active() {
		t.RevokedAt = s.clock.Now()
		if err := s.repo.Save(ctx, t); err != nil {
			return nil, err
		}
	}
	return t, nil
}

// Authenticate reports whether plain is an active console token.
func (s *ConsoleService) Authenticate(ctx context.Context, plain string) (*domain.ConsoleToken, bool) {
	if !strings.HasPrefix(plain, ConsoleTokenPrefix) {
		return nil, false
	}
	t, err := s.repo.GetByTokenHash(ctx, s.tokens.Hash(plain))
	if err != nil || !t.Active() {
		return nil, false
	}
	if now := s.clock.Now(); now.Sub(t.LastUsedAt) >= consoleTouchEvery {
		t.LastUsedAt = now
		_ = s.repo.Save(ctx, t) // best effort: a failed touch must not deny access
	}
	return t, true
}
