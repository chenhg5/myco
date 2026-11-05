package app

import (
	"context"
	"time"

	"github.com/chenhg5/myco/internal/domain"
)

// AgentService manages the agent registry and authentication.
type AgentService struct {
	agents       domain.AgentRepository
	tokens       TokenIssuer
	ids          IDGenerator
	clock        Clock
	onlineWindow time.Duration
	wakers       Wakers
}

// NewAgentService wires the agent use cases. wakers validates push delivery
// settings (nil means only "pull" is supported).
func NewAgentService(agents domain.AgentRepository, tokens TokenIssuer, ids IDGenerator, clock Clock, onlineWindow time.Duration, wakers Wakers) *AgentService {
	if wakers == nil {
		wakers = Wakers{}
	}
	return &AgentService{agents: agents, tokens: tokens, ids: ids, clock: clock, onlineWindow: onlineWindow, wakers: wakers}
}

// OnlineWindow is how recently an agent must have been seen to count as online.
func (s *AgentService) OnlineWindow() time.Duration { return s.onlineWindow }

// Now exposes the service clock (used by presenters for online status).
func (s *AgentService) Now() time.Time { return s.clock.Now() }

// RegisterInput describes an agent registration.
type RegisterInput struct {
	Name         string
	Description  string
	Capabilities []string
	AllowFrom    []string
	Binding      domain.Binding
	Delivery     domain.Delivery
	// RotateToken updates an existing agent and issues a new token instead
	// of failing with a conflict.
	RotateToken bool
	DryRun      bool
}

// RegisterOutput is the result of a registration. Token is the plain
// credential and is only returned once (empty on dry-run).
type RegisterOutput struct {
	Agent   *domain.Agent
	Token   string
	Created bool
}

// Register creates an agent, or rotates its token when RotateToken is set.
func (s *AgentService) Register(ctx context.Context, in RegisterInput) (*RegisterOutput, error) {
	name, err := domain.ParseAgentName(in.Name)
	if err != nil {
		return nil, err
	}
	spec, err := buildSpec(in)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()

	existing, err := s.agents.Get(ctx, name)
	switch {
	case err == nil:
		if !in.RotateToken {
			return nil, domain.Errf(domain.CodeConflict, "agent %s already exists", name).
				WithSuggestion("pass --rotate-token to update it and issue a new token")
		}
		if err := existing.Update(spec); err != nil {
			return nil, err
		}
		if err := s.wakers.Validate(existing); err != nil {
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

	a, err := domain.NewAgent(s.ids.New("agt"), name, spec, "", now)
	if err != nil {
		return nil, err
	}
	if err := s.wakers.Validate(a); err != nil {
		return nil, err
	}
	if in.DryRun {
		return &RegisterOutput{Agent: a, Created: true}, nil
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

func buildSpec(in RegisterInput) (domain.AgentSpec, error) {
	caps, err := domain.ParseCapabilities(in.Capabilities)
	if err != nil {
		return domain.AgentSpec{}, err
	}
	allow, err := domain.ParseAgentNames(in.AllowFrom)
	if err != nil {
		return domain.AgentSpec{}, err
	}
	return domain.AgentSpec{Description: in.Description, Capabilities: caps, Binding: in.Binding, Delivery: in.Delivery, AllowFrom: allow}, nil
}

// AgentFilter narrows List results.
type AgentFilter struct {
	Capability string
	OnlineOnly bool
}

// List returns registered agents matching f.
func (s *AgentService) List(ctx context.Context, f AgentFilter) ([]*domain.Agent, error) {
	all, err := s.agents.List(ctx)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	out := all[:0]
	for _, a := range all {
		if f.Capability != "" && !a.HasCapability(f.Capability) {
			continue
		}
		if f.OnlineOnly && !a.IsOnline(now, s.onlineWindow) {
			continue
		}
		out = append(out, a)
	}
	return out, nil
}

// Get returns one agent by name.
func (s *AgentService) Get(ctx context.Context, name string) (*domain.Agent, error) {
	n, err := domain.ParseAgentName(name)
	if err != nil {
		return nil, err
	}
	return s.agents.Get(ctx, n)
}

// Remove deletes an agent. On dry-run it only verifies existence.
func (s *AgentService) Remove(ctx context.Context, name string, dryRun bool) (*domain.Agent, error) {
	a, err := s.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if dryRun {
		return a, nil
	}
	return a, s.agents.Delete(ctx, a.Name)
}

// Bind updates the calling agent's session binding (empty fields unchanged).
// Runners call it when a harness session is created so the hub can always
// find the agent's session again.
func (s *AgentService) Bind(ctx context.Context, a *domain.Agent, b domain.Binding) (*domain.Agent, error) {
	if err := a.Bind(b); err != nil {
		return nil, err
	}
	if err := s.wakers.Validate(a); err != nil {
		return nil, err
	}
	return a, s.agents.Save(ctx, a)
}

// DeliveryModes lists supported delivery modes.
func (s *AgentService) DeliveryModes() []string { return s.wakers.Modes() }

// Authenticate resolves a bearer token to an agent and records activity.
func (s *AgentService) Authenticate(ctx context.Context, token string) (*domain.Agent, error) {
	if token == "" {
		return nil, domain.Errf(domain.CodeUnauthenticated, "missing agent token")
	}
	a, err := s.agents.GetByTokenHash(ctx, s.tokens.Hash(token))
	if domain.IsCode(err, domain.CodeNotFound) {
		return nil, domain.Errf(domain.CodeUnauthenticated, "invalid agent token").
			WithSuggestion("re-register with: myco agent register --name <name> --rotate-token")
	}
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	if err := s.agents.Touch(ctx, a.Name, now); err != nil {
		return nil, err
	}
	a.LastSeenAt = now
	return a, nil
}
