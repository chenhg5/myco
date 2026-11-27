package app

import (
	"context"
	"github.com/myco-ai/myco/internal/domain"
)

func (o *observedMessages) coordination() (domain.MessageCoordinationRepository, error) {
	r, ok := o.MessageRepository.(domain.MessageCoordinationRepository)
	if !ok {
		return nil, domain.Errf(domain.CodeUnsupported, "message coordination is not supported by this store")
	}
	return r, nil
}
func (o *observedMessages) TaskMetadata(ctx context.Context, id string) (*domain.Message, error) {
	r, err := o.coordination()
	if err != nil {
		return nil, err
	}
	return r.TaskMetadata(ctx, id)
}
func (o *observedMessages) TaskChildren(ctx context.Context, parent *domain.Message, limit int) ([]*domain.Message, error) {
	r, err := o.coordination()
	if err != nil {
		return nil, err
	}
	return r.TaskChildren(ctx, parent, limit)
}
func (o *observedMessages) ReassignTask(ctx context.Context, id string, apply func(*domain.Message) (*domain.AuditEntry, error)) (*domain.Message, error) {
	r, err := o.coordination()
	if err != nil {
		return nil, err
	}
	m, err := r.ReassignTask(ctx, id, apply)
	if err == nil {
		o.bus.Publish(ctx, domain.MessageChanged{Message: m})
	}
	return m, err
}
