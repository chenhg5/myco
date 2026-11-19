package httpapi

import (
	"context"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/domain"
)

// touchTask records that a task's recipient is working on it: at once in
// memory (what the App reads), and kept with the task (what the overview
// and a restarted hub go by).
func (s *Server) touchTask(id string) {
	s.activity.Store(id, time.Now().UTC())
	if s.messages != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		s.messages.TouchActive(ctx, id)
		cancel()
	}
}

func (s *Server) messageDTO(m *domain.Message) api.Message {
	out := messageDTO(m)
	if m.Type == domain.TypeTask && (m.Status == domain.StatusAcked || m.Status == domain.StatusHeld) {
		at := m.LastActive()
		if v, ok := s.activity.Load(m.ID); ok && v.(time.Time).After(at) {
			at = v.(time.Time)
		}
		out.ActiveAt = &at
	} else {
		s.activity.Delete(m.ID)
	}
	return out
}

func (s *Server) messageList(msgs []*domain.Message) api.MessageList {
	out := api.MessageList{Messages: make([]api.Message, 0, len(msgs))}
	for _, m := range msgs {
		out.Messages = append(out.Messages, s.messageDTO(m))
	}
	return out
}
