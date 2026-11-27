package app

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/myco-ai/myco/internal/domain"
)

type TaskWait struct{ ID, Kind string }
type TaskTree struct {
	Message   *domain.Message
	Relation  string
	Reference bool
	Waits     []TaskWait
	Children  []*TaskTree
}
type TaskTreeResult struct {
	Root      *TaskTree
	At        time.Time
	Truncated bool
}

func (s *MessageService) coordinationRepo() (domain.MessageCoordinationRepository, error) {
	r, ok := s.messages.(domain.MessageCoordinationRepository)
	if !ok {
		return nil, domain.Errf(domain.CodeUnsupported, "message coordination is not supported by this store")
	}
	return r, nil
}

func (s *MessageService) taskCoordinator(ctx context.Context, by Actor, project string) (bool, error) {
	if by.Agent == nil || project == "" || s.projects == nil {
		return false, nil
	}
	p, err := s.projects.Get(ctx, project)
	if err != nil {
		return false, err
	}
	ms, err := s.projects.Members(ctx, p.ID)
	if err != nil {
		return false, err
	}
	for _, m := range ms {
		if m.Agent == by.Agent.Name && m.Role == domain.ProjectRoleCoordinator {
			return true, nil
		}
	}
	return false, nil
}

func taskParticipant(by Actor, m *domain.Message) bool {
	if by.Manages() {
		return true
	}
	if by.Agent != nil {
		return m.Involves(by.Agent.Name)
	}
	return by.Member != nil && m.Involves(ConsoleSender(by.Member.Name))
}

func (s *MessageService) canReadTask(ctx context.Context, by Actor, m *domain.Message) (bool, error) {
	if taskParticipant(by, m) {
		return true, nil
	}
	coordinator, err := s.taskCoordinator(ctx, by, m.Project)
	if err != nil || !coordinator {
		return false, err
	}
	from, err := s.visibleTaskAgent(ctx, by, m.From)
	if err != nil || !from {
		return false, err
	}
	return s.visibleTaskAgent(ctx, by, m.To)
}

