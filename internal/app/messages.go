package app

import (
	"context"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/domain"
)

// MessagePolicy tunes delivery semantics.
type MessagePolicy struct {
	// Lease is how long a pulled message stays invisible before redelivery
	// unless acknowledged.
	Lease time.Duration
	// MaxAttempts bounds redeliveries of unacknowledged messages.
	MaxAttempts int
	// MaxWait caps long-poll durations.
	MaxWait time.Duration
}

// DefaultMessagePolicy returns sensible defaults.
func DefaultMessagePolicy() MessagePolicy {
	return MessagePolicy{Lease: 5 * time.Minute, MaxAttempts: 5, MaxWait: 60 * time.Second}
}

// MessageService implements messaging use cases for an authenticated agent.
type MessageService struct {
	agents   domain.AgentRepository
	messages domain.MessageRepository
	notifier Notifier
	ids      IDGenerator
	clock    Clock
	policy   MessagePolicy
	observer DeliveryObserver
	files    domain.FileRepository // nil: attachments disabled
	aliases  domain.AliasRepository
	presence *Presence // nil: no session/runner arbitration
}

// SetPresence enables tracking of live `inbox wait` sessions, so runner
// pulls yield to them (see PullForRunner).
func (s *MessageService) SetPresence(p *Presence) { s.presence = p }

// Presence returns the presence tracker (nil when disabled).
func (s *MessageService) Presence() *Presence { return s.presence }

// SetAliases lets messages be addressed to aliases.
func (s *MessageService) SetAliases(repo domain.AliasRepository) { s.aliases = repo }

// SetFiles enables attachments backed by repo.
func (s *MessageService) SetFiles(repo domain.FileRepository) { s.files = repo }

// NewMessageService wires the messaging use cases.
func NewMessageService(agents domain.AgentRepository, messages domain.MessageRepository, n Notifier, ids IDGenerator, clock Clock, p MessagePolicy) *MessageService {
	return &MessageService{agents: agents, messages: messages, notifier: n, ids: ids, clock: clock, policy: p, observer: noopObserver{}}
}

// SetObserver registers the component told about inbox activity (the
// Dispatcher that wakes push agents).
func (s *MessageService) SetObserver(o DeliveryObserver) { s.observer = o }

// queued notifies long-pollers and the observer that agent has new mail.
func (s *MessageService) queued(agent domain.AgentName) {
	s.notifier.Publish(inboxTopic(string(agent)))
	s.observer.Queued(agent)
}

// SendInput describes a new message.
type SendInput struct {
	To             string
	Type           string
	Body           string
	ThreadID       string
	IdempotencyKey string
	// Attachments are file ids (from upload) the sender may share.
	Attachments []string
	DryRun      bool
}

// SendOutput reports the stored message; Duplicate is true when an earlier
// message with the same idempotency key was returned instead.
type SendOutput struct {
	Message   *domain.Message
	Duplicate bool
	// ViaAlias is the alias the sender used ("" if a real name).
	ViaAlias domain.AgentName
}

// Send queues a task or note from sender.
func (s *MessageService) Send(ctx context.Context, sender *domain.Agent, in SendInput) (*SendOutput, error) {
	return s.send(ctx, sender, in, false)
}

