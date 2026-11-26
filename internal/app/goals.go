package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/myco-ai/myco/internal/domain"
)

type GoalService struct {
	repo     domain.GoalRepository
	agents   domain.AgentRepository
	messages *MessageService
	projects *ProjectService
	audit    *AuditService
	bus      *EventBus
	ids      IDGenerator
	clock    Clock
}

func NewGoalService(repo domain.GoalRepository, agents domain.AgentRepository, messages *MessageService, projects *ProjectService, audit *AuditService, bus *EventBus, ids IDGenerator, clock Clock) *GoalService {
	return &GoalService{repo, agents, messages, projects, audit, bus, ids, clock}
}
func (s *GoalService) Run(ctx context.Context) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		if e := s.Tick(ctx); e != nil && ctx.Err() == nil {
			slog.Warn("goal scheduling failed", "err", e)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
func (s *GoalService) canManage(ctx context.Context, by Actor, r *domain.GoalRecord) bool {
	// The target cannot authorize its own continuation, even when it created a task thread.
	if by.Agent != nil && string(by.Agent.Name) == r.Goal.Agent {
		return false
	}
	if by.Manages() {
		return true
	}
	a, e := s.agents.Get(ctx, domain.AgentName(r.Goal.Agent))
	if e != nil {
		return false
	}
	if by.Owns(a.Owner) {
		return true
	}
	if by.Kind() == r.CreatorKind && goalActorID(by) == r.CreatorID {
		return true
	}
	if by.Agent != nil && r.Goal.Project != "" && s.projects != nil {
		p, e := s.projects.load(ctx, r.Goal.Project)
		return e == nil && p.member(a.Name) != nil && p.coordinates(by.Agent)
	}
	return false
}
func goalActorID(by Actor) string {
	if by.Agent != nil {
		return string(by.Agent.Name)
	}
	if by.Member != nil {
		return by.Member.ID
	}
	return "admin"
}
func (s *GoalService) Get(ctx context.Context, by Actor, id string) (*domain.GoalRecord, error) {
	r, e := s.repo.Get(ctx, id)
	if e != nil {
		return nil, e
	}
	if !s.canManage(ctx, by, r) && (by.Agent == nil || string(by.Agent.Name) != r.Goal.Agent) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, domain.Errf(domain.CodePermissionDenied, "goal is private to its agent, creator, owner, and project coordinator")
	}
	return r, nil
}
func (s *GoalService) List(ctx context.Context, by Actor, agent, project string, all bool) ([]domain.Goal, error) {
	rs, e := s.repo.List(ctx, !all)
	if e != nil {
		return nil, e
	}
	out := []domain.Goal{}
	for _, r := range rs {
		g := r.Goal
		if (agent == "" || g.Agent == agent) && (project == "" || g.Project == project) && (s.canManage(ctx, by, r) || by.Agent != nil && string(by.Agent.Name) == g.Agent) {
			out = append(out, g)
		}
	}
	return out, ctx.Err()
}