func (s *MessageService) visibleTaskAgent(ctx context.Context, by Actor, name domain.AgentName) (bool, error) {
	if IsConsoleSender(name) || by.Manages() {
		return true, nil
	}
	a, err := s.agents.Get(ctx, name)
	if domain.IsCode(err, domain.CodeNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if by.Agent != nil {
		return (domain.Viewer{Agent: by.Agent}).Sees(a), nil
	}
	if by.Member != nil {
		return (domain.Viewer{Member: by.Member.ID}).Sees(a), nil
	}
	return false, nil
}

// MessageTree exposes bounded task metadata, never message contents. Thread
// edges are labeled as inference; explicit parent/hold edges retain their kind.
func (s *MessageService) MessageTree(ctx context.Context, by Actor, id string, limit int) (*TaskTreeResult, error) {
	if limit < 1 || limit > 500 {
		return nil, domain.Errf(domain.CodeInvalidArgument, "limit must be between 1 and 500")
	}
	r, err := s.coordinationRepo()
	if err != nil {
		return nil, err
	}
	root, err := r.TaskMetadata(ctx, id)
	if err != nil {
		return nil, err
	}
	ok, err := s.canReadTask(ctx, by, root)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, domain.Errf(domain.CodeNotFound, "task not found")
	}
	if root.Type != domain.TypeTask {
		return nil, domain.Errf(domain.CodeInvalidArgument, "tree requires a task")
	}
	out := &TaskTreeResult{At: s.clock.Now()}
	seen := map[string]bool{}
	budget := limit
	var visit func(*domain.Message, string, int) (*TaskTree, error)
	visit = func(m *domain.Message, relation string, depth int) (*TaskTree, error) {
		n := &TaskTree{Message: m, Relation: relation, Children: []*TaskTree{}, Waits: []TaskWait{}}
		budget--
		if seen[m.ID] {
			n.Reference = true
			return n, nil
		}
		seen[m.ID] = true
		if !m.Status.Terminal() {
			for _, w := range []TaskWait{{m.After, "after"}, {m.HoldUntil, "task"}, {m.Asking, "answer"}} {
				if w.ID == domain.ManualHold {
					n.Waits = append(n.Waits, TaskWait{Kind: "manual"})
					continue
				}
				if w.ID == "" {
					continue
				}
				dep, e := r.TaskMetadata(ctx, w.ID)
				if domain.IsCode(e, domain.CodeNotFound) {
					n.Waits = append(n.Waits, TaskWait{Kind: "unknown"})
					continue
				}
				if e != nil {
					return nil, e
				}
				visible, e := s.canReadTask(ctx, by, dep)
				if e != nil {
					return nil, e
				}
				if !visible && dep.Project == root.Project {
					visible, e = s.visibleTaskAgent(ctx, by, dep.To)
					if e != nil {
						return nil, e
					}
				}
				if !visible {
					n.Waits = append(n.Waits, TaskWait{Kind: "restricted"})
					continue
				}
				if w.Kind == "answer" || !dep.Status.Terminal() {
					n.Waits = append(n.Waits, w)
				}
			}
		}
		childLimit := budget + 1
		children, e := r.TaskChildren(ctx, m, childLimit)
		if e != nil {
			return nil, e
		}
		// A full batch may hide later visible children after privacy filtering.
		// Report incomplete coverage instead of silently claiming a complete tree.
		if len(children) == childLimit {
			out.Truncated = true
		}
		for _, child := range children {
			visible, e := s.canReadTask(ctx, by, child)
			if e != nil {
				return nil, e
			}
			if !visible && child.Project == root.Project {
				a, e := s.visibleTaskAgent(ctx, by, child.From)
				if e != nil {
					return nil, e
				}
				b, e := s.visibleTaskAgent(ctx, by, child.To)
				if e != nil {
					return nil, e
				}
				visible = a && b
			}
			if !visible {
				continue
			}
			if budget <= 0 || depth >= 32 {
				out.Truncated = true
				break
			}
			rel := "thread_inferred"
			if child.ParentID == m.ID {
				rel = "parent"
			} else if child.ID == m.HoldUntil {
				rel = "hold"
			} else if child.ReplyTo == m.ID {
				rel = "reply_to"
			} else if child.ParentID != "" {
				rel = "parent_message"
			}
			cn, e := visit(child, rel, depth+1)
			if e != nil {
				return nil, e
			}
			n.Children = append(n.Children, cn)
		}
		return n, nil
	}
	out.Root, err = visit(root, "root", 0)
	return out, err
}

// parentForSend verifies provenance. The Hub never accepts a pointer to a
// message the sender is not currently entitled to handle.
func (s *MessageService) parentForSend(ctx context.Context, sender *domain.Agent, id string) (string, error) {
	if id == "" {
		return "", nil
	}
	m, err := s.messages.Get(ctx, id)
	if err != nil {
		return "", err
	}
	if m.To != sender.Name {
		return "", domain.Errf(domain.CodePermissionDenied, "parent must be a message addressed to the sender")
	}
	return m.ID, nil
}

type ReassignOutput struct {
	Message    *domain.Message
	PreviousTo domain.AgentName
	DryRun     bool
}

