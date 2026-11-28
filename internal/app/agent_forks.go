package app

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/domain"
)

const ForkFeature = "fork_retire_v1"
const ForkOperationCLI = "myco-fork"
const RetireOperationCLI = "myco-retire"
const ForkIdleTTL = 30 * time.Minute

type ForkInput struct {
	From       string `json:"from"`
	Count      int    `json:"count,omitempty"`
	Worktree   bool   `json:"worktree,omitempty"`
	NamePrefix string `json:"name_prefix,omitempty"`
	ForTask    string `json:"for_task,omitempty"`
	Force      bool   `json:"force,omitempty"`
	RequestID  string `json:"request_id"`
}
type RetireInput struct {
	Name   string `json:"name,omitempty"`
	ForkOf string `json:"fork_of,omitempty"`
	Now    bool   `json:"now,omitempty"`
}
type ForkCompletion struct {
	Workdir    string `json:"workdir"`
	Repository string `json:"repository,omitempty"`
	Branch     string `json:"branch,omitempty"`
	Retired    bool   `json:"retired,omitempty"`
	Error      string `json:"error,omitempty"`
}

// ForkService keeps lifecycle authority separate from user-editable cards.
type ForkService struct {
	repo     domain.ForkRepository
	agents   domain.AgentRepository
	machines *MachineService
	messages *MessageService
	ids      IDGenerator
	clock    Clock
}