type GoalCreate struct {
	Agent, Objective, DoneWhen, Project, Thread string
	EverySec, MaxRounds, ForSec                 int
}
type GoalUpdate struct {
	EverySec  *int `json:"every_sec,omitempty"`
	MaxRounds *int `json:"max_rounds,omitempty"`
	ForSec    *int `json:"for_sec,omitempty"`
}
type GoalReport struct {
	Message    string `json:"message"`
	Summary    string `json:"summary"`
	NextStep   string `json:"next_step,omitempty"`
	Progressed *bool  `json:"progressed"`
	Blocked    bool   `json:"blocked,omitempty"`
	Achieved   bool   `json:"achieved,omitempty"`
	Abandoned  bool   `json:"abandoned,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

func goalText(v string, max int) bool {
	return strings.TrimSpace(v) != "" && utf8.RuneCountInString(v) <= max && !domain.IsSealed(v)
}
func goalLimits(g *domain.Goal, now time.Time, u GoalUpdate) error {
	if u.EverySec != nil {
		n := *u.EverySec
		if n != 900 && n != 1800 && n != 3600 {
			return domain.Errf(domain.CodeInvalidArgument, "every_sec must be 900, 1800, or 3600")
		}
		g.EverySec = n
		g.BaseEverySec = n
	}
	if u.MaxRounds != nil {
		if *u.MaxRounds < 1 || *u.MaxRounds > 256 || *u.MaxRounds < g.Round {
			return domain.Errf(domain.CodeInvalidArgument, "max_rounds must be 1..256 and not below rounds already dispatched")
		}
		g.MaxRounds = *u.MaxRounds
	}
	if u.ForSec != nil {
		n := *u.ForSec
		if n < 1 || n > 28800 || now.Add(time.Duration(n)*time.Second).After(g.StartedAt.Add(8*time.Hour)) {
			return domain.Errf(domain.CodeInvalidArgument, "deadline must be within eight hours of the goal start")
		}
		g.Deadline = now.Add(time.Duration(n) * time.Second)
	}
	return nil
}
func (s *GoalService) Create(ctx context.Context, by Actor, in GoalCreate, dry bool) (*domain.Goal, error) {
	if !goalText(in.Objective, 8000) || !goalText(in.DoneWhen, 4000) {
		return nil, domain.Errf(domain.CodeInvalidArgument, "objective and done_when are required plain text (8000/4000 characters maximum)")
	}
	a, e := s.agents.Get(ctx, domain.AgentName(in.Agent))
	if e != nil {
		return nil, e
	}
	if a.Kind() == domain.KindShell || a.Kind() == domain.KindMachine {
		return nil, domain.Errf(domain.CodeInvalidArgument, "goals require an agent session, not a shell or machine node")
	}
	now := s.clock.Now()
	if in.EverySec == 0 {
		in.EverySec = 1800
	}
	if in.MaxRounds == 0 {
		in.MaxRounds = 16
	}
	if in.ForSec == 0 {
		in.ForSec = 28800
	}
	r := &domain.GoalRecord{Goal: domain.Goal{ID: s.ids.New("gol"), Agent: in.Agent, Project: in.Project, Thread: in.Thread, Objective: in.Objective, DoneWhen: in.DoneWhen, State: "active", StartedAt: now, NextWakeAt: &now, CreatedBy: by.Name()}, Rounds: []domain.GoalRound{}, CreatorKind: "", CreatorID: "", SummaryTo: ConsoleSender(by.Name()), Reports: map[string]string{}}
	if by.Agent != nil {
		r.SummaryTo = by.Agent.Name
	}
	if in.Project != "" {
		p, e := s.projects.load(ctx, in.Project)
		if e != nil {
			return nil, e
		}
		if p.member(a.Name) == nil {
			return nil, domain.Errf(domain.CodePermissionDenied, "target must belong to the project")
		}
	}
	if !s.canManage(ctx, by, r) {
		return nil, domain.Errf(domain.CodePermissionDenied, "only the agent owner or project coordinator may set a goal")
	}
	r.CreatorKind, r.CreatorID = by.Kind(), goalActorID(by)
	if in.Thread != "" {
		m, e := s.messages.messages.Get(ctx, in.Thread)
		if e != nil {
			return nil, e
		}
		if m.Project != in.Project || m.To != a.Name && m.From != a.Name {
			return nil, domain.Errf(domain.CodePermissionDenied, "thread must be a message involving the target in this project")
		}
		if by.Agent != nil && m.From != by.Agent.Name && m.To != by.Agent.Name {
			return nil, domain.Errf(domain.CodePermissionDenied, "creator must participate in the thread")
		}
		if by.Member != nil && !by.Manages() && m.From != r.SummaryTo && m.To != r.SummaryTo {
			return nil, domain.Errf(domain.CodePermissionDenied, "creator must participate in the thread")
		}
		r.Goal.Thread = m.ThreadID
	}
	if r.Goal.Thread == "" {
		r.Goal.Thread = r.Goal.ID
	}
	if e = goalLimits(&r.Goal, now, GoalUpdate{&in.EverySec, &in.MaxRounds, &in.ForSec}); e != nil {
		return nil, e
	}
	if !dry {
		if e = s.repo.Create(ctx, r); e != nil {
			return nil, e
		}
		s.audit.Record(ctx, by, "goal.create", r.Goal.ID, "")
	}
	return &r.Goal, nil
}
func (s *GoalService) message(r *domain.GoalRecord, task bool, body string) *domain.Message {
	now := s.clock.Now()
	to := r.SummaryTo
	from := domain.AgentName(r.Goal.Agent)
	typ := domain.TypeNote
	if task {
		to = domain.AgentName(r.Goal.Agent)
		typ = domain.TypeTask
		from = r.SummaryTo
	}
	return &domain.Message{ID: s.ids.New("msg"), From: from, To: to, ThreadID: r.Goal.Thread, Type: typ, Body: body, Project: r.Goal.Project, Status: domain.StatusQueued, CreatedAt: now, UpdatedAt: now}
}
func (s *GoalService) summary(r *domain.GoalRecord) *domain.Message {
	g := r.Goal
	body := fmt.Sprintf("Goal %s: %s\nObjective: %s\nRounds: %d/%d\nProgress: %s\nNext step: %s\nReason: %s", g.ID, g.State, g.Objective, g.Round, g.MaxRounds, g.LastSummary, g.NextStep, g.Outcome)
	if g.State == "paused" {
		body += "\n请决定下一步怎么做，确认后再恢复目标。 / Please decide how to continue; resume the goal after your decision."
	}
	return s.message(r, false, body)
}
func (s *GoalService) publish(ctx context.Context, ms []*domain.Message) {
	for _, m := range ms {
		s.bus.Publish(ctx, domain.MessageCreated{Message: m})
		s.messages.queued(m.To)
	}
}
func (s *GoalService) Change(ctx context.Context, by Actor, id, action, reason string, u GoalUpdate, dry bool) (*domain.Goal, error) {
	r, e := s.Get(ctx, by, id)
	if e != nil {
		return nil, e
	}
	if !s.canManage(ctx, by, r) {
		return nil, domain.Errf(domain.CodePermissionDenied, "target agent may only report")
	}
	if utf8.RuneCountInString(reason) > 2000 {
		return nil, domain.Errf(domain.CodeInvalidArgument, "reason too long")
	}
	fn := func(r *domain.GoalRecord, _ *domain.Message, _ bool) ([]*domain.Message, error) {
		g := &r.Goal
		now := s.clock.Now()
		if !g.Open() || !now.Before(g.Deadline) {
			return nil, domain.Errf(domain.CodeInvalidState, "goal has ended or expired")
		}
		switch action {
		case "pause":
			if g.State != "active" {
				return nil, domain.Errf(domain.CodeInvalidState, "goal is not active")
			}
			g.State = "paused"
			g.NextWakeAt = nil
			g.Outcome = reason
			return []*domain.Message{s.summary(r)}, nil
		case "resume":
			if g.State != "paused" {
				return nil, domain.Errf(domain.CodeInvalidState, "goal is not paused")
			}
			g.State = "active"
			g.Outcome = ""
			g.NextWakeAt = &now
		case "stop":
			g.End(now, "stopped", by.Name(), reason)
			return []*domain.Message{s.summary(r)}, nil
		case "update":
			if e := goalLimits(g, now, u); e != nil {
				return nil, e
			}
			if u.EverySec != nil && g.NextWakeAt != nil {
				t := now.Add(time.Duration(g.EverySec) * time.Second)
				g.NextWakeAt = &t
			}
		default:
			return nil, domain.Errf(domain.CodeInvalidArgument, "unknown goal action")
		}
		return nil, nil
	}
	if dry {
		_, e = fn(r, nil, false)
		return &r.Goal, e
	}
	r, ms, e := s.repo.Mutate(ctx, id, fn)
	if e != nil {
		return nil, e
	}
	s.publish(ctx, ms)
	s.audit.Record(ctx, by, "goal."+action, id, reason)
	return &r.Goal, nil
}
func (s *GoalService) Report(ctx context.Context, by Actor, id string, in GoalReport, dry bool) (*domain.Goal, error) {
	r, e := s.Get(ctx, by, id)
	if e != nil {
		return nil, e
	}
	if by.Agent == nil || string(by.Agent.Name) != r.Goal.Agent {
		return nil, domain.Errf(domain.CodePermissionDenied, "only the goal agent may report")
	}
	if in.Message == "" || in.Progressed == nil || !goalText(in.Summary, 4000) || utf8.RuneCountInString(in.NextStep) > 2000 || in.Achieved && in.Abandoned || (in.Achieved || in.Abandoned) && !goalText(in.Reason, 2000) || utf8.RuneCountInString(in.Reason) > 2000 {
		return nil, domain.Errf(domain.CodeInvalidArgument, "message, summary and progressed are required; terminal reports need a reason")
	}
	raw, _ := json.Marshal(in)
	fn := func(r *domain.GoalRecord, m *domain.Message, _ bool) ([]*domain.Message, error) {
		if prev, ok := r.Reports[in.Message]; ok {
			if prev == string(raw) {
				return nil, nil
			}
			return nil, domain.Errf(domain.CodeConflict, "this round already has a different report")
		}
		g := &r.Goal
		now := s.clock.Now()
		if !g.Open() || !now.Before(g.Deadline) || len(r.Rounds) == 0 {
			return nil, domain.Errf(domain.CodeInvalidState, "no live goal round")
		}
		rd := &r.Rounds[len(r.Rounds)-1]
		if rd.Message != in.Message || rd.EndedAt != nil || m == nil || m.Status != domain.StatusAcked {
			return nil, domain.Errf(domain.CodeInvalidState, "report must refer to the currently acknowledged round task")
		}
		rd.Summary = in.Summary
		rd.NextStep = in.NextStep
		rd.Progressed = *in.Progressed
		rd.Blocked = in.Blocked
		rd.EndedAt = &now
		if r.Reports == nil {
			r.Reports = map[string]string{}
		}
		r.Reports[in.Message] = string(raw)
		old := g.State
		g.FinishRound(now, rd)
		if in.Achieved {
			g.End(now, "achieved", by.Name(), in.Reason)
		} else if in.Abandoned {
			g.End(now, "abandoned", by.Name(), in.Reason)
		}
		if g.State != old {
			return []*domain.Message{s.summary(r)}, nil
		}
		return nil, nil
	}
	if dry {
		m, _ := s.messages.messages.Get(ctx, in.Message)
		_, e = fn(r, m, false)
		return &r.Goal, e
	}
	r, ms, e := s.repo.Mutate(ctx, id, fn)
	if e != nil {
		return nil, e
	}
	s.publish(ctx, ms)
	s.audit.Record(ctx, by, "goal.report", id, "message="+in.Message)
	return &r.Goal, nil
}
func (s *GoalService) Tick(ctx context.Context) error {
	rs, e := s.repo.List(ctx, true)
	if e != nil {
		return e
	}
	for _, record := range rs {
		unavailable := ""
		if _, e := s.agents.Get(ctx, domain.AgentName(record.Goal.Agent)); e != nil {
			if !domain.IsCode(e, domain.CodeNotFound) {
				return e
			}
			unavailable = "target agent removed"
		}
		if record.Goal.Project != "" {
			p, e := s.projects.load(ctx, record.Goal.Project)
			if e != nil && !domain.IsCode(e, domain.CodeNotFound) {
				return e
			}
			if p == nil || p.member(domain.AgentName(record.Goal.Agent)) == nil {
				unavailable = "target left the project"
			}
		}
		_, ms, e := s.repo.Mutate(ctx, record.Goal.ID, func(r *domain.GoalRecord, m *domain.Message, busy bool) ([]*domain.Message, error) {
			g := &r.Goal
			if !g.Open() {
				return nil, nil
			}
			now := s.clock.Now()
			if unavailable != "" {
				g.End(now, "stopped", "hub", unavailable)
				return []*domain.Message{s.summary(r)}, nil
			}
			if !now.Before(g.Deadline) {
				g.End(now, "expired", "hub", "deadline reached")
				return []*domain.Message{s.summary(r)}, nil
			}
			if len(r.Rounds) > 0 {
				rd := &r.Rounds[len(r.Rounds)-1]
				if m != nil && !m.Status.Terminal() {
					return nil, nil
				}
				if rd.EndedAt == nil {
					rd.EndedAt = &now
					rd.Summary = "没有报告 / No report"
					old := g.State
					g.FinishRound(now, rd)
					if old != g.State {
						return []*domain.Message{s.summary(r)}, nil
					}
				}
				if g.State == "active" && g.NextWakeAt == nil {
					t := now.Add(time.Duration(g.EverySec) * time.Second)
					g.NextWakeAt = &t
				}
			}
			if g.State != "active" {
				return nil, nil
			}
			if g.Round >= g.MaxRounds {
				g.End(now, "stopped", "hub", "round_limit")
				return []*domain.Message{s.summary(r)}, nil
			}
			if g.NextWakeAt == nil || now.Before(*g.NextWakeAt) || busy {
				return nil, nil
			}
			g.Round++
			g.NextWakeAt = nil
			body := fmt.Sprintf("Goal turn %d/%d · %s\nREAD-ONLY OBJECTIVE: %s\nDONE WHEN: %s\nDeadline: %s\nPrevious summary: %s\nNext step: %s\nAdvance this goal while respecting the owner's permissions. Important actions still require human approval. Before finishing this turn, run myco goal report --id %s --summary <summary> --progressed (or --no-progress). For success use --achieved --reason <reason>; if impossible use --abandoned --reason <reason>; if blocked use --blocked. Do not wait for another user message. Report is bound to MYCO_MESSAGE_ID; do not report on behalf of another turn.", g.Round, g.MaxRounds, g.ID, g.Objective, g.DoneWhen, g.Deadline.UTC().Format(time.RFC3339), g.LastSummary, g.NextStep, g.ID)
			wake := s.message(r, true, body)
			r.Rounds = append(r.Rounds, domain.GoalRound{Goal: g.ID, Round: g.Round, StartedAt: now, Message: wake.ID})
			return []*domain.Message{wake}, nil
		})
		if e != nil {
			return e
		}
		s.publish(ctx, ms)
		for _, m := range ms {
			if m.Type == domain.TypeNote {
				s.audit.Record(ctx, Actor{Admin: true}, "goal.scheduled_outcome", record.Goal.ID, "automatic lifecycle notification")
			}
		}
	}
	return nil
}
