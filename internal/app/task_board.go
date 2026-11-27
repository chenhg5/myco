package app

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/domain"
)

const TaskAlertCooldown = 5 * time.Minute
const TaskFailureWindow = 10 * time.Minute
const TaskMonitorTick = 30 * time.Second

// TaskMonitorService evaluates only metadata. Delivery and rate limits are
// persisted atomically by the repository, so restarts do not repeat wakeups.
type TaskMonitorService struct {
	repo         domain.TaskMonitorRepository
	projects     *ProjectService
	messages     *MessageService
	bus          *EventBus
	ids          IDGenerator
	clock        Clock
	onlineWindow time.Duration
	mu           sync.Mutex
}

func NewTaskMonitorService(repo domain.TaskMonitorRepository, projects *ProjectService, messages *MessageService, bus *EventBus, ids IDGenerator, clock Clock, onlineWindow time.Duration) *TaskMonitorService {
	return &TaskMonitorService{repo: repo, projects: projects, messages: messages, bus: bus, ids: ids, clock: clock, onlineWindow: onlineWindow}
}
func taskActor(by Actor) domain.AgentName {
	if by.Agent != nil {
		return by.Agent.Name
	}
	if by.Member != nil {
		return ConsoleSender(by.Member.Name)
	}
	return ConsoleSender("")
}
func (s *TaskMonitorService) project(ctx context.Context, by Actor, name string, manage bool) (*ProjectView, error) {
	if s.projects == nil {
		return nil, domain.Errf(domain.CodeUnsupported, "task projects are unavailable").WithSuggestion("upgrade the workspace Hub")
	}
	p, err := s.projects.Get(ctx, by, name)
	if err != nil {
		return nil, err
	}
	if manage && !by.Manages() && !(by.Member != nil && p.Project.Owner == by.Member.ID) && !(by.Agent != nil && p.coordinates(by.Agent)) {
		return nil, domain.Errf(domain.CodePermissionDenied, "task board --all and alert settings require a project coordinator or owner").WithSuggestion("use your sent-task board without --all, or ask the project's owner")
	}
	return p, nil
}
func validTaskStale(d time.Duration) error {
	if d < time.Second || d > 7*24*time.Hour || d%time.Second != 0 {
		return domain.Errf(domain.CodeInvalidArgument, "stale threshold must be whole seconds from 1s to 168h").WithSuggestion("use a duration such as 15m")
	}
	return nil
}
func (s *TaskMonitorService) Board(ctx context.Context, by Actor, project string, all bool, stale time.Duration) (api.TaskBoard, error) {
	out := api.TaskBoard{ObservedAt: s.clock.Now(), Tasks: []api.TaskBoardItem{}}
	if by.Agent == nil && by.Member == nil && !by.Admin {
		return out, domain.Errf(domain.CodeUnauthenticated, "task board requires an authenticated caller").WithSuggestion("use your agent or console credentials")
	}
	if all && project == "" {
		return out, domain.Errf(domain.CodeInvalidArgument, "--all requires --project").WithSuggestion("use myco task board --project <name> --all")
	}
	if stale != 0 {
		if err := validTaskStale(stale); err != nil {
			return out, err
		}
	}
	f := domain.TaskFilter{Sender: taskActor(by), Project: project}
	if by.Member != nil {
		f.ConsoleOwner = by.Member.ID
	}
	if project != "" {
		p, err := s.project(ctx, by, project, all)
		if err != nil {
			return out, err
		}
		f.Project = p.Project.Name
	}
	if all {
		f.Sender = ""
		f.ConsoleOwner = ""
	}
	rows, err := s.repo.ListTasks(ctx, f)
	if err != nil {
		return out, err
	}
	for _, r := range rows {
		threshold := r.StaleAfter
		if stale != 0 {
			threshold = stale
		}
		last := r.LastActivity()
		row := api.TaskBoardItem{ID: r.ID, From: string(r.Sender), To: string(r.Recipient), ThreadID: r.ThreadID, Project: r.Project, Status: string(r.Status), CreatedAt: r.CreatedAt, LastActivityAt: last, AgeMS: max(int64(0), out.ObservedAt.Sub(r.CreatedAt).Milliseconds()), IdleMS: max(int64(0), out.ObservedAt.Sub(last).Milliseconds()), StaleAfterMS: threshold.Milliseconds()}
		if !r.ActiveAt.IsZero() {
			at := r.ActiveAt
			row.ActiveAt = &at
		}
		if r.Status == domain.StatusQueued {
			row.QueuedForMS = row.AgeMS
		}
		row.Stale = !out.ObservedAt.Before(last.Add(threshold))
		out.Tasks = append(out.Tasks, row)
	}
	sort.Slice(out.Tasks, func(i, j int) bool {
		a, b := out.Tasks[i], out.Tasks[j]
		if a.Stale != b.Stale {
			return a.Stale
		}
		if a.IdleMS != b.IdleMS {
			return a.IdleMS > b.IdleMS
		}
		return a.ID < b.ID
	})
	return out, nil
}
func (s *TaskMonitorService) AlertPolicy(ctx context.Context, by Actor, name string, set *time.Duration, dry bool) (api.TaskAlertPolicy, error) {
	p, err := s.project(ctx, by, name, true)
	if err != nil {
		return api.TaskAlertPolicy{}, err
	}
	d, err := s.repo.TaskAlertThreshold(ctx, p.Project.ID)
	if err != nil {
		return api.TaskAlertPolicy{}, err
	}
	if set != nil {
		if err := validTaskStale(*set); err != nil {
			return api.TaskAlertPolicy{}, err
		}
		d = *set
		if !dry {
			if err := s.repo.SetTaskAlertThreshold(ctx, p.Project.ID, d); err != nil {
				return api.TaskAlertPolicy{}, err
			}
		}
	}
	return api.TaskAlertPolicy{Project: p.Project.Name, StaleSeconds: int(d / time.Second)}, nil
}

