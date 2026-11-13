package app

import (
	"context"
	"sort"
	"strings"
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
	aliases      domain.AliasRepository
	visibility   func(ctx context.Context, owner string) domain.Visibility
	projects     domain.ProjectRepository
	avatars      domain.AvatarStore
}

// SetAvatarStore keeps the pictures agents use as faces (without one, only
// drawn faces can be chosen).
func (s *AgentService) SetAvatarStore(st domain.AvatarStore) { s.avatars = st }

// SetDefaultVisibility decides the visibility of new agents from their
// owner (see DefaultVisibility).
func (s *AgentService) SetDefaultVisibility(f func(ctx context.Context, owner string) domain.Visibility) {
	s.visibility = f
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

// Stale lists agents that have been offline for at least d (never seen:
// since they were created), oldest first. Identities registered by hand
// without a harness are left out (nothing marks them online), and so are
// machine nodes (a machine's runner recreates its node when it comes back).
func (s *AgentService) Stale(ctx context.Context, d time.Duration) ([]*domain.Agent, error) {
	all, err := s.agents.List(ctx)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	var out []*domain.Agent
	for _, a := range all {
		if k := a.Kind(); k == domain.KindHuman || k == domain.KindMachine || a.IsOnline(now, s.onlineWindow) {
			continue
		}
		seen := a.LastSeenAt
		if seen.IsZero() {
			seen = a.CreatedAt
		}
		if now.Sub(seen) >= d {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return lastSeen(out[i]).Before(lastSeen(out[j])) })
	return out, nil
}

func lastSeen(a *domain.Agent) time.Time {
	if a.LastSeenAt.IsZero() {
		return a.CreatedAt
	}
	return a.LastSeenAt
}

// Now exposes the service clock (used by presenters for online status).
func (s *AgentService) Now() time.Time { return s.clock.Now() }

// RegisterInput describes an agent registration.
type RegisterInput struct {
	Name         string
	DisplayName  string
	Description  string
	Capabilities []string
	AllowFrom    []string
	Labels       []string
	Binding      domain.Binding
	Delivery     domain.Delivery
	// RotateToken updates an existing agent and issues a new token instead
	// of failing with a conflict.
	RotateToken bool
	DryRun      bool
	// Owner is the member registering it (new agents only).
	Owner string
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

	if s.resolver().aliasTaken(ctx, name) {
		return nil, domain.Errf(domain.CodeConflict, "%s is already used as an alias", name).
			WithSuggestion("pick another name, or remove the alias: myco agent alias remove --alias %s", name)
	}
	a, err := domain.NewAgent(s.ids.New("agt"), name, spec, "", now)
	if err != nil {
		return nil, err
	}
	a.Owner = in.Owner
	if s.visibility != nil {
		a.Visibility = s.visibility(ctx, a.Owner)
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
	labels, err := domain.ParseLabels(in.Labels)
	if err != nil {
		return domain.AgentSpec{}, err
	}
	return domain.AgentSpec{DisplayName: in.DisplayName, Description: in.Description, Capabilities: caps, Labels: labels, Binding: in.Binding, Delivery: in.Delivery, AllowFrom: allow}, nil
}

// AgentFilter narrows List results.
type AgentFilter struct {
	Capability string
	OnlineOnly bool
	// Selector matches labels (see domain.Selector); "" matches all.
	Selector string
}

// List returns registered agents matching f.
func (s *AgentService) List(ctx context.Context, f AgentFilter) ([]*domain.Agent, error) {
	sel, err := domain.ParseSelector(f.Selector)
	if err != nil {
		return nil, err
	}
	all, err := s.agents.List(ctx)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	out := all[:0]
	for _, a := range all {
		if !sel.Matches(a) {
			continue
		}
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

// Get returns one agent by name or alias.
func (s *AgentService) Get(ctx context.Context, name string) (*domain.Agent, error) {
	n, err := domain.ParseAgentName(name)
	if err != nil {
		return nil, err
	}
	a, _, err := s.resolver().resolve(ctx, n)
	return a, err
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
	// Aliases go first: if the hub stops between the two deletes, the agent
	// is left without its aliases (harmless, and removing it again finishes
	// the job) rather than aliases pointing at a name that a new agent could
	// later take over. The hub also sweeps dangling aliases at start-up.
	if s.aliases != nil {
		if err := s.aliases.DeleteForTarget(ctx, a.Name); err != nil {
			return nil, err
		}
	}
	if s.projects != nil {
		if err := s.projects.DeleteAgent(ctx, a.Name); err != nil {
			return nil, err
		}
	}
	if err := s.agents.Delete(ctx, a.Name); err != nil {
		return nil, err
	}
	return a, nil
}

// SetProjects takes removed agents out of their projects.
func (s *AgentService) SetProjects(repo domain.ProjectRepository) { s.projects = repo }

// Kick signs an agent out: its token stops working at once (a fresh one is
// issued and thrown away), and it counts as offline. Its record, labels,
// aliases and queued messages stay; messages it had leased return to the
// queue when their lease runs out. It comes back by joining again (`myco
// join` in the session, or the runner re-enrolling with the machine's
// credentials), which issues a new token.
func (s *AgentService) Kick(ctx context.Context, name string, dryRun bool) (*domain.Agent, error) {
	a, err := s.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if dryRun {
		return a, nil
	}
	_, hash, err := s.tokens.Issue()
	if err != nil {
		return nil, err
	}
	a.RotateToken(hash)
	if err := s.agents.Save(ctx, a); err != nil {
		return nil, err
	}
	// Offline right away, not after the online window runs out.
	a.LastSeenAt = time.Time{}
	if err := s.agents.Touch(ctx, a.Name, a.LastSeenAt); err != nil {
		return nil, err
	}
	return a, nil
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

// SetLabels adds/removes labels on agent name (callers check who may).
func (s *AgentService) SetLabels(ctx context.Context, name string, add, remove []string) (*domain.Agent, error) {
	a, err := s.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if err := a.SetLabels(add, remove); err != nil {
		return nil, err
	}
	return a, s.agents.Save(ctx, a)
}

// SetSkills replaces an agent's skills (its capabilities follow).
func (s *AgentService) SetSkills(ctx context.Context, name string, skills []domain.Skill) (*domain.Agent, error) {
	a, err := s.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if err := a.SetSkills(skills); err != nil {
		return nil, err
	}
	return a, s.agents.Save(ctx, a)
}

// SetVisibility makes an agent visible to the team or private to its owner.
func (s *AgentService) SetVisibility(ctx context.Context, name string, v string) (*domain.Agent, error) {
	vis, err := domain.ParseVisibility(v)
	if err != nil {
		return nil, err
	}
	a, err := s.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if vis == domain.VisibilityPrivate && a.Owner == "" {
		return nil, domain.Errf(domain.CodeInvalidState, "%s has no owner, so it cannot be private", a.Name).
			WithSuggestion("give it to a member first: myco agent owner set --name %s --owner <member>", a.Name)
	}
	a.Visibility = vis
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

// SetStatus publishes the agent's public status line ("" clears it).
func (s *AgentService) SetStatus(ctx context.Context, name string, text string) (*domain.Agent, error) {
	a, err := s.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if err := a.SetStatus(text, s.clock.Now()); err != nil {
		return nil, err
	}
	return a, s.agents.Save(ctx, a)
}

// UpdateProfile changes cosmetic fields and, for the admin, allow_from
// (nil = unchanged). The agent's token is not touched.
func (s *AgentService) UpdateProfile(ctx context.Context, name string, displayName, description *string, allowFrom *[]string, acceptExecRisk bool, avatar *Avatar) (*domain.Agent, error) {
	a, err := s.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if err := a.SetProfile(displayName, description); err != nil {
		return nil, err
	}
	if avatar != nil {
		if err := s.setAvatar(ctx, a, *avatar); err != nil {
			return nil, err
		}
	}
	if allowFrom != nil {
		names, err := domain.ParseAgentNames(*allowFrom)
		if err != nil {
			return nil, err
		}
		if err := a.SetAllowFrom(names, acceptExecRisk); err != nil {
			return nil, err
		}
	}
	return a, s.agents.Save(ctx, a)
}

// Avatar is a face chosen for an agent: a drawn one (Look, "" for the
// default) or a picture.
type Avatar struct {
	Look    string
	Picture []byte
}

func (s *AgentService) setAvatar(ctx context.Context, a *domain.Agent, av Avatar) error {
	if av.Picture == nil {
		look, err := domain.ParseAvatarLook(av.Look)
		if err != nil {
			return err
		}
		a.Avatar = look
		return nil
	}
	if s.avatars == nil {
		return domain.Errf(domain.CodeInvalidArgument, "this hub does not keep pictures as faces")
	}
	ct, word, err := domain.CheckAvatarPicture(av.Picture)
	if err != nil {
		return err
	}
	if err := s.avatars.PutAvatar(ctx, a.Name, ct, av.Picture); err != nil {
		return err
	}
	a.Avatar = word
	return nil
}

// AvatarPicture returns the picture an agent uses as its face.
func (s *AgentService) AvatarPicture(ctx context.Context, a *domain.Agent) (string, []byte, error) {
	if s.avatars == nil || !strings.HasPrefix(a.Avatar, "pic:") {
		return "", nil, domain.Errf(domain.CodeNotFound, "agent %s has no picture", a.Name)
	}
	return s.avatars.GetAvatar(ctx, a.Name)
}

// SetOwner gives an agent to a member.
func (s *AgentService) SetOwner(ctx context.Context, name, memberID string) (*domain.Agent, error) {
	a, err := s.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	a.Owner = memberID
	return a, s.agents.Save(ctx, a)
}