// send stores a message; asAdmin skips the recipient's allow_from (used only
// for MachineService.Spawn, sent as AdminSender).
func (s *MessageService) send(ctx context.Context, sender *domain.Agent, in SendInput, asAdmin bool) (*SendOutput, error) {
	to, err := domain.ParseAgentName(in.To)
	if err != nil {
		return nil, err
	}
	typ, err := domain.ParseSendableType(in.Type)
	if err != nil {
		return nil, err
	}
	if in.IdempotencyKey != "" {
		prev, err := s.messages.FindByIdempotencyKey(ctx, sender.Name, in.IdempotencyKey)
		if err == nil {
			return &SendOutput{Message: prev, Duplicate: true}, nil
		}
		if !domain.IsCode(err, domain.CodeNotFound) {
			return nil, err
		}
	}
	recipient, via, err := aliasResolver{agents: s.agents, aliases: s.aliases}.resolve(ctx, to)
	if err != nil {
		if domain.IsCode(err, domain.CodeNotFound) && !strings.Contains(err.Error(), "alias") {
			return nil, domain.Errf(domain.CodeNotFound, "agent %s not found", to).
				WithSuggestion("list agents with: myco agent list (aliases: myco agent alias list)")
		}
		return nil, err
	}
	to = recipient.Name // messages always store the real name
	if !asAdmin && !recipient.Accepts(sender.Name) {
		return nil, domain.Errf(domain.CodePermissionDenied, "agent %s does not accept messages from %s", to, sender.Name).
			WithSuggestion("ask the hub admin to add %s to %s's allow-from list", sender.Name, to)
	}
	if in.ThreadID != "" {
		if err := s.requireThreadParticipant(ctx, sender.Name, in.ThreadID); err != nil {
			return nil, err
		}
	}
	m, err := domain.NewMessage(s.ids.New("msg"), sender.Name, to, typ, in.Body, in.ThreadID, "", in.IdempotencyKey, s.clock.Now())
	if err != nil {
		return nil, err
	}
	if m.Attachments, err = resolveAttachments(ctx, s.files, s.clock, sender.Name, in.Attachments); err != nil {
		return nil, err
	}
	if in.DryRun {
		return &SendOutput{Message: m, ViaAlias: via}, nil
	}
	if s.files != nil {
		if err := grantAttachments(ctx, s.files, m.Attachments, to); err != nil {
			return nil, err
		}
	}
	if err := s.messages.Create(ctx, m); err != nil {
		if domain.IsCode(err, domain.CodeConflict) && in.IdempotencyKey != "" {
			prev, ferr := s.messages.FindByIdempotencyKey(ctx, sender.Name, in.IdempotencyKey)
			if ferr == nil {
				return &SendOutput{Message: prev, Duplicate: true}, nil
			}
		}
		return nil, err
	}
	s.queued(to)
	return &SendOutput{Message: m, ViaAlias: via}, nil
}

func (s *MessageService) requireThreadParticipant(ctx context.Context, agent domain.AgentName, threadID string) error {
	msgs, err := s.messages.ListThread(ctx, threadID)
	if err != nil {
		return err
	}
	for _, m := range msgs {
		if m.Involves(agent) {
			return nil
		}
	}
	return domain.Errf(domain.CodeNotFound, "thread %s not found", threadID).
		WithSuggestion("omit --thread to start a new thread")
}

// Pull leases up to limit due messages for agent, long-polling up to wait.
func (s *MessageService) Pull(ctx context.Context, agent *domain.Agent, limit int, waitFor time.Duration, types ...domain.MessageType) ([]*domain.Message, error) {
	return s.pull(ctx, agent, limit, waitFor, nil, types)
}

// PullForRunner is Pull for an agent's runner: while a session of the agent
// is live (keeps `inbox wait` running), the runner gets nothing, so it does
// not wake a session that is already receiving by itself. It still waits out
// waitFor so the runner's loop stays a slow long-poll.
func (s *MessageService) PullForRunner(ctx context.Context, agent *domain.Agent, limit int, waitFor time.Duration, types ...domain.MessageType) ([]*domain.Message, error) {
	if s.presence == nil {
		return s.Pull(ctx, agent, limit, waitFor, types...)
	}
	s.presence.RunnerPulled(agent.Name)
	return s.pull(ctx, agent, limit, waitFor, func() bool { return s.presence.Live(agent.Name) }, types)
}

// pull leases due messages; when yield reports true it claims nothing and
// sleeps until the deadline instead.
func (s *MessageService) pull(ctx context.Context, agent *domain.Agent, limit int, waitFor time.Duration, yield func() bool, types []domain.MessageType) ([]*domain.Message, error) {
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	s.observer.Pulled(agent.Name)
	waitFor = min(max(waitFor, 0), s.policy.MaxWait)
	deadline := s.clock.Now().Add(waitFor)
	for {
		// Subscribe before querying so a concurrent Send cannot be missed.
		if yield != nil && yield() {
			return nil, sleepUntil(ctx, deadline.Sub(s.clock.Now()))
		}
		ch, cancel := s.notifier.Subscribe(inboxTopic(string(agent.Name)))
		now := s.clock.Now()
		msgs, err := s.messages.ClaimDue(ctx, agent.Name, now, limit, types, func(m *domain.Message) bool {
			return m.Claim(now, s.policy.Lease, s.policy.MaxAttempts)
		})
		if err != nil || len(msgs) > 0 {
			cancel()
			return msgs, err
		}
		remaining := deadline.Sub(s.clock.Now())
		if remaining <= 0 {
			cancel()
			return nil, nil
		}
		fired := wait(ctx, ch, remaining)
		cancel()
		if !fired {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			// Timer elapsed: one final pass catches expired leases.
			if yield != nil && yield() {
				return nil, nil
			}
			now := s.clock.Now()
			return s.messages.ClaimDue(ctx, agent.Name, now, limit, types, func(m *domain.Message) bool {
				return m.Claim(now, s.policy.Lease, s.policy.MaxAttempts)
			})
		}
	}
}