// taskAlertCandidates is pure and keeps event time separate from observation time.
func taskAlertCandidates(tasks []domain.TaskRecord, failures []domain.TaskFailure, now time.Time, onlineWindow time.Duration) []domain.TaskAlert {
	var out []domain.TaskAlert
	for _, t := range tasks {
		if IsConsoleSender(t.Sender) && t.SenderOwner == "" {
			continue
		}
		if !now.Before(t.LastActivity().Add(t.StaleAfter)) {
			out = append(out, domain.TaskAlert{Key: "stale:" + t.ID, Kind: "stale", TaskID: t.ID, Project: t.Project, SenderOwner: t.SenderOwner, Sender: t.Sender, Recipient: t.Recipient, At: now})
		}
		if (t.Status == domain.StatusAcked || t.Status == domain.StatusDelivered) && !t.RecipientSeenAt.IsZero() && !t.RecipientSeenAt.Add(onlineWindow).Before(t.CreatedAt) && now.Sub(t.RecipientSeenAt) > onlineWindow {
			out = append(out, domain.TaskAlert{Key: "offline:" + t.ID, Kind: "offline", TaskID: t.ID, Project: t.Project, SenderOwner: t.SenderOwner, Sender: t.Sender, Recipient: t.Recipient, At: now})
		}
	}
	groups := map[string][]domain.TaskFailure{}
	for _, f := range failures {
		if f.At.After(now) || f.At.Before(now.Add(-TaskFailureWindow)) {
			continue
		}
		key := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%q/%q/%d", f.Machine, f.MachineOwner, f.Incident))))
		groups[key] = append(groups[key], f)
	}
	for key, fs := range groups {
		agents := map[domain.AgentName]bool{}
		for _, f := range fs {
			agents[f.Recipient] = true
		}
		if len(agents) < 3 {
			continue
		}
		sent := map[string]bool{}
		for _, f := range fs {
			if IsConsoleSender(f.Sender) && f.SenderOwner == "" {
				continue
			}
			identity := (domain.TaskAlert{Sender: f.Sender, SenderOwner: f.SenderOwner}).DeliveryKey()
			if sent[identity] {
				continue
			}
			sent[identity] = true
			out = append(out, domain.TaskAlert{Key: "machine:" + key + ":" + identity, Kind: "machine", TaskID: f.ID, Project: f.Project, SenderOwner: f.SenderOwner, Sender: f.Sender, Recipient: f.Recipient, Agents: len(agents), At: now})
		}
	}
	return out
}
func (s *TaskMonitorService) Tick(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tasks, err := s.repo.ListTasks(ctx, domain.TaskFilter{})
	if err != nil {
		return err
	}
	now := s.clock.Now()
	failures, err := s.repo.TaskFailures(ctx, now.Add(-TaskFailureWindow))
	if err != nil {
		return err
	}
	if err = s.repo.StageTaskAlerts(ctx, taskAlertCandidates(tasks, failures, now, s.onlineWindow)); err != nil {
		return err
	}
	senders, err := s.repo.PendingTaskAlertSenders(ctx)
	if err != nil {
		return err
	}
	for _, to := range senders {
		note, err := s.repo.DeliverTaskAlerts(ctx, to, now, TaskAlertCooldown, s.onlineWindow, func(batch []domain.TaskAlert) *domain.Message {
			var b strings.Builder
			b.WriteString("Task alerts:\n")
			for _, a := range batch {
				switch a.Kind {
				case "stale":
					fmt.Fprintf(&b, "- %s to %s: no recipient activity for the configured stale threshold. Inspect: myco task board; follow up: myco message steer --id %s --body <update>.\n", a.TaskID, a.Recipient, a.TaskID)
				case "offline":
					fmt.Fprintf(&b, "- %s to %s: recipient heartbeat expired while the task was in progress. Inspect: myco task board; follow up after reconnecting: myco message steer --id %s --body <update>.\n", a.TaskID, a.Recipient, a.TaskID)
				case "machine":
					fmt.Fprintf(&b, "- A task you sent (%s to %s) failed in a same-machine incident involving at least %d agents within 10 minutes. This does not establish a root cause. Inspect: myco task board.\n", a.TaskID, a.Recipient, a.Agents)
				}
			}
			n, _ := domain.NewMessage(s.ids.New("msg"), AdminSender, batch[0].Sender, domain.TypeNote, b.String(), "", "", "", now)
			return n
		})
		if err != nil {
			return err
		}
		if note != nil {
			if s.bus != nil {
				s.bus.Publish(ctx, domain.MessageCreated{Message: note})
			}
			s.messages.queued(note.To)
		}
	}
	return nil
}
func (s *TaskMonitorService) Run(ctx context.Context) {
	tick := time.NewTicker(TaskMonitorTick)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if err := s.Tick(ctx); err != nil && ctx.Err() == nil {
				slog.Warn("task monitor evaluation failed")
			}
		}
	}
}
