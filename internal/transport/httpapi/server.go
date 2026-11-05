// Package httpapi exposes the app use cases over HTTP/JSON.
package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/domain"
)

// Server is the hub's HTTP adapter.
type Server struct {
	enroll     *app.EnrollmentService
	agents     *app.AgentService
	messages   *app.MessageService
	adminToken string
	log        *slog.Logger
	mux        *http.ServeMux
	ingress    map[string]Ingress
}

// Inbound is a reply arriving from an external system.
type Inbound struct {
	MessageID string
	Body      string
	Source    string
}

// Ingress authenticates and parses callbacks from an external system
// (mounted at POST /v1/ingress/{kind}).
type Ingress interface {
	Kind() string
	Parse(r *http.Request, body []byte) ([]Inbound, error)
}

// New builds the HTTP handler. adminToken guards agent administration.
func New(agents *app.AgentService, messages *app.MessageService, enroll *app.EnrollmentService, adminToken string, log *slog.Logger, ingress ...Ingress) *Server {
	s := &Server{enroll: enroll, agents: agents, messages: messages, adminToken: adminToken, log: log, mux: http.NewServeMux(), ingress: map[string]Ingress{}}
	for _, in := range ingress {
		s.ingress[in.Kind()] = in
	}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, domain.MaxBodyBytes+64*1024)
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	m := s.mux
	m.HandleFunc("GET /v1/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, api.Health{OK: true, Version: api.Version})
	})

	// Administration (admin token).
	m.HandleFunc("POST /v1/agents", s.admin(s.register))
	m.HandleFunc("DELETE /v1/agents/{name}", s.admin(s.removeAgent))
	m.HandleFunc("POST /v1/enrollments", s.admin(s.createEnrollment))
	m.HandleFunc("GET /v1/enrollments", s.admin(s.listEnrollments))
	m.HandleFunc("DELETE /v1/enrollments/{id}", s.admin(s.revokeEnrollment))

	// Self-service join with an enrollment token.
	m.HandleFunc("POST /v1/enroll", s.enrollSession)

	// Discovery (admin or any agent).
	m.HandleFunc("GET /v1/agents", s.adminOrAgent(s.listAgents))
	m.HandleFunc("GET /v1/agents/{name}", s.adminOrAgent(s.getAgent))

	// Agent-scoped operations.
	m.HandleFunc("GET /v1/me", s.agent(s.me))
	m.HandleFunc("PUT /v1/me/binding", s.agent(s.bind))
	m.HandleFunc("GET /v1/inbox/wait", s.agent(s.waitPending))
	m.HandleFunc("POST /v1/ingress/{kind}", s.handleIngress)
	m.HandleFunc("POST /v1/messages", s.agent(s.send))
	m.HandleFunc("GET /v1/messages/{id}", s.agent(s.getMessage))
	m.HandleFunc("POST /v1/messages/{id}/ack", s.agent(s.transition(app.TransitionAck)))
	m.HandleFunc("POST /v1/messages/{id}/hold", s.agent(s.transition(app.TransitionHold)))
	m.HandleFunc("POST /v1/messages/{id}/done", s.agent(s.transition(app.TransitionComplete)))
	m.HandleFunc("POST /v1/messages/{id}/reply", s.agent(s.reply))
	m.HandleFunc("POST /v1/messages/{id}/fail", s.agent(s.fail))
	m.HandleFunc("GET /v1/messages/{id}/reply", s.agent(s.waitReply))
	m.HandleFunc("POST /v1/inbox/pull", s.agent(s.pull))
	m.HandleFunc("GET /v1/threads/{id}", s.agent(s.thread))

	m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, domain.Errf(domain.CodeNotFound, "no route for %s %s", r.Method, r.URL.Path).
			WithSuggestion("see the API reference in docs/api.md"))
	})
}

// ---- auth middleware ----

type ctxKey struct{}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if t, ok := strings.CutPrefix(h, "Bearer "); ok {
		return strings.TrimSpace(t)
	}
	return ""
}

func (s *Server) isAdmin(r *http.Request) bool {
	t := bearer(r)
	return s.adminToken != "" && t != "" && subtle.ConstantTimeCompare([]byte(t), []byte(s.adminToken)) == 1
}

func (s *Server) admin(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.isAdmin(r) {
			writeError(w, domain.Errf(domain.CodePermissionDenied, "admin token required").
				WithSuggestion("set MYCO_ADMIN_TOKEN to the hub's admin token"))
			return
		}
		h(w, r)
	}
}

func (s *Server) agent(h func(http.ResponseWriter, *http.Request, *domain.Agent)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a, err := s.agents.Authenticate(r.Context(), bearer(r))
		if err != nil {
			writeError(w, err)
			return
		}
		h(w, r, a)
	}
}