// Transition names a recipient-side state change.
type Transition string

const (
	TransitionAck      Transition = "ack"
	TransitionHold     Transition = "hold"
	TransitionComplete Transition = "done"
)

// Transition applies ack/hold/done to a message addressed to agent.
func (s *MessageService) Transition(ctx context.Context, agent *domain.Agent, id string, t Transition, dryRun bool) (*domain.Message, error) {
	apply := func(m *domain.Message) error {
		now := s.clock.Now()
		switch t {
		case TransitionAck:
			return m.Ack(agent.Name, now)
		case TransitionHold:
			return m.Hold(agent.Name, now)
		case TransitionComplete:
			return m.Complete(agent.Name, now)
		}
		return domain.Errf(domain.CodeInvalidArgument, "unknown transition %q", t)
	}
	if dryRun {
		m, err := s.messages.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		return m, apply(m)
	}
	return s.messages.Mutate(ctx, id, func(m *domain.Message) ([]*domain.Message, error) {
		return nil, apply(m)
	})
}

// Release defers message id (addressed to agent) for redelivery after delay
// without counting a failed attempt.
func (s *MessageService) Release(ctx context.Context, agent *domain.Agent, id string, delay time.Duration, dryRun bool) (*domain.Message, error) {
	apply := func(m *domain.Message) error { return m.Release(agent.Name, delay, s.clock.Now()) }
	if dryRun {
		m, err := s.messages.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		return m, apply(m)
	}
	return s.messages.Mutate(ctx, id, func(m *domain.Message) ([]*domain.Message, error) { return nil, apply(m) })
}

// Reply answers message id on behalf of agent and returns the reply.
func (s *MessageService) Reply(ctx context.Context, agent *domain.Agent, id, body string, dryRun bool) (*domain.Message, error) {
	return s.ReplyWith(ctx, agent, id, body, nil, dryRun)
}

// ReplyWith answers message id with optional attachments (file ids).
func (s *MessageService) ReplyWith(ctx context.Context, agent *domain.Agent, id, body string, attachIDs []string, dryRun bool) (*domain.Message, error) {
	atts, err := resolveAttachments(ctx, s.files, s.clock, agent.Name, attachIDs)
	if err != nil {
		return nil, err
	}
	var reply *domain.Message
	fn := func(m *domain.Message) ([]*domain.Message, error) {
		r, err := m.Reply(agent.Name, s.ids.New("msg"), body, s.clock.Now())
		if err != nil {
			return nil, err
		}
		r.Attachments = atts
		reply = r
		return []*domain.Message{r}, nil
	}
	if err := s.mutateOrPreview(ctx, id, dryRun, fn); err != nil {
		return nil, err
	}
	// Grant outside the mutation's transaction (the store may serialize
	// connections). The reply is already stored, so a failure here only
	// delays the recipient's access; report it.
	if !dryRun && s.files != nil {
		if err := grantAttachments(ctx, s.files, atts, reply.To); err != nil {
			return nil, err
		}
	}
	if !dryRun {
		s.queued(reply.To)
		s.notifier.Publish(replyTopic(id))
	}
	return reply, nil
}

// Progress records an interim update on task id from its recipient and
// wakes anyone waiting for the task's reply.
func (s *MessageService) Progress(ctx context.Context, agent *domain.Agent, id, body string, dryRun bool) (*domain.Message, error) {
	var out *domain.Message
	err := s.mutateOrPreview(ctx, id, dryRun, func(m *domain.Message) ([]*domain.Message, error) {
		p, err := m.Progress(agent.Name, s.ids.New("msg"), body, s.clock.Now())
		if err != nil {
			return nil, err
		}
		out = p
		return []*domain.Message{p}, nil
	})
	if err != nil {
		return nil, err
	}
	if !dryRun {
		s.notifier.Publish(replyTopic(id))
	}
	return out, nil
}

