package httpapi

import (
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/domain"
)

func (s *Server) touchTask(id string) { s.activity.Store(id, time.Now().UTC()) }

func (s *Server) messageDTO(m *domain.Message) api.Message {
	out := messageDTO(m)
	if m.Type == domain.TypeTask && (m.Status == domain.StatusAcked || m.Status == domain.StatusHeld) {
		at := m.UpdatedAt
		if v, ok := s.activity.Load(m.ID); ok {
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