func (s *Server) adminOrAgent(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.isAdmin(r) {
			h(w, r)
			return
		}
		s.agent(func(w http.ResponseWriter, r *http.Request, _ *domain.Agent) { h(w, r) })(w, r)
	}
}

// ---- handlers ----

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var req api.RegisterRequest
	if !decode(w, r, &req) {
		return
	}
	dry := dryRun(r)
	out, err := s.agents.Register(r.Context(), app.RegisterInput{
		Name: req.Name, Description: req.Description, Capabilities: req.Capabilities,
		AllowFrom: req.AllowFrom, Binding: bindingFromAPI(req.Binding), RotateToken: req.RotateToken, DryRun: dry,
		Delivery: domain.Delivery{Mode: req.Delivery.Mode, URL: req.Delivery.URL, Secret: req.Delivery.Secret, Params: req.Delivery.Params},
	})
	if err != nil {
		writeError(w, err)
		return
	}
	status := http.StatusOK
	if out.Created && !dry {
		status = http.StatusCreated
	}
	writeJSON(w, status, api.RegisterResponse{Agent: s.agentDTO(out.Agent), Token: out.Token, Created: out.Created, DryRun: dry})
}

func (s *Server) removeAgent(w http.ResponseWriter, r *http.Request) {
	dry := dryRun(r)
	a, err := s.agents.Remove(r.Context(), r.PathValue("name"), dry)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"agent": s.agentDTO(a), "removed": !dry, "dry_run": dry})
}

func (s *Server) listAgents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	list, err := s.agents.List(r.Context(), app.AgentFilter{Capability: q.Get("capability"), OnlineOnly: q.Get("online") == "true"})
	if err != nil {
		writeError(w, err)
		return
	}
	out := api.AgentList{Agents: make([]api.Agent, 0, len(list))}
	for _, a := range list {
		out.Agents = append(out.Agents, s.agentDTO(a))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getAgent(w http.ResponseWriter, r *http.Request) {
	a, err := s.agents.Get(r.Context(), r.PathValue("name"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.agentDTO(a))
}

func (s *Server) me(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	writeJSON(w, http.StatusOK, s.agentDTO(a))
}

func (s *Server) bind(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	var req api.BindRequest
	if !decode(w, r, &req) {
		return
	}
	a, err := s.agents.Bind(r.Context(), a, bindingFromAPI(req))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.agentDTO(a))
}

func (s *Server) waitPending(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	msgs, err := s.messages.WaitPending(r.Context(), a, waitParam(r))
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, api.PendingResponse{Pending: messageList(msgs).Messages, TimedOut: len(msgs) == 0})
}

func (s *Server) handleIngress(w http.ResponseWriter, r *http.Request) {
	in, ok := s.ingress[r.PathValue("kind")]
	if !ok {
		writeError(w, domain.Errf(domain.CodeNotFound, "no ingress %q on this hub", r.PathValue("kind")))
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, domain.Errf(domain.CodeInvalidArgument, "read body: %v", err))
		return
	}
	replies, err := in.Parse(r, body)
	if err != nil {
		writeError(w, err)
		return
	}
	accepted := 0
	for _, rep := range replies {
		if _, err := s.messages.ReplyOnBehalf(r.Context(), rep.MessageID, rep.Body, rep.Source); err != nil {
			s.log.Info("ingress reply ignored", "kind", in.Kind(), "message_id", rep.MessageID, "err", err)
			continue
		}
		accepted++
	}
	writeJSON(w, http.StatusOK, map[string]int{"received": len(replies), "accepted": accepted})
}

