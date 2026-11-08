package app

import (
	"context"
	"fmt"
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
	aliases     domain.AliasRepository
}

// SetAliases makes enrollment avoid names already used as aliases.
func (s *EnrollmentService) SetAliases(repo domain.AliasRepository) { s.aliases = repo }

// NewEnrollmentService wires the enrollment use cases.
func NewEnrollmentService(enrollments domain.EnrollmentRepository, agents domain.AgentRepository, tokens TokenIssuer, ids IDGenerator, clock Clock) *EnrollmentService {
	return &EnrollmentService{enrollments: enrollments, agents: agents, tokens: tokens, ids: ids, clock: clock}
}

// CreateEnrollmentInput configures a new enrollment (admin).
type CreateEnrollmentInput struct {
	// Kind is domain.EnrollmentPlain (default) or domain.EnrollmentLink.
	Kind         string
	NamePrefix   string
	Harness      string
	AllowFrom    []string
	Capabilities []string
	Labels       []string
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
	labels, err := domain.ParseLabels(in.Labels)
	if err != nil {
		return nil, "", err
	}
	plain, err := s.issueEnrollToken()
	if err != nil {
		return nil, "", err
	}
	if in.Kind == domain.EnrollmentMachine {
		return nil, "", domain.Errf(domain.CodeInvalidArgument, "machine enrollments are created by joining with a link")
	}
	e, err := domain.NewEnrollment(s.ids.New("enr"), s.tokens.Hash(plain), domain.EnrollmentSpec{
		Kind: in.Kind, NamePrefix: in.NamePrefix, Harness: in.Harness, AllowFrom: allow, Capabilities: caps, Labels: labels,
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

// JoinInput is a machine joining through a link.
type JoinInput struct {
	// Machine is the requested machine name (usually the host name).
	Machine string
	DryRun  bool
}

// JoinOutput is the machine's enrollment and, unless dry-run, its token.
type JoinOutput struct {
	Machine *domain.Enrollment
	Link    *domain.Enrollment
	Token   string
	Created bool
}

// PeekLink validates a join link without using it (for serving its page).
func (s *EnrollmentService) PeekLink(ctx context.Context, token string) (*domain.Enrollment, error) {
	e, err := s.authenticate(ctx, token)
	if err != nil {
		return nil, err
	}
	if e.Kind != domain.EnrollmentLink {
		return nil, domain.Errf(domain.CodeInvalidArgument, "not a join link")
	}
	return e, e.Usable(s.clock.Now(), false)
}

// Join gives the calling machine its own enrollment, derived from the link:
// same harness and allow_from restrictions, name prefix "<machine>-" and the
// label machine=<machine>. The same machine joining again with the same link
// gets its enrollment back with a new token, without using up the link.
func (s *EnrollmentService) Join(ctx context.Context, linkToken string, in JoinInput) (*JoinOutput, error) {
	link, err := s.PeekLink(ctx, linkToken)
	if err != nil {
		return nil, err
	}
	machine, err := domain.ParseMachineName(in.Machine)
	if err != nil {
		return nil, err
	}
	all, err := s.enrollments.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, e := range all {
		if e.Kind == domain.EnrollmentMachine && e.ParentID == link.ID && e.Machine == machine && e.RevokedAt.IsZero() {
			if in.DryRun {
				return &JoinOutput{Machine: e, Link: link}, nil
			}
			plain, err := s.issueEnrollToken()
			if err != nil {
				return nil, err
			}
			e.SetToken(s.tokens.Hash(plain))
			return &JoinOutput{Machine: e, Link: link, Token: plain}, s.enrollments.Save(ctx, e)
		}
	}
	if err := link.Usable(s.clock.Now(), true); err != nil {
		return nil, err
	}
	// Another machine (from another link) may already use this name.
	taken := map[string]bool{}
	for _, e := range all {
		if e.Kind == domain.EnrollmentMachine && e.RevokedAt.IsZero() {
			taken[e.Machine] = true
		}
	}
	base := machine
	for i := 2; taken[machine]; i++ {
		machine = fmt.Sprintf("%s-%d", base, i)
	}
	labels, err := domain.MergeLabels(link.Labels, []string{"machine=" + machine}, nil)
	if err != nil {
		return nil, err
	}
	plain, err := s.issueEnrollToken()
	if err != nil {
		return nil, err
	}
	e, err := domain.NewEnrollment(s.ids.New("enr"), s.tokens.Hash(plain), domain.EnrollmentSpec{
		Kind: domain.EnrollmentMachine, ParentID: link.ID, Machine: machine, NamePrefix: machine + "-",
		Harness: link.Harness, AllowFrom: link.AllowFrom, Capabilities: link.Capabilities, Labels: labels,
		Note: "machine " + machine + " (joined via " + link.ID + ")",
	}, s.clock.Now())
	if err != nil {
		return nil, err
	}
	if in.DryRun {
		return &JoinOutput{Machine: e, Link: link, Created: true}, nil
	}
	if err := s.enrollments.ConsumeUse(ctx, link.ID); err != nil {
		return nil, err
	}
	if err := s.enrollments.Create(ctx, e); err != nil {
		return nil, err
	}
	return &JoinOutput{Machine: e, Link: link, Token: plain, Created: true}, nil
}

func (s *EnrollmentService) issueEnrollToken() (string, error) {
	plain, _, err := s.tokens.Issue()
	if err != nil {
		return "", err
	}
	return EnrollTokenPrefix + strings.TrimPrefix(plain, "myco_"), nil
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
	DisplayName  string
	Description  string
	Capabilities []string
	// Labels are added to the enrollment's default labels.
	Labels  []string
	Binding domain.Binding
	DryRun  bool
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
				WithSuggestion("use the enrollment token that created it (%s), or have the hub admin run `myco agent register --name %s --rotate-token --no-save` and then `myco agent login --token-file <file>` here", existing.EnrolledBy, existing.Name)
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
	if in.Binding.Harness == domain.HarnessMachine {
		want := domain.MachineNodeName(e.NamePrefix)
		if in.Name != "" && in.Name != want {
			return nil, domain.Errf(domain.CodeInvalidArgument, "a machine node is always named %s", want)
		}
		in.Name = want
	}
	name, err := e.NameFor(in.Name, in.Binding.SessionID)
	if err != nil {
		return nil, err
	}
	taken := func(n domain.AgentName) bool {
		_, err := s.agents.Get(ctx, n)
		return err == nil || (aliasResolver{agents: s.agents, aliases: s.aliases}).aliasTaken(ctx, n)
	}
	if taken(name) {
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
	labels, err := domain.MergeLabels(e.Labels, in.Labels, nil)
	if err != nil {
		return nil, err
	}
	a, err := domain.NewAgent(s.ids.New("agt"), name, domain.AgentSpec{
		DisplayName: in.DisplayName, Description: in.Description, Capabilities: caps, Labels: labels, Binding: in.Binding,
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
	switch {
	case token == "":
		return nil, domain.Errf(domain.CodeUnauthenticated, "no enrollment token was sent").
			WithSuggestion("export MYCO_ENROLL_TOKEN (a plain `VAR=value` line in a sourced file is not exported to child processes), or pass --enroll-token-file")
	case !strings.HasPrefix(token, EnrollTokenPrefix):
		hint := "this looks like an agent token, not an enrollment token"
		if strings.HasPrefix(token, "enr_") {
			hint = "this is an enrollment ID, not its token"
		}
		return nil, domain.Errf(domain.CodeUnauthenticated, "not an enrollment token (expected prefix %s; %s)", EnrollTokenPrefix, hint).
			WithSuggestion("use the `token: myco_enroll_...` line printed by: myco enrollment create")
	}
	e, err := s.enrollments.GetByTokenHash(ctx, s.tokens.Hash(token))
	if domain.IsCode(err, domain.CodeNotFound) {
		return nil, domain.Errf(domain.CodeUnauthenticated, "enrollment token not issued by this hub").
			WithSuggestion("check MYCO_HUB points at the hub that created it; list valid ones with: myco enrollment list")
	}
	return e, err
}
