package app

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/myco-ai/myco/internal/domain"
)

// WatchService evaluates durable waits without starting a model or a command.
type WatchService struct {
	repo      domain.WatchRepository
	agents    domain.AgentRepository
	messages  *MessageService
	projects  *ProjectService
	workflows *WorkflowService
	audit     *AuditService
	bus       *EventBus
	ids       IDGenerator
	clock     Clock
	kick      chan struct{}
}

func NewWatchService(repo domain.WatchRepository, agents domain.AgentRepository, messages *MessageService, projects *ProjectService, workflows *WorkflowService, audit *AuditService, bus *EventBus, ids IDGenerator, clock Clock) *WatchService {
	s := &WatchService{repo: repo, agents: agents, messages: messages, projects: projects, workflows: workflows, audit: audit, bus: bus, ids: ids, clock: clock, kick: make(chan struct{}, 1)}
	bus.Subscribe(func(context.Context, domain.Event) {
		select {
		case s.kick <- struct{}{}:
		default:
		}
	})
	return s
}
func (s *WatchService) Run(ctx context.Context) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		if err := s.Tick(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("watch evaluation failed", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-s.kick:
		case <-t.C:
		}
	}
}
func (s *WatchService) allowed(ctx context.Context, by Actor, w *domain.Watch) bool {
	if by.Manages() {
		return true
	}
	a, err := s.agents.Get(ctx, domain.AgentName(w.Agent))
	if err != nil {
		return false
	}
	if by.Owns(a.Owner) || by.Agent != nil && by.Agent.Name == a.Name {
		return true
	}
	if by.Agent != nil && w.Project != "" && s.projects != nil {
		p, err := s.projects.load(ctx, w.Project)
		return err == nil && p.member(a.Name) != nil && p.coordinates(by.Agent)
	}
	return false
}
func (s *WatchService) Get(ctx context.Context, by Actor, id string) (*domain.Watch, error) {
	w, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !s.allowed(ctx, by, w) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return nil, domain.Errf(domain.CodePermissionDenied, "only the watching agent, its owner, or its project coordinator may access this watch")
	}
	return w, nil
}
func (s *WatchService) List(ctx context.Context, by Actor, agent, project string, all bool) ([]*domain.Watch, error) {
	ws, err := s.repo.List(ctx, !all)
	if err != nil {
		return nil, err
	}
	out := []*domain.Watch{}
	for _, w := range ws {
		if (agent == "" || w.Agent == agent) && (project == "" || w.Project == project) && s.allowed(ctx, by, w) {
			out = append(out, w)
		}
	}
	return out, ctx.Err()
}

type WatchStart struct {
	What, Kind, Target, Project, Task string
	Command, Match                    string
	EverySec                          int
	ForSec                            int
}