// Steer sends a correction for open task id from its sender to the task's
// recipient.
func (s *MessageService) Steer(ctx context.Context, agent *domain.Agent, id, body string, dryRun bool) (*domain.Message, error) {
	var out *domain.Message
	err := s.mutateOrPreview(ctx, id, dryRun, func(m *domain.Message) ([]*domain.Message, error) {
		st, err := m.Steer(agent.Name, s.ids.New("msg"), body, s.clock.Now())
		if err != nil {
			return nil, err
		}
		out = st
		return []*domain.Message{st}, nil
	})
	if err != nil {
		return nil, err
	}
	if !dryRun {
		s.queued(out.To)
	}
	return out, nil
}

// WaitUpdate is WaitReply that also returns early with the task's progress
// updates created after since.
func (s *MessageService) WaitUpdate(ctx context.Context, agent *domain.Agent, id string, waitFor time.Duration, since time.Time) (*domain.Message, []*domain.Message, error) {
	orig, err := s.messages.Get(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if orig.From != agent.Name {
		return nil, nil, domain.Errf(domain.CodePermissionDenied, "only the sender of %s can wait for its reply", id)
	}
	waitFor = min(max(waitFor, 0), s.policy.MaxWait)
	deadline := s.clock.Now().Add(waitFor)
	for {
		ch, cancel := s.notifier.Subscribe(replyTopic(id))
		progress, err := s.progressSince(ctx, orig, since)
		if err != nil {
			cancel()
			return nil, nil, err
		}
		r, err := s.messages.FindReply(ctx, id)
		switch {
		case err == nil:
			cancel()
			r, err = s.consume(ctx, agent, r)
			return r, progress, err
		case !domain.IsCode(err, domain.CodeNotFound):
			cancel()
			return nil, nil, err
		case len(progress) > 0:
			cancel()
			return nil, progress, nil
		}
		remaining := deadline.Sub(s.clock.Now())
		if remaining <= 0 {
			cancel()
			return nil, nil, nil
		}
		fired := wait(ctx, ch, remaining)
		cancel()
		if !fired && ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		if !fired {
			deadline = s.clock.Now()
		}
	}
}

func (s *MessageService) progressSince(ctx context.Context, task *domain.Message, since time.Time) ([]*domain.Message, error) {
	all, err := s.messages.ListThread(ctx, task.ThreadID)
	if err != nil {
		return nil, err
	}
	var out []*domain.Message
	for _, m := range all {
		if m.Type == domain.TypeProgress && m.ReplyTo == task.ID && m.CreatedAt.After(since) {
			out = append(out, m)
		}
	}
	return out, nil
}

// ReplyOnBehalf answers message id as its recipient. It is used by ingress
// adapters (e.g. a GitHub comment written by Codex cloud) that the hub has
// already authenticated; source is recorded in the reply body header.
func (s *MessageService) ReplyOnBehalf(ctx context.Context, id, body, source string) (*domain.Message, error) {
	m, err := s.messages.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	recipient, err := s.agents.Get(ctx, m.To)
	if err != nil {
		return nil, err
	}
	if source != "" {
		body = "[via " + source + "]\n" + body
	}
	return s.Reply(ctx, recipient, id, body, false)
}

// Fail closes message id as failed; the sender is notified for tasks.
func (s *MessageService) Fail(ctx context.Context, agent *domain.Agent, id, reason string, dryRun bool) (*domain.Message, error) {
	var event *domain.Message
	fn := func(m *domain.Message) ([]*domain.Message, error) {
		ev, err := m.Fail(agent.Name, s.ids.New("msg"), reason, s.clock.Now())
		if err != nil || ev == nil {
			return nil, err
		}
		event = ev
		return []*domain.Message{ev}, nil
	}
	if dryRun {
		m, err := s.messages.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		_, err = fn(m)
		return m, err
	}
	m, err := s.messages.Mutate(ctx, id, fn)
	if err != nil {
		return nil, err
	}
	if event != nil {
		s.queued(event.To)
		s.notifier.Publish(replyTopic(id))
	}
	return m, nil
}

func (s *MessageService) mutateOrPreview(ctx context.Context, id string, dryRun bool, fn domain.MutateFunc) error {
	if dryRun {
		m, err := s.messages.Get(ctx, id)
		if err != nil {
			return err
		}
		_, err = fn(m)
		return err
	}
	_, err := s.messages.Mutate(ctx, id, fn)
	return err
}

// Get returns a message visible to agent.
func (s *MessageService) Get(ctx context.Context, agent *domain.Agent, id string) (*domain.Message, error) {
	m, err := s.messages.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !m.Involves(agent.Name) {
		return nil, domain.Errf(domain.CodeNotFound, "message %s not found", id)
	}
	return m, nil
}

// Thread returns all messages of a thread agent participates in.
func (s *MessageService) Thread(ctx context.Context, agent *domain.Agent, threadID string) ([]*domain.Message, error) {
	if err := s.requireThreadParticipant(ctx, agent.Name, threadID); err != nil {
		return nil, err
	}
	return s.messages.ListThread(ctx, threadID)
}

// ThreadAll returns a thread without participant checks (admin views).
func (s *MessageService) ThreadAll(ctx context.Context, threadID string) ([]*domain.Message, error) {
	msgs, err := s.messages.ListThread(ctx, threadID)
	if err == nil && len(msgs) == 0 {
		err = domain.Errf(domain.CodeNotFound, "thread %s not found", threadID)
	}
	return msgs, err
}

// WaitReply blocks up to waitFor for the answer to a message agent sent.
// A returned reply is marked done so it is not delivered again via pull.
// It returns (nil, nil) on timeout.
func (s *MessageService) WaitReply(ctx context.Context, agent *domain.Agent, id string, waitFor time.Duration) (*domain.Message, error) {
	orig, err := s.messages.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if orig.From != agent.Name {
		return nil, domain.Errf(domain.CodePermissionDenied, "only the sender of %s can wait for its reply", id)
	}
	waitFor = min(max(waitFor, 0), s.policy.MaxWait)
	deadline := s.clock.Now().Add(waitFor)
	for {
		ch, cancel := s.notifier.Subscribe(replyTopic(id))
		r, err := s.messages.FindReply(ctx, id)
		if err == nil {
			cancel()
			return s.consume(ctx, agent, r)
		}
		if !domain.IsCode(err, domain.CodeNotFound) {
			cancel()
			return nil, err
		}
		remaining := deadline.Sub(s.clock.Now())
		if remaining <= 0 {
			cancel()
			return nil, nil
		}
		fired := wait(ctx, ch, remaining)
		cancel()
		if !fired && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if !fired {
			deadline = s.clock.Now() // one last lookup, then give up
		}
	}
}

// consume marks a reply as done so pull will not redeliver it.
func (s *MessageService) consume(ctx context.Context, agent *domain.Agent, r *domain.Message) (*domain.Message, error) {
	if r.Status.Terminal() {
		return r, nil
	}
	return s.messages.Mutate(ctx, r.ID, func(m *domain.Message) ([]*domain.Message, error) {
		if m.Status.Terminal() {
			return nil, nil
		}
		if m.Status == domain.StatusQueued {
			m.Claim(s.clock.Now(), 0, 0)
		}
		return nil, m.Complete(agent.Name, s.clock.Now())
	})
}

// WaitPending blocks up to waitFor until agent has deliverable messages and
// returns them without claiming (for "sleep until there is mail" loops).
func (s *MessageService) WaitPending(ctx context.Context, agent *domain.Agent, waitFor time.Duration) ([]*domain.Message, error) {
	waitFor = min(max(waitFor, 0), s.policy.MaxWait)
	if s.presence != nil {
		defer s.presence.Listen(agent.Name)()
	}
	deadline := s.clock.Now().Add(waitFor)
	for {
		ch, cancel := s.notifier.Subscribe(inboxTopic(string(agent.Name)))
		msgs, err := s.messages.ListDue(ctx, agent.Name, s.clock.Now(), 20)
		if err != nil || len(msgs) > 0 {
			cancel()
			return msgs, err
		}
		remaining := deadline.Sub(s.clock.Now())
		if remaining <= 0 {
			cancel()
			return nil, nil
		}
		fired := wait(ctx, ch, remaining)
		cancel()
		if !fired {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return s.messages.ListDue(ctx, agent.Name, s.clock.Now(), 20)
		}
	}
}