func (s *MessageService) Reassign(ctx context.Context, by Actor, id, to, note string, dry bool) (*ReassignOutput, error) {
	if utf8.RuneCountInString(note) > 2000 {
		return nil, domain.Errf(domain.CodeInvalidArgument, "note must be at most 2000 characters")
	}
	r, err := s.coordinationRepo()
	if err != nil {
		return nil, err
	}
	m, err := s.messages.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	name, err := domain.ParseAgentName(to)
	if err != nil {
		return nil, err
	}
	target, err := s.agents.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	allowed := by.Manages() || by.Agent != nil && m.From == by.Agent.Name || by.Member != nil && m.From == ConsoleSender(by.Member.Name)
	if !allowed {
		allowed, err = s.taskCoordinator(ctx, by, m.Project)
		if err != nil {
			return nil, err
		}
	}
	if !allowed {
		return nil, domain.Errf(domain.CodePermissionDenied, "only the sender, a project coordinator or an admin may reassign a task")
	}
	// A project role must not turn an otherwise hidden private task into a
	// forwarding capability. Apply the same visibility boundary as tree reads.
	readable, err := s.canReadTask(ctx, by, m)
	if err != nil {
		return nil, err
	}
	if !readable {
		return nil, domain.Errf(domain.CodePermissionDenied, "task is not visible to this coordinator").WithSuggestion("ask the original sender to reassign this task")
	}
	visible, err := s.visibleTaskAgent(ctx, by, target.Name)
	if err != nil {
		return nil, err
	}
	if !visible {
		return nil, domain.Errf(domain.CodeNotFound, "recipient not found")
	}
	original := &domain.Agent{Name: m.From}
	if IsConsoleSender(m.From) {
		if owner := s.ownerName(ctx, target); owner != "" && ConsoleSender(owner) == m.From {
			original.Owner = target.Owner
		}
		if target.Private() && original.Owner == "" {
			return nil, domain.Errf(domain.CodePermissionDenied, "new recipient is private to a different owner")
		}
	} else {
		original, err = s.agents.Get(ctx, m.From)
		if err != nil {
			return nil, err
		}
	}
	if !domain.MaySendTo(original, target) || !IsConsoleSender(m.From) && !target.Accepts(m.From) {
		return nil, domain.Errf(domain.CodePermissionDenied, "new recipient does not accept tasks from the original sender").WithSuggestion("choose a recipient visible to and accepting the original sender")
	}
	if m.Project != "" && s.projects != nil {
		p, e := s.projects.Get(ctx, m.Project)
		if e != nil {
			return nil, e
		}
		if p.Archived {
			return nil, domain.Errf(domain.CodeInvalidState, "project %s is archived", p.Name).WithSuggestion("unarchive the project before reassigning tasks")
		}
		members, e := s.projects.Members(ctx, p.ID)
		if e != nil {
			return nil, e
		}
		member := false
		for _, v := range members {
			if v.Agent == target.Name {
				member = true
				break
			}
		}
		if !member {
			return nil, domain.Errf(domain.CodePermissionDenied, "new recipient must belong to the task's project")
		}
	}
	before := *m
	apply := func(cur *domain.Message) (*domain.AuditEntry, error) {
		if domain.IsSealed(cur.Body) {
			return nil, domain.Errf(domain.CodeUnsupported, "E2E tasks cannot be reassigned by the Hub").WithSuggestion("the sender must stop + send a new task so it is encrypted for the new recipient")
		}
		if cur.Type != domain.TypeTask || cur.Status != domain.StatusQueued || cur.EverDelivered || cur.Attempts != 0 || !cur.ActiveAt.IsZero() {
			return nil, domain.Errf(domain.CodeInvalidState, "only an unclaimed queued task can be reassigned").WithSuggestion("stop the task, then send a new task with: myco message stop --id %s; myco message send --to %s", id, name)
		}
		if cur.To != before.To || cur.Project != before.Project || cur.From != before.From {
			return nil, domain.Errf(domain.CodeConflict, "task changed while preparing reassignment").WithSuggestion("read the task again before reassigning")
		}
		if cur.To == target.Name {
			return nil, domain.Errf(domain.CodeInvalidArgument, "task is already addressed to this recipient")
		}
		cur.To = target.Name
		cur.ReassignNote = strings.TrimSpace(note)
		cur.GrantedBy = ""
		cur.UpdatedAt = s.clock.Now()
		return &domain.AuditEntry{ID: s.ids.New("aud"), At: cur.UpdatedAt, Actor: by.MemberID(), ActorName: by.Name(), ActorKind: by.Kind(), Action: "message.reassign", Target: id, Detail: fmt.Sprintf("from %s to %s; note_present=%t", before.To, target.Name, cur.ReassignNote != "")}, nil
	}
	if dry {
		_, err = apply(m)
	} else {
		m, err = r.ReassignTask(ctx, id, apply)
	}
	if err != nil {
		return nil, err
	}
	if !dry {
		s.queued(target.Name)
	}
	return &ReassignOutput{Message: m, PreviousTo: before.To, DryRun: dry}, nil
}
