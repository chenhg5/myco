package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/domain"
)

func (s *Server) agentDTO(a *domain.Agent) api.Agent {
	out := api.Agent{
		ID: a.ID, Name: string(a.Name), Description: a.Description,
		Capabilities: nonNil(a.Capabilities),
		Labels:       nonNil(a.Labels),
		Binding: api.Binding{Harness: a.Binding.Harness, SessionID: a.Binding.SessionID,
			Environment: a.Binding.Environment, Workdir: a.Binding.Workdir},
		Delivery: api.Delivery{Mode: a.Delivery.Mode, URL: redactURL(a.Delivery.URL),
			HasSecret: a.Delivery.Secret != "", Params: a.Delivery.Params},
		AllowFrom:  make([]string, 0, len(a.AllowFrom)),
		EnrolledBy: a.EnrolledBy,
		Online:     a.IsOnline(s.agents.Now(), s.agents.OnlineWindow()),
		CreatedAt:  a.CreatedAt, LastSeenAt: timePtr(a.LastSeenAt),
	}
	for _, n := range a.AllowFrom {
		out.AllowFrom = append(out.AllowFrom, string(n))
	}
	return out
}

func enrollmentDTO(e *domain.Enrollment, now time.Time) api.Enrollment {
	out := api.Enrollment{ID: e.ID, NamePrefix: e.NamePrefix, Harness: e.Harness, Capabilities: nonNil(e.Capabilities), Labels: nonNil(e.Labels),
		AllowFrom: make([]string, 0, len(e.AllowFrom)), Note: e.Note, MaxUses: e.MaxUses, Uses: e.Uses,
		ExpiresAt: timePtr(e.ExpiresAt), RevokedAt: timePtr(e.RevokedAt), CreatedAt: e.CreatedAt,
		Active: e.Usable(now, true) == nil}
	for _, n := range e.AllowFrom {
		out.AllowFrom = append(out.AllowFrom, string(n))
	}
	return out
}

func fileDTO(f *domain.File) api.File {
	return api.File{ID: f.ID, Name: f.Name, Size: f.Size, SHA256: f.SHA256, ContentType: f.ContentType,
		Owner: string(f.Owner), CreatedAt: f.CreatedAt, ExpiresAt: timePtr(f.ExpiresAt)}
}

// redactURL drops query strings (which may carry tokens) from wake URLs.
func redactURL(raw string) string {
	if i := strings.IndexByte(raw, '?'); i >= 0 {
		return raw[:i] + "?…"
	}
	return raw
}

func messageDTO(m *domain.Message) api.Message {
	out := api.Message{
		ID: m.ID, From: string(m.From), To: string(m.To), ThreadID: m.ThreadID, ReplyTo: m.ReplyTo,
		Type: string(m.Type), Body: m.Body, Status: string(m.Status), Attempts: m.Attempts,
		Error: m.Error, IdempotencyKey: m.IdempotencyKey, BroadcastID: m.BroadcastID, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
	if m.Status == domain.StatusDelivered {
		out.LeaseUntil = timePtr(m.LeaseUntil)
	}
	for _, a := range m.Attachments {
		out.Attachments = append(out.Attachments, api.Attachment{FileID: a.FileID, Name: a.Name, Size: a.Size, SHA256: a.SHA256, ContentType: a.ContentType})
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