func NewForkService(repo domain.ForkRepository, agents domain.AgentRepository, machines *MachineService, messages *MessageService, ids IDGenerator, clock Clock) *ForkService {
	return &ForkService{repo, agents, machines, messages, ids, clock}
}
func forkUnsupported(why string) error {
	return domain.Errf(domain.CodeUnsupported, "%s", why).WithSuggestion("upgrade the Hub and this machine's runner for agent fork/retire")
}
func forkCreator(a Actor) string {
	if a.Agent != nil {
		return "agent:" + a.Agent.ID
	}
	if a.Member != nil {
		return "member:" + a.Member.ID
	}
	if a.Admin {
		return "admin"
	}
	return ""
}
func forkBy(a Actor) string {
	if a.Agent != nil {
		return string(a.Agent.Name)
	}
	return a.Name()
}
func (s *ForkService) all(ctx context.Context) ([]*domain.AgentFork, error) { return s.repo.List(ctx) }
func (s *ForkService) permitted(ctx context.Context, a Actor, f *domain.AgentFork) bool {
	if a.Manages() || (forkCreator(a) != "" && forkCreator(a) == f.CreatorID) {
		return true
	}
	m, err := s.machines.Get(ctx, f.MachineID)
	return err == nil && a.Owns(m.Owner)
}
func (s *ForkService) Get(ctx context.Context, a Actor, id string) (*domain.AgentFork, error) {
	f, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if a.Agent != nil && a.Agent.ID == f.AgentID {
		m, e := s.machines.Get(ctx, f.MachineID)
		if e != nil {
			return nil, e
		}
		if a.Agent.Owner != f.Owner || a.Agent.MachineID != f.MachineID || m.Owner != f.Owner {
			return nil, domain.Errf(domain.CodePermissionDenied, "copy ownership or machine changed")
		}
		if strings.HasPrefix(a.Agent.Binding.SessionID, "fork:") {
			src, e := s.agents.Get(ctx, domain.AgentName(f.ForkOf))
			if e != nil {
				return nil, e
			}
			if src.ID != f.SourceID || src.Owner != f.Owner || src.MachineID != f.MachineID || src.Binding.SessionID != f.SourceSession || src.Visibility != f.SourceVisibility || !slices.Equal(src.AllowFrom, f.SourceAllowFrom) {
				return nil, domain.Errf(domain.CodePermissionDenied, "source binding or permissions changed before the first fork turn")
			}
		}
		return f, nil
	}
	if !s.permitted(ctx, a, f) {
		return nil, domain.Errf(domain.CodePermissionDenied, "only the creator, machine owner or an administrator may inspect this copy")
	}
	return f, nil
}
func (s *ForkService) Fork(ctx context.Context, a Actor, v domain.Viewer, in ForkInput, dry bool) ([]*domain.AgentFork, error) {
	if in.Count == 0 {
		in.Count = 1
	}
	if in.Count < 1 || in.Count > 32 {
		return nil, domain.Errf(domain.CodeInvalidArgument, "count must be between 1 and 32")
	}
	if in.RequestID == "" || len(in.RequestID) > 100 {
		return nil, domain.Errf(domain.CodeInvalidArgument, "request_id is required and must be at most 100 characters")
	}
	src, err := s.agents.Get(ctx, domain.AgentName(in.From))
	if err != nil {
		return nil, err
	}
	if !v.Sees(src) {
		return nil, domain.Errf(domain.CodeNotFound, "source agent not found")
	}
	fs, err := s.all(ctx)
	if err != nil {
		return nil, err
	}
	for _, prior := range fs {
		if prior.CreatorID == forkCreator(a) && prior.Batch == in.RequestID {
			out := []*domain.AgentFork{}
			for _, f := range fs {
				if f.CreatorID == prior.CreatorID && f.Batch == prior.Batch {
					if f.SourceID != src.ID || f.Worktree != in.Worktree || f.ForTask != in.ForTask || f.Force != in.Force || f.NamePrefix != strings.TrimSpace(in.NamePrefix) {
						return nil, domain.Errf(domain.CodeConflict, "batch key was used for a different fork request")
					}
					out = append(out, f)
				}
			}
			if len(out) != in.Count {
				return nil, domain.Errf(domain.CodeConflict, "batch key was used with a different count")
			}
			if !dry {
				for i, f := range out {
					if f.State == "planned" && f.MessageID == "" {
						sent, e := s.sendFork(ctx, a, f, false)
						if e != nil {
							return nil, e
						}
						out[i], e = s.repo.Mutate(ctx, f.ID, func(x *domain.AgentFork) error { x.MessageID = sent.Message.ID; return nil })
						if e != nil {
							return nil, e
						}
					}
				}
			}
			return out, nil
		}
	}
	for _, f := range fs {
		if f.State != "retired" && ((a.Agent != nil && f.AgentID == a.Agent.ID) || f.AgentID == src.ID) {
			return nil, domain.Errf(domain.CodePermissionDenied, "copies cannot fork agents or be forked again")
		}
	}
	if strings.HasPrefix(src.Binding.SessionID, "fork:") {
		return nil, domain.Errf(domain.CodePermissionDenied, "an unmaterialized session copy cannot be forked again")
	}
	if src.Binding.SessionID == "" || src.Binding.Workdir == "" || src.MachineID == "" {
		return nil, domain.Errf(domain.CodeInvalidState, "source needs a bound session, working directory and machine")
	}
	if src.Binding.Harness != "codex" && src.Binding.Harness != "claude" {
		return nil, forkUnsupported("this source harness does not support managed session forks")
	}
	if in.Worktree && src.Binding.Harness == "claude" {
		return nil, forkUnsupported("Claude session lookup is directory-scoped; --worktree is not supported for Claude")
	}
	nodeName, err := s.machines.SpawnNode(ctx, src.MachineID)
	if err != nil {
		return nil, err
	}
	node, err := s.agents.Get(ctx, nodeName)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(node.Runtime.FeatureList(), ForkFeature) {
		return nil, forkUnsupported("the machine runner does not advertise managed fork/retire support")
	}
	m, err := s.machines.Get(ctx, src.MachineID)
	if err != nil {
		return nil, err
	}
	if m.Owner != src.Owner || node.Owner != m.Owner {
		return nil, domain.Errf(domain.CodePermissionDenied, "source and machine ownership do not match")
	}
	if a.Agent == nil && !a.Owns(m.Owner) {
		return nil, domain.Errf(domain.CodePermissionDenied, "only this machine's owner or an administrator may fork from the console")
	}
	if in.ForTask != "" && a.Agent != nil {
		if _, err := s.messages.Get(ctx, a.Agent, in.ForTask); err != nil {
			return nil, err
		}
	}
	capacity := -1
	if !in.Force {
		q := s.machines.AccountQuota(ctx, src)
		if domain.QuotaStatusAt(q, s.clock.Now()) != "ok" {
			return nil, domain.Errf(domain.CodeInvalidState, "source quota status is not ok").WithSuggestion("wait for healthy quota data or pass --force to override only the resource checks")
		}
		var slots struct {
			MaxConcurrent int       `json:"max_concurrent"`
			InUse         int       `json:"in_use"`
			At            time.Time `json:"at"`
		}
		if json.Unmarshal([]byte(src.Runtime.RunnerSlots), &slots) != nil || slots.MaxConcurrent < 1 || slots.At.IsZero() || s.clock.Now().Sub(slots.At) > 2*time.Minute || slots.At.After(s.clock.Now().Add(time.Minute)) {
			return nil, domain.Errf(domain.CodeInvalidState, "fresh runner slot data is unavailable").WithSuggestion("wait for the runner report or pass --force")
		}
		capacity = slots.MaxConcurrent - slots.InUse
		if capacity < in.Count {
			return nil, domain.Errf(domain.CodeConflict, "runner has insufficient free slots").WithSuggestion("wait for free slots or pass --force")
		}
	}
	prefix := strings.TrimSpace(in.NamePrefix)
	if prefix == "" {
		prefix = string(src.Name) + "-fork"
	}
	if _, err := domain.ParseAgentName(strings.TrimSuffix(prefix, "-")); err != nil {
		return nil, err
	}
	machinePrefix := strings.TrimSuffix(string(node.Name), "machine")
	if in.NamePrefix == "" {
		n := 63 - 9
		if !strings.HasPrefix(prefix, machinePrefix) {
			n -= len(machinePrefix)
		}
		if n < 1 {
			return nil, domain.Errf(domain.CodeInvalidArgument, "machine prefix leaves no space for a copy name")
		}
		if len(prefix) > n {
			prefix = prefix[:n]
		}
	}
	out := make([]*domain.AgentFork, 0, in.Count)
	for i := 0; i < in.Count; i++ {
		id := s.ids.New("fork")
		suffix := id
		if len(suffix) > 8 {
			suffix = suffix[len(suffix)-8:]
		}
		name := strings.TrimSuffix(prefix, "-") + "-" + suffix
		if !strings.HasPrefix(name, machinePrefix) {
			name = machinePrefix + name
		}
		if _, err := domain.ParseAgentName(name); err != nil {
			return nil, err
		}
		if _, err = s.agents.Get(ctx, domain.AgentName(name)); err == nil {
			return nil, domain.Errf(domain.CodeConflict, "copy name already exists")
		}
		if !domain.IsCode(err, domain.CodeNotFound) {
			return nil, err
		}
		f := &domain.AgentFork{ID: id, Batch: in.RequestID, NamePrefix: strings.TrimSpace(in.NamePrefix), SourceID: src.ID, SourceVisibility: src.Visibility, SourceAllowFrom: append([]domain.AgentName(nil), src.AllowFrom...), ForkOf: string(src.Name), CLI: src.Binding.Harness, SourceSession: src.Binding.SessionID, SourceDir: src.Binding.Workdir, NodeID: node.ID, Node: string(node.Name), MachineID: src.MachineID, Owner: m.Owner, Name: name, ForkedBy: forkBy(a), CreatorID: forkCreator(a), ForTask: in.ForTask, Worktree: in.Worktree, Force: in.Force, Access: src.Runtime.Access, Model: src.Runtime.Model, State: "planned", CreatedAt: s.clock.Now().UTC().Truncate(time.Millisecond)}
		if _, err = s.sendFork(ctx, a, f, true); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	if dry {
		return out, nil
	}
	out, err = s.repo.CreateBatch(ctx, out, capacity)
	if err != nil {
		return nil, err
	}
	for i, f := range out {
		if f.State != "planned" || f.MessageID != "" {
			continue
		}
		msg, err := s.sendFork(ctx, a, f, false)
		if err != nil {
			return out, err
		}
		out[i], err = s.repo.Mutate(ctx, f.ID, func(x *domain.AgentFork) error { x.MessageID = msg.Message.ID; return nil })
		if err != nil {
			return out, err
		}
	}
	return out, nil
}
func (s *ForkService) sendFork(ctx context.Context, a Actor, f *domain.AgentFork, dry bool) (*SendOutput, error) {
	body, _ := json.Marshal(map[string]string{"cli": ForkOperationCLI, "fork_id": f.ID})
	sender := a.Agent
	if sender == nil {
		sender = &domain.Agent{Name: AdminSender}
	}
	return s.messages.send(ctx, sender, SendInput{To: f.Node, Type: "task", Body: string(body), IdempotencyKey: "managed-fork:" + f.ID, DryRun: dry}, a.Agent == nil)
}
func (s *ForkService) Retire(ctx context.Context, a Actor, in RetireInput, dry bool) ([]*domain.AgentFork, error) {
	if (in.Name == "") == (in.ForkOf == "") {
		return nil, domain.Errf(domain.CodeInvalidArgument, "choose exactly one of name and fork_of")
	}
	fs, err := s.all(ctx)
	if err != nil {
		return nil, err
	}
	out := []*domain.AgentFork{}
	for _, f := range fs {
		if (in.Name != "" && f.Name == in.Name) || (in.ForkOf != "" && f.ForkOf == in.ForkOf) {
			if !s.permitted(ctx, a, f) {
				return nil, domain.Errf(domain.CodePermissionDenied, "only the creator, machine owner or an administrator may retire a copy")
			}
			if f.State != "retired" {
				out = append(out, f)
			}
		}
	}
	if len(out) == 0 && in.Name != "" {
		for _, f := range fs {
			if f.Name == in.Name && f.State == "retired" {
				return []*domain.AgentFork{f}, nil
			}
		}
		return nil, domain.Errf(domain.CodePermissionDenied, "retirement is restricted to registered managed copies")
	}
	if dry {
		return out, nil
	}
	for i, f := range out {
		retry := false
		if f.RetireMessageID != "" {
			m, e := s.messages.messages.Get(ctx, f.RetireMessageID)
			if e != nil {
				return nil, e
			}
			retry = m.Status.Terminal()
		}
		retiring, e := s.repo.BeginRetirement(ctx, f.ID, in.Now)
		if e != nil {
			return nil, e
		}
		f = retiring
		out[i], err = s.repo.Mutate(ctx, f.ID, func(x *domain.AgentFork) error {
			if x.State == "retired" {
				return nil
			}
			x.Now = x.Now || in.Now
			x.Error = ""
			if retry {
				x.RetireMessageID = ""
				x.RetireAttempt++
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	// A later tick retries dispatch; marking retirement does not depend on the
	// caller keeping this HTTP request alive.
	for _, f := range out {
		_ = s.dispatchRetire(ctx, f)
	}
	return out, nil
}
func (s *ForkService) dispatchRetire(ctx context.Context, f *domain.AgentFork) error {
	if f.State != "retiring" || f.Error != "" {
		return nil
	}
	if f.RetireMessageID != "" {
		m, err := s.messages.messages.Get(ctx, f.RetireMessageID)
		if err != nil {
			return err
		}
		if !m.Status.Terminal() {
			return nil
		}
		old := f.RetireMessageID
		f, err = s.repo.Mutate(ctx, f.ID, func(x *domain.AgentFork) error {
			if x.State == "retiring" && x.RetireMessageID == old && x.Error == "" {
				x.RetireMessageID = ""
				x.RetireAttempt++
			}
			return nil
		})
		if err != nil {
			return err
		}
		if f.RetireMessageID != "" || f.State != "retiring" || f.Error != "" {
			return nil
		}
	}
	busy, err := s.repo.Busy(ctx, f)
	if err != nil || (!f.Now && busy) {
		return err
	}
	body, _ := json.Marshal(map[string]string{"cli": RetireOperationCLI, "fork_id": f.ID})
	out, err := s.messages.send(ctx, &domain.Agent{Name: AdminSender}, SendInput{To: f.Node, Type: "task", Body: string(body), IdempotencyKey: fmt.Sprintf("managed-retire:%s:%d", f.ID, f.RetireAttempt)}, true)
	if err != nil {
		return err
	}
	_, err = s.repo.Mutate(ctx, f.ID, func(x *domain.AgentFork) error { x.RetireMessageID = out.Message.ID; return nil })
	return err
}

// MachinePlan is readable only by the exact registered machine-node identity.
func (s *ForkService) MachinePlan(ctx context.Context, node *domain.Agent, id string) (*domain.AgentFork, error) {
	f, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	m, err := s.machines.Get(ctx, f.MachineID)
	if err != nil {
		return nil, err
	}
	if node.ID != f.NodeID || node.Kind() != domain.KindMachine || node.MachineID != f.MachineID || node.Owner != f.Owner || m.Owner != f.Owner {
		return nil, domain.Errf(domain.CodePermissionDenied, "copy belongs to a different machine identity or owner")
	}
	if f.State == "retiring" && !f.Now {
		busy, e := s.repo.Busy(ctx, f)
		if e != nil {
			return nil, e
		}
		if busy {
			return nil, domain.Errf(domain.CodeInvalidState, "copy still has current work; wait for it or explicitly request --now")
		}
	}
	if f.State == "planned" {
		src, err := s.agents.Get(ctx, domain.AgentName(f.ForkOf))
		if err != nil {
			return nil, err
		}
		if src.ID != f.SourceID || src.Binding.SessionID != f.SourceSession || src.Binding.Workdir != f.SourceDir || src.Owner != f.Owner || src.MachineID != f.MachineID || src.Visibility != f.SourceVisibility || !slices.Equal(src.AllowFrom, f.SourceAllowFrom) {
			return nil, domain.Errf(domain.CodeConflict, "source binding or ownership changed")
		}
	}
	return f, nil
}
func (s *ForkService) Complete(ctx context.Context, node *domain.Agent, id string, in ForkCompletion) (*domain.AgentFork, error) {
	f, err := s.MachinePlan(ctx, node, id)
	if err != nil {
		return nil, err
	}
	if in.Error != "" {
		return s.repo.Mutate(ctx, id, func(x *domain.AgentFork) error {
			x.Error = "machine could not complete the operation; check its local logs"
			return nil
		})
	}
	if in.Retired {
		if f.State == "retired" {
			return f, nil
		}
		out, err := s.repo.Finalize(ctx, id, s.clock.Now(), func() string { return s.ids.New("msg") })
		if err != nil {
			return nil, err
		}
		for _, event := range out.Events {
			s.messages.queued(event.To)
		}
		for _, closed := range out.Closed {
			s.messages.notifier.Publish(replyTopic(closed.ID))
			s.messages.wakeAfter(ctx, closed.ID)
			s.messages.workflowEnded(closed.Workflow)
		}
		return out.Fork, nil
	}
	if in.Workdir == "" || len(in.Workdir) > 512 || len(in.Repository) > 512 || len(in.Branch) > 128 {
		return nil, domain.Errf(domain.CodeInvalidArgument, "invalid fork completion")
	}
	return s.repo.Activate(ctx, id, in.Workdir, in.Repository, in.Branch)
}
func (s *ForkService) Tick(ctx context.Context) error {
	fs, err := s.all(ctx)
	if err != nil {
		return err
	}
	for _, f := range fs {
		if f.State == "ready" || f.State == "planned" {
			f, err = s.repo.ExpireIdle(ctx, f.ID, s.clock.Now().Add(-ForkIdleTTL))
			if err != nil {
				return err
			}
		}
		if f.State == "retiring" {
			if err := s.dispatchRetire(ctx, f); err != nil {
				return fmt.Errorf("dispatch copy retirement: %w", err)
			}
		}
	}
	return nil
}
func (s *ForkService) Run(ctx context.Context) {
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_ = s.Tick(ctx)
		}
	}
}
