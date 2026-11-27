package app

import (
	"context"
	"github.com/myco-ai/myco/internal/domain"
	"time"
)

// Mark possible delivery before external I/O. A failed wake may already have
// exposed the body, so this flag is never cleared on error or recovery.
func preparePendingDisclosure(ctx context.Context, repo domain.MessageRepository, recipient domain.AgentName, pending []*domain.Message, now time.Time) ([]*domain.Message, error) {
	out := make([]*domain.Message, 0, len(pending))
	for _, old := range pending {
		eligible := false
		m, err := repo.Mutate(ctx, old.ID, func(cur *domain.Message) ([]*domain.Message, error) {
			if cur.To == recipient && cur.IsDue(now) {
				cur.EverDelivered = true
				eligible = true
			}
			return nil, nil
		})
		if err != nil {
			return nil, err
		}
		if eligible {
			out = append(out, m)
		}
	}
	return out, nil
}

func (s *MessageService) pendingDisclosure(ctx context.Context, recipient domain.AgentName) ([]*domain.Message, error) {
	now := s.clock.Now()
	pending, err := s.messages.ListDue(ctx, recipient, now, 20)
	if err != nil || len(pending) == 0 {
		return pending, err
	}
	return preparePendingDisclosure(ctx, s.messages, recipient, pending, now)
}
