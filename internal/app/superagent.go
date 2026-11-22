package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chenhg5/myco/internal/domain"
)

// SuperagentService: a person's own agent with the App's powers in its
// workspace. Only its owner names it, and
// approves what it asks to do that is hard to undo.
type SuperagentService struct {
	repo      domain.SuperagentRepository
	approvals domain.ApprovalRepository
	agents    domain.AgentRepository
	members   domain.MemberRepository
	messages  *MessageService
	audit     *AuditService
	ids       IDGenerator
	clock     Clock
	typed     map[string]TypedApproval
}

// TypedApproval carries out a typed approval (one any agent may ask for,
// such as acting on a computer) when its owner decides it. Approve returns
// what to tell the agent; narrow is what the owner chose to allow, when
// less than asked (the handler's own JSON).
type TypedApproval interface {
	Approve(ctx context.Context, a *domain.Approval, by *domain.Member, narrow json.RawMessage) (string, error)
	Deny(ctx context.Context, a *domain.Approval)
}

// SetTyped registers the handler of a typed approval kind.
func (s *SuperagentService) SetTyped(kind string, h TypedApproval) {
	if s.typed == nil {
		s.typed = map[string]TypedApproval{}
	}
	s.typed[kind] = h
}

// AskTyped holds a typed request of any agent for owner's approval (not
// only a superagent's: the kind's handler decides what approving does).
func (s *SuperagentService) AskTyped(ctx context.Context, owner *domain.Member, agent domain.AgentName, kind, what, reason, payload string) (*domain.Approval, error) {
	if _, ok := s.typed[kind]; !ok {
		return nil, domain.Errf(domain.CodeInvalidArgument, "unknown approval kind %q", kind)
	}
	a := &domain.Approval{ID: s.ids.New("apr"), Owner: owner.ID, Agent: agent, What: what, Reason: clipRunes(strings.TrimSpace(reason), 500),
		Method: "", Path: "", Kind: kind, Payload: payload, Status: domain.ApprovalPending, CreatedAt: s.clock.Now()}
	if err := s.approvals.Create(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

// NewSuperagentService wires superagents.
func NewSuperagentService(repo domain.SuperagentRepository, approvals domain.ApprovalRepository, agents domain.AgentRepository,
	members domain.MemberRepository, messages *MessageService, audit *AuditService, ids IDGenerator, clock Clock) *SuperagentService {
	return &SuperagentService{repo: repo, approvals: approvals, agents: agents, members: members, messages: messages, audit: audit, ids: ids, clock: clock}
}

// person is the member an actor is, acting themselves (not through their
// superagent).
func person(a Actor, what string) (*domain.Member, error) {
	if a.Via != nil {
		return nil, domain.Errf(domain.CodePermissionDenied, "a superagent cannot %s; only its owner can", what)
	}
	if a.Member == nil {
		return nil, domain.Errf(domain.CodePermissionDenied, "only a member can %s (sign in as yourself in the app)", what)
	}
	return a.Member, nil
}

// Of returns member's superagent (nil when none).
func (s *SuperagentService) Of(ctx context.Context, member string) (*domain.Agent, error) {
	name, err := s.repo.Of(ctx, member)
	if err != nil || name == "" {
		return nil, err
	}
	a, err := s.agents.Get(ctx, name)
	if domain.IsCode(err, domain.CodeNotFound) {
		return nil, nil
	}
	return a, err
}

// OwnerOf returns whose superagent agent is (nil when it is none's, or its
// owner left).
func (s *SuperagentService) OwnerOf(ctx context.Context, agent domain.AgentName) (*domain.Member, error) {
	id, err := s.repo.OwnerOf(ctx, agent)
	if err != nil || id == "" {
		return nil, err
	}
	m, err := s.members.Get(ctx, id)
	if domain.IsCode(err, domain.CodeNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !m.Active() {
		return nil, nil
	}
	return m, nil
}

// Set names agent the actor's superagent. It must be the actor's own agent
// (one they added), and not a person, a computer or a terminal.
func (s *SuperagentService) Set(ctx context.Context, by Actor, name domain.AgentName) (*domain.Agent, error) {
	m, err := person(by, "name a superagent")
	if err != nil {
		return nil, err
	}
	a, err := s.agents.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if a.Owner != m.ID {
		return nil, domain.Errf(domain.CodePermissionDenied, "%s is not your agent; a superagent must be one you added", name).
			WithSuggestion("add one in the app (新建 agent), then name it")
	}
	if a.Kind() != domain.KindAgent {
		return nil, domain.Errf(domain.CodeInvalidArgument, "%s is a %s; a superagent is an agent (Claude Code, Codex, …)", name, a.Kind())
	}
	if other, err := s.repo.OwnerOf(ctx, name); err == nil && other != "" && other != m.ID {
		return nil, domain.Errf(domain.CodeConflict, "%s is someone else's superagent", name)
	}
	before, _ := s.repo.Of(ctx, m.ID)
	if err := s.repo.Set(ctx, m.ID, name, s.clock.Now()); err != nil {
		return nil, err
	}
	if before != "" && before != name {
		s.withdraw(ctx, before)
	}
	s.audit.Record(ctx, by, "superagent.set", string(name), "")
	return a, nil
}

// SetFor names agent memberID's superagent without a person calling: an
// agent that joined through a superagent link that member made. The agent
// is theirs (the link's owner owns what joins through it).
func (s *SuperagentService) SetFor(ctx context.Context, memberID string, name domain.AgentName) error {
	m, err := s.members.Get(ctx, memberID)
	if err != nil {
		return err
	}
	a, err := s.agents.Get(ctx, name)
	if err != nil {
		return err
	}
	if a.Owner != m.ID || a.Kind() != domain.KindAgent {
		return domain.Errf(domain.CodeInvalidArgument, "%s cannot be %s's superagent", name, m.Name)
	}
	before, _ := s.repo.Of(ctx, m.ID)
	if err := s.repo.Set(ctx, m.ID, name, s.clock.Now()); err != nil {
		return err
	}
	if before != "" && before != name {
		s.withdraw(ctx, before)
	}
	s.audit.Record(ctx, Actor{Member: m}, "superagent.set", string(name), "joined through a superagent link")
	return nil
}

// Clear makes the actor's superagent an ordinary agent again.
func (s *SuperagentService) Clear(ctx context.Context, by Actor) error {
	m, err := person(by, "change the superagent")
	if err != nil {
		return err
	}
	name, err := s.repo.Of(ctx, m.ID)
	if err != nil || name == "" {
		return err
	}
	if err := s.repo.Clear(ctx, m.ID); err != nil {
		return err
	}
	s.withdraw(ctx, name)
	s.audit.Record(ctx, by, "superagent.clear", string(name), "")
	return nil
}

// withdraw drops what an agent that is no longer a superagent still asked.
func (s *SuperagentService) withdraw(ctx context.Context, agent domain.AgentName) {
	list, err := s.approvals.ListByAgent(ctx, agent, 200)
	if err != nil {
		return
	}
	for _, a := range list {
		if a.Status == domain.ApprovalPending && a.Kind == domain.ApprovalKindRequest {
			a.Status, a.DecidedAt = domain.ApprovalWithdrew, s.clock.Now()
			_ = s.approvals.Save(ctx, a)
		}
	}
}

// Ask holds a request of owner's superagent for its owner's approval.
func (s *SuperagentService) Ask(ctx context.Context, owner *domain.Member, agent domain.AgentName, what, reason, method, path, contentType string, body []byte) (*domain.Approval, error) {
	a := &domain.Approval{ID: s.ids.New("apr"), Owner: owner.ID, Agent: agent, What: what, Reason: clipRunes(strings.TrimSpace(reason), 500),
		Method: method, Path: path, Body: body, ContentType: contentType, Status: domain.ApprovalPending, CreatedAt: s.clock.Now()}
	if err := s.approvals.Create(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

// expire marks a pending approval past ApprovalTTL as expired.
func (s *SuperagentService) expire(ctx context.Context, a *domain.Approval) {
	if a.Status == domain.ApprovalPending && s.clock.Now().Sub(a.CreatedAt) > domain.ApprovalTTL {
		a.Status, a.DecidedAt = domain.ApprovalExpired, s.clock.Now()
		_ = s.approvals.Save(ctx, a)
	}
}

// Approvals lists the actor's approvals (pending ones when status is
// "pending"; all when "").
func (s *SuperagentService) Approvals(ctx context.Context, by Actor, status string) ([]*domain.Approval, error) {
	m, err := person(by, "see approvals")
	if err != nil {
		return nil, err
	}
	list, err := s.approvals.List(ctx, m.ID, "", 100)
	if err != nil {
		return nil, err
	}
	out := []*domain.Approval{}
	for _, a := range list {
		s.expire(ctx, a)
		if status == "" || a.Status == status {
			out = append(out, a)
		}
	}
	return out, nil
}

// AgentApprovals lists what an agent asked for.
func (s *SuperagentService) AgentApprovals(ctx context.Context, agent domain.AgentName) ([]*domain.Approval, error) {
	list, err := s.approvals.ListByAgent(ctx, agent, 50)
	if err != nil {
		return nil, err
	}
	for _, a := range list {
		s.expire(ctx, a)
	}
	return list, nil
}

// Replay carries out an approved request as its owner, through its
// superagent; it returns the HTTP status and the start of the answer.
type Replay func(ctx context.Context, a *domain.Approval, owner *domain.Member, agent *domain.Agent) (int, string)

// Decide approves (carrying the request out once, with replay) or denies an
// approval; its superagent hears how it went.
func (s *SuperagentService) Decide(ctx context.Context, by Actor, id string, approve bool, replay Replay) (*domain.Approval, error) {
	return s.DecideWith(ctx, by, id, approve, nil, replay)
}

// DecideWith is Decide with what the owner chose to allow of a typed
// request (narrow; nil for all it asked).
func (s *SuperagentService) DecideWith(ctx context.Context, by Actor, id string, approve bool, narrow json.RawMessage, replay Replay) (*domain.Approval, error) {
	m, err := person(by, "approve its own requests")
	if err != nil {
		return nil, err
	}
	a, err := s.approvals.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if a.Owner != m.ID {
		return nil, domain.Errf(domain.CodeNotFound, "approval %s not found", id)
	}
	s.expire(ctx, a)
	if a.Status != domain.ApprovalPending {
		return nil, domain.Errf(domain.CodeInvalidState, "approval %s is %s", id, a.Status)
	}
	a.DecidedBy, a.DecidedAt = m.Name, s.clock.Now()
	if a.Kind != domain.ApprovalKindRequest {
		return s.decideTyped(ctx, by, m, a, approve, narrow)
	}
	if !approve {
		a.Status = domain.ApprovalDenied
		if err := s.approvals.Save(ctx, a); err != nil {
			return nil, err
		}
		s.audit.Record(ctx, by, "approval.deny", string(a.Agent), a.What)
		s.tell(ctx, m, a, fmt.Sprintf("[myco] %s 没有同意：%s（%s）。不要再用别的办法去做这件事；需要的话先问清楚。", m.Name, a.What, a.ID))
		return a, nil
	}
	agent, err := s.agents.Get(ctx, a.Agent)
	if err != nil {
		return nil, err
	}
	// Taken before it runs: never carried out twice.
	a.Status = domain.ApprovalDone
	if err := s.approvals.Save(ctx, a); err != nil {
		return nil, err
	}
	a.Result, a.ResultBody = replay(ctx, a, m, agent)
	a.ResultBody = clipRunes(a.ResultBody, 4000)
	if err := s.approvals.Save(ctx, a); err != nil {
		return nil, err
	}
	s.audit.Record(ctx, by, "approval.approve", string(a.Agent), fmt.Sprintf("%s → %d", a.What, a.Result))
	how := "做好了"
	if a.Result >= 400 {
		how = fmt.Sprintf("但没做成（%d）：%s", a.Result, clipRunes(a.ResultBody, 400))
	}
	s.tell(ctx, m, a, fmt.Sprintf("[myco] %s 同意了：%s（%s），%s。", m.Name, a.What, a.ID, how))
	return a, nil
}

// decideTyped decides a typed approval through its kind's handler.
func (s *SuperagentService) decideTyped(ctx context.Context, by Actor, m *domain.Member, a *domain.Approval, approve bool, narrow json.RawMessage) (*domain.Approval, error) {
	h, ok := s.typed[a.Kind]
	if !ok {
		return nil, domain.Errf(domain.CodeUnsupported, "this hub cannot decide %q approvals", a.Kind)
	}
	if !approve {
		a.Status = domain.ApprovalDenied
		if err := s.approvals.Save(ctx, a); err != nil {
			return nil, err
		}
		h.Deny(ctx, a)
		s.audit.Record(ctx, by, "approval.deny", string(a.Agent), a.What)
		s.tell(ctx, m, a, fmt.Sprintf("[myco] %s 没有同意：%s（%s）。不要再用别的办法去做这件事；需要的话先问清楚。", m.Name, a.What, a.ID))
		return a, nil
	}
	note, err := h.Approve(ctx, a, m, narrow)
	if err != nil {
		if domain.IsCode(err, domain.CodeInvalidArgument) {
			return nil, err // the owner's choice was wrong: still theirs to decide
		}
		// The request is gone (its session ended, the hub restarted): it
		// cannot be granted any more.
		a.Status, a.ResultBody = domain.ApprovalExpired, clipRunes(err.Error(), 400)
		_ = s.approvals.Save(ctx, a)
		return nil, err
	}
	a.Status, a.Result = domain.ApprovalDone, 200
	if err := s.approvals.Save(ctx, a); err != nil {
		return nil, err
	}
	s.audit.Record(ctx, by, "approval.approve", string(a.Agent), a.What)
	s.tell(ctx, m, a, fmt.Sprintf("[myco] %s 同意了：%s（%s）。%s", m.Name, a.What, a.ID, note))
	return a, nil
}

// tell lets the superagent know how its request went (a note from its
// owner, which wakes it to go on).
func (s *SuperagentService) tell(ctx context.Context, owner *domain.Member, a *domain.Approval, body string) {
	if s.messages == nil {
		return
	}
	_, _ = s.messages.SendFromMember(ctx, owner.Name, owner.ID, SendInput{To: string(a.Agent), Type: string(domain.TypeNote), Body: body})
}
