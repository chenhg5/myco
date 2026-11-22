package app

import (
	"context"
	"sync"
	"time"

	"github.com/chenhg5/myco/internal/domain"
)

// EventBus hands the hub's events (domain.Event) to whoever subscribed.
// Publishing is cheap and never fails: a subscriber that has slow work to
// do (storing, sending) queues it and returns.
type EventBus struct {
	mu   sync.RWMutex
	subs []func(context.Context, domain.Event)
}

// NewEventBus returns a bus with no subscribers.
func NewEventBus() *EventBus { return &EventBus{} }

// Subscribe adds fn; it is called for every event, in the publisher's
// goroutine, after what the event says is stored.
func (b *EventBus) Subscribe(fn func(context.Context, domain.Event)) {
	b.mu.Lock()
	b.subs = append(b.subs, fn)
	b.mu.Unlock()
}

// Publish tells every subscriber. A nil bus does nothing.
func (b *EventBus) Publish(ctx context.Context, e domain.Event) {
	if b == nil {
		return
	}
	b.mu.RLock()
	subs := b.subs
	b.mu.RUnlock()
	for _, fn := range subs {
		fn(ctx, e)
	}
}

// --- observers: repositories that publish what they stored ---
//
// Wrapping a repository at the composition root is how a feature learns
// what others do without their code calling it: the services keep using
// the repository as before.

// ObserveMessages publishes MessageCreated for every message stored through
// repo (sent, or made by a mutation: replies, progress, events, steers) and
// MessageChanged for every message a mutation changed.
func ObserveMessages(repo domain.MessageRepository, bus *EventBus) domain.MessageRepository {
	return &observedMessages{MessageRepository: repo, bus: bus}
}

type observedMessages struct {
	domain.MessageRepository
	bus *EventBus
}

func (o *observedMessages) Create(ctx context.Context, m *domain.Message) error {
	if err := o.MessageRepository.Create(ctx, m); err != nil {
		return err
	}
	o.bus.Publish(ctx, domain.MessageCreated{Message: m})
	return nil
}

func (o *observedMessages) CreateMany(ctx context.Context, ms []*domain.Message) error {
	if err := o.MessageRepository.CreateMany(ctx, ms); err != nil {
		return err
	}
	for _, m := range ms {
		o.bus.Publish(ctx, domain.MessageCreated{Message: m})
	}
	return nil
}

func (o *observedMessages) Mutate(ctx context.Context, id string, fn domain.MutateFunc) (*domain.Message, error) {
	var created []*domain.Message
	m, err := o.MessageRepository.Mutate(ctx, id, func(m *domain.Message) ([]*domain.Message, error) {
		c, err := fn(m)
		created = c
		return c, err
	})
	if err != nil {
		return m, err
	}
	o.bus.Publish(ctx, domain.MessageChanged{Message: m})
	for _, c := range created {
		o.bus.Publish(ctx, domain.MessageCreated{Message: c})
	}
	return m, nil
}

func (o *observedMessages) ClaimDue(ctx context.Context, recipient domain.AgentName, now time.Time, limit int, types []domain.MessageType, claim func(*domain.Message) bool) ([]*domain.Message, error) {
	ms, err := o.MessageRepository.ClaimDue(ctx, recipient, now, limit, types, claim)
	if err != nil {
		return ms, err
	}
	for _, m := range ms {
		o.bus.Publish(ctx, domain.MessageChanged{Message: m})
	}
	return ms, nil
}

// ObserveApprovals publishes ApprovalChanged for every approval stored.
func ObserveApprovals(repo domain.ApprovalRepository, bus *EventBus) domain.ApprovalRepository {
	return &observedApprovals{ApprovalRepository: repo, bus: bus}
}

type observedApprovals struct {
	domain.ApprovalRepository
	bus *EventBus
}

func (o *observedApprovals) Create(ctx context.Context, a *domain.Approval) error {
	if err := o.ApprovalRepository.Create(ctx, a); err != nil {
		return err
	}
	o.bus.Publish(ctx, domain.ApprovalChanged{Approval: a})
	return nil
}

func (o *observedApprovals) Save(ctx context.Context, a *domain.Approval) error {
	if err := o.ApprovalRepository.Save(ctx, a); err != nil {
		return err
	}
	o.bus.Publish(ctx, domain.ApprovalChanged{Approval: a})
	return nil
}

// ObserveWorkflows publishes StepChanged and RunChanged for every step and
// run stored.
func ObserveWorkflows(repo domain.WorkflowRepository, bus *EventBus) domain.WorkflowRepository {
	return &observedWorkflows{WorkflowRepository: repo, bus: bus}
}

type observedWorkflows struct {
	domain.WorkflowRepository
	bus *EventBus
}

func (o *observedWorkflows) SaveStep(ctx context.Context, s *domain.RunStep) error {
	if err := o.WorkflowRepository.SaveStep(ctx, s); err != nil {
		return err
	}
	o.bus.Publish(ctx, domain.StepChanged{Step: s})
	return nil
}

func (o *observedWorkflows) SaveRun(ctx context.Context, r *domain.WorkflowRun) error {
	if err := o.WorkflowRepository.SaveRun(ctx, r); err != nil {
		return err
	}
	o.bus.Publish(ctx, domain.RunChanged{Run: r})
	return nil
}