func (s *WatchService) Start(ctx context.Context, by Actor, in WatchStart, dry bool) (*domain.Watch, error) {
	if by.Agent == nil {
		return nil, domain.Errf(domain.CodePermissionDenied, "start a watch with the initiating agent's token")
	}
	in.What = strings.TrimSpace(in.What)
	in.Target = strings.TrimSpace(in.Target)
	if in.Target == "" && in.Kind != "command" {
		return nil, domain.Errf(domain.CodeInvalidArgument, "target is required")
	}
	if in.What == "" || utf8.RuneCountInString(in.What) > 200 || strings.ContainsAny(in.What, "\r\n") {
		return nil, domain.Errf(domain.CodeInvalidArgument, "what must be 1 to 200 characters")
	}
	if in.Kind != "task" && in.Kind != "reply" && in.Kind != "workflow" && in.Kind != "command" {
		return nil, domain.Errf(domain.CodeInvalidArgument, "kind must be reply, task, workflow, or command")
	}
	if in.Kind == "command" {
		if err := commandWatchAgent(by.Agent); err != nil {
			return nil, err
		}
		if strings.TrimSpace(in.Command) == "" || len(in.Command) > 8192 || strings.ContainsRune(in.Command, 0) || in.Target != "" {
			return nil, domain.Errf(domain.CodeInvalidArgument, "command requires 1–8192 bytes, no NUL, and no target")
		}
		if len(in.Match) > 2048 {
			return nil, domain.Errf(domain.CodeInvalidArgument, "match is too long (max 2048 bytes)")
		}
		if _, err := regexp.Compile(in.Match); err != nil {
			return nil, domain.Errf(domain.CodeInvalidArgument, "invalid match regular expression: %v", err)
		}
		if in.EverySec == 0 {
			in.EverySec = 30
		}
		if in.EverySec < 1 || in.EverySec > 86400 {
			return nil, domain.Errf(domain.CodeInvalidArgument, "every_sec must be between 1 and 86400")
		}
	} else if in.Command != "" || in.Match != "" || in.EverySec != 0 {
		return nil, domain.Errf(domain.CodeInvalidArgument, "command, match, and every_sec require kind=command")
	}
	if in.ForSec == 0 {
		in.ForSec = int(domain.WatchDefaultDuration / time.Second)
	}
	if in.ForSec < 1 || in.ForSec > int(domain.WatchMaxDuration/time.Second) {
		return nil, domain.Errf(domain.CodeInvalidArgument, "for_sec must be between 1 and 86400")
	}
	var bind func(*domain.Message) error
	if in.Task != "" {
		m, err := s.messages.Get(ctx, by.Agent, in.Task)
		if err != nil {
			return nil, err
		}
		if m.To != by.Agent.Name || m.Type != domain.TypeTask || m.Status.Terminal() {
			return nil, domain.Errf(domain.CodeInvalidArgument, "task must be an open task addressed to this agent")
		}
		if in.Project == "" {
			in.Project = m.Project
		}
		if in.Project != m.Project {
			return nil, domain.Errf(domain.CodeInvalidArgument, "watch project must match its task")
		}
		bind = func(task *domain.Message) error {
			if task.To != by.Agent.Name || task.Type != domain.TypeTask || task.Project != in.Project {
				return domain.Errf(domain.CodePermissionDenied, "task must belong to this agent and project")
			}
			if task.Asking != "" || task.Status == domain.StatusHeld && task.HoldUntil != "" && task.HoldUntil != domain.ManualHold && task.HoldUntil != in.Target {
				return domain.Errf(domain.CodeInvalidState, "task is held for another reason")
			}
			if err := task.Hold(by.Agent.Name, s.clock.Now()); err != nil {
				return err
			}
			task.HoldUntil = domain.ManualHold
			return nil
		}
		if err := bind(m); err != nil {
			return nil, err
		}
	}
	if in.Project != "" {
		p, err := s.projects.load(ctx, in.Project)
		if err != nil {
			return nil, err
		}
		if p.member(by.Agent.Name) == nil {
			return nil, domain.Errf(domain.CodePermissionDenied, "join the project before starting a watch for it")
		}
	}
	now := s.clock.Now()
	w := &domain.Watch{ID: s.ids.New("wat"), Agent: string(by.Agent.Name), What: in.What, Kind: in.Kind, Target: in.Target, Project: in.Project, Task: in.Task, StartedAt: now, Deadline: now.Add(time.Duration(in.ForSec) * time.Second), State: domain.WatchWatching, EverySec: 2}
	if in.Task != "" && in.Task == in.Target {
		return nil, domain.Errf(domain.CodeInvalidArgument, "watch cannot wait on its own associated task")
	}
	if in.Kind == "command" {
		w.Command, w.Match, w.EverySec = in.Command, in.Match, in.EverySec
		w.MachineID, w.Environment, w.Workdir = by.Agent.MachineID, by.Agent.Binding.Environment, by.Agent.Binding.Workdir
	}
	if in.Project != "" && (in.Kind == "task" || in.Kind == "reply") {
		m, err := s.messages.messages.Get(ctx, in.Target)
		if err != nil {
			return nil, err
		}
		if m.Project != in.Project {
			return nil, domain.Errf(domain.CodePermissionDenied, "target task is outside this watch's project")
		}
	}
	if _, _, _, err := s.condition(ctx, w); err != nil {
		return nil, err
	}
	if !dry {
		if bind != nil {
			parent, err := s.repo.Hold(ctx, w.Task, w, bind)
			if err != nil {
				return nil, err
			}
			s.bus.Publish(ctx, domain.MessageChanged{Message: parent})
		} else if err := s.repo.Create(ctx, w); err != nil {
			return nil, err
		}
		select {
		case s.kick <- struct{}{}:
		default:
		}
	}
	return w, nil
}
func (s *WatchService) condition(ctx context.Context, w *domain.Watch) (bool, string, time.Time, error) {
	if w.Kind == "command" {
		return false, "waiting for runner check", time.Time{}, nil
	}
	if w.Kind == "workflow" {
		a, err := s.agents.Get(ctx, domain.AgentName(w.Agent))
		if err != nil {
			return false, "", time.Time{}, err
		}
		c := Caller{Agent: a.Name, MemberID: a.Owner, Manages: a.Owner == ""}
		if w.Project != "" {
			c.ProjectScope = w.Project
		}
		r, err := s.workflows.getRun(ctx, c, w.Target)
		if err != nil {
			return false, "", time.Time{}, err
		}
		if w.Project != "" && r.Project != w.Project {
			return false, "", time.Time{}, domain.Errf(domain.CodePermissionDenied, "workflow run is outside this watch's project")
		}
		at := r.EndedAt
		if at.IsZero() {
			at = r.UpdatedAt
		}
		return domain.FlowOver(r.Status), "workflow " + r.Status, at, nil
	}
	m, err := s.messages.messages.Get(ctx, w.Target)
	if err != nil {
		return false, "", time.Time{}, err
	}
	if !m.Involves(domain.AgentName(w.Agent)) {
		return false, "", time.Time{}, domain.Errf(domain.CodePermissionDenied, "watch target must be a message involving this agent")
	}
	if m.Type != domain.TypeTask {
		return false, "", time.Time{}, domain.Errf(domain.CodeInvalidArgument, "watch target must be a task message")
	}
	if w.Kind == "reply" {
		if m.From != domain.AgentName(w.Agent) {
			return false, "", time.Time{}, domain.Errf(domain.CodePermissionDenied, "reply watch must target a task sent by this agent")
		}
		r, err := s.messages.messages.FindReply(ctx, m.ID)
		if domain.IsCode(err, domain.CodeNotFound) {
			return false, "waiting for reply", time.Time{}, nil
		}
		if err != nil {
			return false, "", time.Time{}, err
		}
		if r != nil && r.To == domain.AgentName(w.Agent) {
			return true, "reply received", r.CreatedAt, nil
		}
		return false, "waiting for reply", time.Time{}, nil
	}
	return m.Status.Terminal(), "task " + string(m.Status), m.UpdatedAt, nil
}
func (s *WatchService) wake(w *domain.Watch) *domain.Message {
	now := s.clock.Now()
	body := "Watch condition met: " + w.What + " (" + w.Target + ")."
	if w.Kind == "command" {
		body = "Watch condition met: " + w.What + "."
	}
	if w.State == domain.WatchExpired {
		body = fmt.Sprintf("盯了 %d 分钟没等到：%s。请决定接下来怎么做。\nWatch expired: %s. Decide what to do next.", int(now.Sub(w.StartedAt)/time.Minute), w.What, w.What)
	}
	if w.State == domain.WatchStopped {
		body = "Watch stopped: " + w.What + ". " + w.Outcome
	}
	if w.Kind == "command" && w.LastResult != "" {
		body += "\nLast check: " + domain.SanitizeWatchResult(w.LastResult)
	}
	id := s.ids.New("msg")
	return &domain.Message{ID: id, From: domain.AgentName(w.Agent), To: domain.AgentName(w.Agent), ThreadID: id, Type: domain.TypeEvent, Body: body, Status: domain.StatusQueued, CreatedAt: now, UpdatedAt: now, Project: w.Project}
}
func (s *WatchService) publish(ctx context.Context, parent, wake *domain.Message) {
	if parent != nil {
		s.bus.Publish(ctx, domain.MessageChanged{Message: parent})
		s.messages.queued(parent.To)
	}
	if wake != nil {
		s.bus.Publish(ctx, domain.MessageCreated{Message: wake})
		if wake.Status == domain.StatusQueued {
			s.messages.queued(wake.To)
		}
	}
}
func (s *WatchService) Tick(ctx context.Context) error {
	ws, err := s.repo.List(ctx, true)
	if err != nil {
		return err
	}
	for _, w := range ws {
		if err := ctx.Err(); err != nil {
			return err
		}
		now := s.clock.Now()
		if now.Before(w.Deadline) && w.LastCheckAt != nil && now.Sub(*w.LastCheckAt) < 2*time.Second {
			continue
		}
		oldDeadline, oldChecks := w.Deadline, w.Checks
		state, outcome := domain.WatchWatching, ""
		if _, e := s.agents.Get(ctx, domain.AgentName(w.Agent)); e != nil {
			if !domain.IsCode(e, domain.CodeNotFound) {
				return e
			}
			state, outcome = domain.WatchStopped, "watching agent removed"
		}
		if w.Task != "" {
			m, e := s.messages.messages.Get(ctx, w.Task)
			if e != nil && !domain.IsCode(e, domain.CodeNotFound) {
				return e
			}
			if m == nil || m.Status.Terminal() {
				state, outcome = domain.WatchStopped, "associated task ended"
			} else if m.Status != domain.StatusHeld || m.Asking != "" || m.HoldUntil != "" && m.HoldUntil != domain.ManualHold && m.HoldUntil != w.Target {
				state, outcome = domain.WatchStopped, "associated task resumed"
			}
		}
		if state == domain.WatchWatching && w.Kind == "command" {
			if now.Before(w.Deadline) {
				continue
			}
			state, outcome = domain.WatchExpired, "deadline reached"
		}
		if state == domain.WatchWatching {
			met, result, at, e := s.condition(ctx, w)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			now = s.clock.Now()
			if e != nil {
				if !domain.IsCode(e, domain.CodeNotFound) && !domain.IsCode(e, domain.CodePermissionDenied) {
					return e
				}
				if !now.Before(w.Deadline) {
					state, outcome = domain.WatchExpired, "deadline reached"
				} else {
					state, outcome = domain.WatchStopped, "target is no longer available"
				}
			} else {
				w.LastCheckAt, w.Checks, w.LastResult = &now, w.Checks+1, result
				if at.IsZero() {
					at = now
				}
				if met && at.Before(w.Deadline) {
					state, outcome = domain.WatchMet, result
				} else if !now.Before(w.Deadline) {
					state, outcome = domain.WatchExpired, "deadline reached"
				}
			}
		}
		var wake *domain.Message
		if state != domain.WatchWatching {
			if err = w.End(now, state, "hub", outcome); err != nil {
				return err
			}
			if outcome != "associated task ended" && outcome != "watching agent removed" && outcome != "associated task resumed" {
				wake = s.wake(w)
			}
		}
		changed, parent, created, err := s.repo.Save(ctx, w, oldDeadline, oldChecks, wake)
		if err != nil {
			return err
		}
		if changed {
			s.publish(ctx, parent, created)
		}
	}
	return nil
}
func (s *WatchService) Change(ctx context.Context, by Actor, id, reason string, seconds int, stop, dry bool) (*domain.Watch, error) {
	w, err := s.Get(ctx, by, id)
	if err != nil {
		return nil, err
	}
	oldDeadline, oldChecks := w.Deadline, w.Checks
	if !s.clock.Now().Before(w.Deadline) {
		return nil, domain.Errf(domain.CodeInvalidState, "watch deadline has passed")
	}
	if !stop && (seconds < 1 || seconds > 86400) {
		return nil, domain.Errf(domain.CodeInvalidArgument, "for_sec must be between 1 and 86400")
	}
	if utf8.RuneCountInString(reason) > 200 {
		return nil, domain.Errf(domain.CodeInvalidArgument, "reason must be at most 200 characters")
	}
	if stop {
		err = w.End(s.clock.Now(), domain.WatchStopped, by.Name(), strings.TrimSpace(reason))
	} else {
		err = w.Extend(s.clock.Now(), time.Duration(seconds)*time.Second)
	}
	if err != nil {
		return nil, err
	}
	if dry {
		return w, nil
	}
	var wake *domain.Message
	if stop {
		wake = s.wake(w)
	}
	changed, parent, created, err := s.repo.Save(ctx, w, oldDeadline, oldChecks, wake)
	if err != nil {
		return nil, err
	}
	if !changed {
		return nil, domain.Errf(domain.CodeConflict, "watch changed; read it again before retrying")
	}
	s.publish(ctx, parent, created)
	action := "watch.extend"
	if stop {
		action = "watch.stop"
	}
	detail := reason
	if !stop {
		detail = "deadline=" + w.Deadline.UTC().Format(time.RFC3339Nano)
	}
	s.audit.Record(ctx, by, action, w.ID, detail)
	return w, nil
}
func (s *WatchService) Hold(ctx context.Context, agent *domain.Agent, id string, child *domain.Message, fn func(*domain.Message) error) (*domain.Message, error) {
	now := s.clock.Now()
	w := &domain.Watch{ID: s.ids.New("wat"), Agent: string(agent.Name), Task: id, Kind: "task", Target: child.ID, What: string(child.To) + " 的任务回复", StartedAt: now, Deadline: now.Add(domain.WatchDefaultDuration), State: domain.WatchWatching, EverySec: 2}
	m, err := s.repo.Hold(ctx, id, w, func(m *domain.Message) error {
		if err := fn(m); err != nil {
			return err
		}
		w.Project = m.Project
		// Never copy encrypted or cross-project task content into a public label.
		if child.Project == m.Project && !domain.IsSealed(child.Body) {
			title := strings.TrimSpace(strings.SplitN(child.Body, "\n", 2)[0])
			rs := []rune(title)
			if len(rs) > 80 {
				title = string(rs[:80])
			}
			w.What = string(child.To) + ": " + title
		}
		return nil
	})
	if err == nil {
		s.bus.Publish(ctx, domain.MessageChanged{Message: m})
		select {
		case s.kick <- struct{}{}:
		default:
		}
	}
	return m, err
}