func (s *Server) createEnrollment(w http.ResponseWriter, r *http.Request) {
	var req api.CreateEnrollmentRequest
	if !decode(w, r, &req) {
		return
	}
	dry := dryRun(r)
	e, tok, err := s.enroll.Create(r.Context(), app.CreateEnrollmentInput{
		NamePrefix: req.NamePrefix, Harness: req.Harness, AllowFrom: req.AllowFrom, Capabilities: req.Capabilities,
		Note: req.Note, MaxUses: req.MaxUses, TTL: time.Duration(req.TTLSeconds) * time.Second, DryRun: dry,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	status := http.StatusCreated
	if dry {
		status = http.StatusOK
	}
	writeJSON(w, status, api.CreateEnrollmentResponse{Enrollment: enrollmentDTO(e, s.agents.Now()), Token: tok, DryRun: dry})
}

func (s *Server) listEnrollments(w http.ResponseWriter, r *http.Request) {
	list, err := s.enroll.List(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	out := api.EnrollmentList{Enrollments: make([]api.Enrollment, 0, len(list))}
	for _, e := range list {
		out.Enrollments = append(out.Enrollments, enrollmentDTO(e, s.agents.Now()))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) revokeEnrollment(w http.ResponseWriter, r *http.Request) {
	dry := dryRun(r)
	e, err := s.enroll.Revoke(r.Context(), r.PathValue("id"), dry)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enrollment": enrollmentDTO(e, s.agents.Now()), "revoked": !dry, "dry_run": dry})
}

func (s *Server) enrollSession(w http.ResponseWriter, r *http.Request) {
	var req api.EnrollRequest
	if !decode(w, r, &req) {
		return
	}
	dry := dryRun(r)
	out, err := s.enroll.Enroll(r.Context(), bearer(r), app.EnrollInput{
		Name: req.Name, Description: req.Description, Capabilities: req.Capabilities,
		Binding: bindingFromAPI(req.Binding), DryRun: dry,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	status := http.StatusOK
	if out.Created && !dry {
		status = http.StatusCreated
	}
	writeJSON(w, status, api.RegisterResponse{Agent: s.agentDTO(out.Agent), Token: out.Token, Created: out.Created, DryRun: dry})
}

func bindingFromAPI(b api.Binding) domain.Binding {
	return domain.Binding{Harness: b.Harness, SessionID: b.SessionID, Environment: b.Environment, Workdir: b.Workdir}
}

func (s *Server) send(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	var req api.SendRequest
	if !decode(w, r, &req) {
		return
	}
	dry := dryRun(r)
	out, err := s.messages.Send(r.Context(), a, app.SendInput{
		To: req.To, Type: req.Type, Body: req.Body, ThreadID: req.ThreadID, IdempotencyKey: req.IdempotencyKey, DryRun: dry,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	status := http.StatusCreated
	if out.Duplicate || dry {
		status = http.StatusOK
	}
	writeJSON(w, status, api.SendResponse{Message: messageDTO(out.Message), Duplicate: out.Duplicate, DryRun: dry})
}

func (s *Server) getMessage(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	m, err := s.messages.Get(r.Context(), a, r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, messageDTO(m))
}

func (s *Server) transition(t app.Transition) func(http.ResponseWriter, *http.Request, *domain.Agent) {
	return func(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
		dry := dryRun(r)
		m, err := s.messages.Transition(r.Context(), a, r.PathValue("id"), t, dry)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, api.MessageResult{Message: messageDTO(m), DryRun: dry})
	}
}

func (s *Server) reply(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	var req api.ReplyRequest
	if !decode(w, r, &req) {
		return
	}
	dry := dryRun(r)
	m, err := s.messages.Reply(r.Context(), a, r.PathValue("id"), req.Body, dry)
	if err != nil {
		writeError(w, err)
		return
	}
	status := http.StatusCreated
	if dry {
		status = http.StatusOK
	}
	writeJSON(w, status, api.MessageResult{Message: messageDTO(m), DryRun: dry})
}

func (s *Server) fail(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	var req api.FailRequest
	if !decode(w, r, &req) {
		return
	}
	dry := dryRun(r)
	m, err := s.messages.Fail(r.Context(), a, r.PathValue("id"), req.Reason, dry)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, api.MessageResult{Message: messageDTO(m), DryRun: dry})
}

func (s *Server) waitReply(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	m, err := s.messages.WaitReply(r.Context(), a, r.PathValue("id"), waitParam(r))
	if err != nil {
		writeError(w, err)
		return
	}
	if m == nil {
		writeJSON(w, http.StatusOK, api.WaitResponse{TimedOut: true})
		return
	}
	dto := messageDTO(m)
	writeJSON(w, http.StatusOK, api.WaitResponse{Reply: &dto})
}

func (s *Server) pull(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	var req api.PullRequest
	if r.ContentLength != 0 && !decode(w, r, &req) {
		return
	}
	msgs, err := s.messages.Pull(r.Context(), a, req.Limit, time.Duration(req.WaitSeconds)*time.Second)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return // client went away
		}
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, messageList(msgs))
}

func (s *Server) thread(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	msgs, err := s.messages.Thread(r.Context(), a, r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, messageList(msgs))
}

// ---- helpers ----

func dryRun(r *http.Request) bool {
	v, _ := strconv.ParseBool(r.URL.Query().Get("dry_run"))
	return v
}

func waitParam(r *http.Request) time.Duration {
	n, _ := strconv.Atoi(r.URL.Query().Get("wait_seconds"))
	return time.Duration(n) * time.Second
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, domain.Errf(domain.CodeInvalidArgument, "invalid JSON body: %v", err))
		return false
	}
	return true
}
