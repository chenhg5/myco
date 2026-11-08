package app

import (
	"context"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/domain"
)

// PairingCodePrefix marks one-time pairing codes (app links that a device
// exchanges for its own console token).
const PairingCodePrefix = "myco_pair_"

// PairingTTL is how long a pairing link works.
const PairingTTL = 24 * time.Hour

// ConsoleTokenPrefix marks console tokens, so they are easy to recognise
// (and to tell apart from agent tokens) in logs and secret scanners.
const ConsoleTokenPrefix = "myco_console_"

// consoleTouchEvery limits last-used writes to one per token per minute.
const consoleTouchEvery = time.Minute

// ConsoleService manages console tokens: per-device admin credentials for
// the console app, revocable one by one, so the hub's admin token never has
// to leave the hub machine.
type ConsoleService struct {
	repo     domain.ConsoleTokenRepository
	pairings domain.ConsolePairingRepository
	tokens   TokenIssuer
	ids      IDGenerator
	clock    Clock
}

// NewConsoleService wires the console token use cases.
func NewConsoleService(repo domain.ConsoleTokenRepository, tokens TokenIssuer, ids IDGenerator, clock Clock) *ConsoleService {
	return &ConsoleService{repo: repo, tokens: tokens, ids: ids, clock: clock}
}

// Create issues a console token for a device. The plain token is returned
// only here.
func (s *ConsoleService) Create(ctx context.Context, name string) (*domain.ConsoleToken, string, error) {
	return s.CreateFor(ctx, "", name)
}

// CreateFor issues a console token belonging to member memberID ("" = the
// hub's owner, assigned when the hub starts).
func (s *ConsoleService) CreateFor(ctx context.Context, memberID, name string) (*domain.ConsoleToken, string, error) {
	name, err := domain.ParseConsoleName(name)
	if err != nil {
		return nil, "", err
	}
	plain, _, err := s.tokens.Issue()
	if err != nil {
		return nil, "", err
	}
	plain = ConsoleTokenPrefix + strings.TrimPrefix(plain, "myco_")
	t := &domain.ConsoleToken{ID: s.ids.New("con"), Name: name, MemberID: memberID, TokenHash: s.tokens.Hash(plain), CreatedAt: s.clock.Now()}
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

// RevokeMember revokes every active console token of a member and says how
// many it revoked.
func (s *ConsoleService) RevokeMember(ctx context.Context, memberID string) (int, error) {
	list, err := s.repo.List(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, t := range list {
		if t.MemberID == memberID && t.Active() {
			t.RevokedAt = s.clock.Now()
			if err := s.repo.Save(ctx, t); err != nil {
				return n, err
			}
			n++
		}
	}
	return n, nil
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

// SetPairings enables one-time pairing codes.
func (s *ConsoleService) SetPairings(repo domain.ConsolePairingRepository) { s.pairings = repo }

// CreatePairing issues a one-time code for memberID, valid for PairingTTL.
// The plain code is returned only here.
func (s *ConsoleService) CreatePairing(ctx context.Context, memberID string) (string, time.Time, error) {
	if s.pairings == nil {
		return "", time.Time{}, domain.Errf(domain.CodeNotFound, "pairing links are not enabled on this hub")
	}
	plain, _, err := s.tokens.Issue()
	if err != nil {
		return "", time.Time{}, err
	}
	plain = PairingCodePrefix + strings.TrimPrefix(plain, "myco_")
	now := s.clock.Now()
	p := &domain.ConsolePairing{ID: s.ids.New("pair"), CodeHash: s.tokens.Hash(plain), MemberID: memberID, CreatedAt: now, ExpiresAt: now.Add(PairingTTL)}
	return plain, p.ExpiresAt, s.pairings.Create(ctx, p)
}

// Pair exchanges a pairing code (once) for a new console token of its
// member, named after the device.
func (s *ConsoleService) Pair(ctx context.Context, code, device string) (*domain.ConsoleToken, string, error) {
	if s.pairings == nil || !strings.HasPrefix(code, PairingCodePrefix) {
		return nil, "", domain.Errf(domain.CodeInvalidArgument, "not a pairing link")
	}
	if strings.TrimSpace(device) == "" {
		device = "device"
	}
	p, err := s.pairings.Use(ctx, s.tokens.Hash(code), s.clock.Now())
	if err != nil {
		return nil, "", err
	}
	return s.CreateFor(ctx, p.MemberID, device)
}

// Devices lists a member's active console tokens (their devices).
func (s *ConsoleService) Devices(ctx context.Context, memberID string) ([]*domain.ConsoleToken, error) {
	list, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	var out []*domain.ConsoleToken
	for _, t := range list {
		if t.MemberID == memberID && t.Active() {
			out = append(out, t)
		}
	}
	return out, nil
}
