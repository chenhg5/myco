package app

import (
	"context"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/domain"
)

// EnrollTokenPrefix marks enrollment tokens so they are never confused with
// agent tokens.
const EnrollTokenPrefix = "myco_enroll_"

// EnrollmentService lets sessions join the network with a limited token.
type EnrollmentService struct {
	enrollments domain.EnrollmentRepository
	agents      domain.AgentRepository
	tokens      TokenIssuer
	ids         IDGenerator
	clock       Clock
}

// NewEnrollmentService wires the enrollment use cases.
func NewEnrollmentService(enrollments domain.EnrollmentRepository, agents domain.AgentRepository, tokens TokenIssuer, ids IDGenerator, clock Clock) *EnrollmentService {
	return &EnrollmentService{enrollments: enrollments, agents: agents, tokens: tokens, ids: ids, clock: clock}
}

// CreateEnrollmentInput configures a new enrollment (admin).
type CreateEnrollmentInput struct {
	NamePrefix   string
	Harness      string
	AllowFrom    []string
	Capabilities []string
	Note         string
	MaxUses      int
	TTL          time.Duration
	DryRun       bool
}

// Create issues a new enrollment and returns its plain token (once).
func (s *EnrollmentService) Create(ctx context.Context, in CreateEnrollmentInput) (*domain.Enrollment, string, error) {
	allow, err := domain.ParseAgentNames(in.AllowFrom)
	if err != nil {
		return nil, "", err
	}
	caps, err := domain.ParseCapabilities(in.Capabilities)
	if err != nil {
		return nil, "", err
	}
	plain, _, err := s.tokens.Issue()
	if err != nil {
		return nil, "", err
	}
	plain = EnrollTokenPrefix + strings.TrimPrefix(plain, "myco_")
	e, err := domain.NewEnrollment(s.ids.New("enr"), s.tokens.Hash(plain), domain.EnrollmentSpec{
		NamePrefix: in.NamePrefix, Harness: in.Harness, AllowFrom: allow, Capabilities: caps,
		Note: in.Note, MaxUses: in.MaxUses, TTL: in.TTL,
	}, s.clock.Now())
	if err != nil {
		return nil, "", err
	}
	if in.DryRun {
		return e, "", nil
	}
	return e, plain, s.enrollments.Create(ctx, e)
}

// List returns all enrollments (admin).
func (s *EnrollmentService) List(ctx context.Context) ([]*domain.Enrollment, error) {
	return s.enrollments.List(ctx)
}

// Revoke disables an enrollment; agents it created keep working (admin).
func (s *EnrollmentService) Revoke(ctx context.Context, id string, dryRun bool) (*domain.Enrollment, error) {
	e, err := s.enrollments.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	e.Revoke(s.clock.Now())
	if dryRun {
		return e, nil
	}
	return e, s.enrollments.Save(ctx, e)
}

// EnrollInput is a session asking to join (or rejoin) the network.
type EnrollInput struct {
	Name         string
	Description  string
	Capabilities []string
	Binding      domain.Binding
	DryRun       bool
}

// Enroll registers the calling session as an agent, or recovers the agent
// already bound to that session (issuing it a fresh token).
func (s *EnrollmentService) Enroll(ctx context.Context, enrollToken string, in EnrollInput) (*RegisterOutput, error) {
	e, err := s.authenticate(ctx, enrollToken)
	if err != nil {
		return nil, err
	}
	if err := e.Admit(in.Binding); err != nil {
		return nil, err
	}
	now := s.clock.Now()

	// Recovery: same session (and harness) → same agent.
	existing, err := s.agents.FindBySession(ctx, in.Binding.Harness, in.Binding.SessionID)
	switch {
	case err == nil:
		if existing.EnrolledBy != e.ID {
			return nil, domain.Errf(domain.CodePermissionDenied,
				"session %s is bound to agent %s, which this enrollment did not create", in.Binding.SessionID, existing.Name).
				WithSuggestion("ask the hub admin to rotate that agent's token with: myco agent register --name %s --rotate-token", existing.Name)
		}
		if err := e.Usable(now, false); err != nil {
			return nil, err
		}
		if err := existing.Bind(in.Binding); err != nil {
			return nil, err
		}
		if in.DryRun {
			return &RegisterOutput{Agent: existing}, nil
		}
		plain, hash, err := s.tokens.Issue()
		if err != nil {
			return nil, err
		}
		existing.RotateToken(hash)
		if err := s.agents.Save(ctx, existing); err != nil {
			return nil, err
		}
		return &RegisterOutput{Agent: existing, Token: plain}, nil
	case !domain.IsCode(err, domain.CodeNotFound):
		return nil, err
	}

	// New agent.
	if err := e.Usable(now, true); err != nil {
		return nil, err
	}
	name, err := e.NameFor(in.Name, in.Binding.SessionID)
	if err != nil {
		return nil, err
	}
	if _, err := s.agents.Get(ctx, name); err == nil {
		if in.Name != "" {
			return nil, domain.Errf(domain.CodeConflict, "agent %s already exists", name).
				WithSuggestion("pick another --name, or omit it to derive one from the session id")
		}
		name, err = domain.ParseAgentName(string(name) + "-" + strings.ToLower(s.ids.New("x")[2:6]))
		if err != nil {
			return nil, err
		}
	}
	caps := in.Capabilities
	if len(caps) == 0 {
		caps = e.Capabilities
	}
	if caps, err = domain.ParseCapabilities(caps); err != nil {
		return nil, err
	}
	a, err := domain.NewAgent(s.ids.New("agt"), name, domain.AgentSpec{
		Description: in.Description, Capabilities: caps, Binding: in.Binding,
		Delivery: domain.Delivery{Mode: domain.DeliveryPull}, AllowFrom: e.AllowFrom,
	}, "", now)
	if err != nil {
		return nil, err
	}
	a.EnrolledBy = e.ID
	if in.DryRun {
		return &RegisterOutput{Agent: a, Created: true}, nil
	}
	if err := s.enrollments.ConsumeUse(ctx, e.ID); err != nil {
		return nil, err
	}
	plain, hash, err := s.tokens.Issue()
	if err != nil {
		return nil, err
	}
	a.RotateToken(hash)
	if err := s.agents.Create(ctx, a); err != nil {
		return nil, err
	}
	return &RegisterOutput{Agent: a, Token: plain, Created: true}, nil
}

func (s *EnrollmentService) authenticate(ctx context.Context, token string) (*domain.Enrollment, error) {
	if !strings.HasPrefix(token, EnrollTokenPrefix) {
		return nil, domain.Errf(domain.CodeUnauthenticated, "missing or invalid enrollment token").
			WithSuggestion("set MYCO_ENROLL_TOKEN to a token from: myco enrollment create")
	}
	e, err := s.enrollments.GetByTokenHash(ctx, s.tokens.Hash(token))
	if domain.IsCode(err, domain.CodeNotFound) {
		return nil, domain.Errf(domain.CodeUnauthenticated, "unknown enrollment token")
	}
	return e, err
}
