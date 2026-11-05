package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/domain"
)

func (s *Server) agentDTO(a *domain.Agent) api.Agent {
	out := api.Agent{
		ID: a.ID, Name: string(a.Name), Description: a.Description,
		Capabilities: nonNil(a.Capabilities), Delivery: string(a.Delivery),
		AllowFrom: make([]string, 0, len(a.AllowFrom)),
		Online:    a.IsOnline(s.agents.Now(), s.agents.OnlineWindow()),
		CreatedAt: a.CreatedAt, LastSeenAt: timePtr(a.LastSeenAt),
	}
	for _, n := range a.AllowFrom {
		out.AllowFrom = append(out.AllowFrom, string(n))
	}
	return out
}

func messageDTO(m *domain.Message) api.Message {
	out := api.Message{
		ID: m.ID, From: string(m.From), To: string(m.To), ThreadID: m.ThreadID, ReplyTo: m.ReplyTo,
		Type: string(m.Type), Body: m.Body, Status: string(m.Status), Attempts: m.Attempts,
		Error: m.Error, IdempotencyKey: m.IdempotencyKey, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
	if m.Status == domain.StatusDelivered {
		out.LeaseUntil = timePtr(m.LeaseUntil)
	}
	return out
}

func messageList(msgs []*domain.Message) api.MessageList {
	out := api.MessageList{Messages: make([]api.Message, 0, len(msgs))}
	for _, m := range msgs {
		out.Messages = append(out.Messages, messageDTO(m))
	}
	return out
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

var statusByCode = map[domain.ErrorCode]int{
	domain.CodeInvalidArgument:  http.StatusBadRequest,
	domain.CodeNotFound:         http.StatusNotFound,
	domain.CodeConflict:         http.StatusConflict,
	domain.CodeInvalidState:     http.StatusConflict,
	domain.CodePermissionDenied: http.StatusForbidden,
	domain.CodeUnauthenticated:  http.StatusUnauthorized,
	domain.CodeInternal:         http.StatusInternalServerError,
}

func writeError(w http.ResponseWriter, err error) {
	var de *domain.Error
	if !errors.As(err, &de) {
		de = domain.Errf(domain.CodeInternal, "%v", err)
	}
	status, ok := statusByCode[de.Code]
	if !ok {
		status = http.StatusInternalServerError
	}
	writeJSON(w, status, api.Error{
		Code: string(de.Code), Message: de.Message, Suggestion: de.Suggestion,
		Retryable: de.Code == domain.CodeInternal,
	})
}
