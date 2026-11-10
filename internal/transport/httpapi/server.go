// Package httpapi exposes the app use cases over HTTP/JSON.
package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/domain"
	"github.com/chenhg5/myco/internal/osutil"
	"github.com/chenhg5/myco/internal/release"
)

// Server is the hub's HTTP adapter.
type Server struct {
	enroll     *app.EnrollmentService
	agents     *app.AgentService
	messages   *app.MessageService
	adminToken string
	log        *slog.Logger
	publicURL  atomic.Value // string
	// lan: the hub listens on every interface without a public URL, so
	// links made over localhost use this machine's LAN address instead.
	lan      atomic.Bool
	e2e      *app.E2EService
	version  string        // this hub's myco build
	latest   func() string // newest known release ("" = unknown)
	mux      *http.ServeMux
	ingress  map[string]Ingress
	files    *app.FileService
	overview *app.OverviewService
	console  *app.ConsoleService
	machines *app.MachineService
	members  *app.MemberService
	audit    *app.AuditService
	namesMu  sync.Mutex
	names    map[string]string // member id -> name (see ownerName)
	namesAt  time.Time
}

// SetMachines enables machine reporting, /v1/machines and machines in the
// admin overview.
func (s *Server) SetMachines(m *app.MachineService) { s.machines = m }

// SetConsole enables console tokens (per-device admin credentials) and the
// /v1/console/tokens endpoints.
func (s *Server) SetConsole(c *app.ConsoleService) { s.console = c }

// SetOverview enables GET /v1/overview.
func (s *Server) SetOverview(o *app.OverviewService) { s.overview = o }

// SetFiles enables the file endpoints.
func (s *Server) SetFiles(f *app.FileService) { s.files = f }

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
	limit := int64(domain.MaxBodyBytes + 64*1024)
	if r.Method == http.MethodPost && r.URL.Path == "/v1/files" && s.files != nil {
		limit = s.files.Policy().MaxSize + 1 // FileService reports "too large" itself
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	m := s.mux
	m.HandleFunc("GET /v1/health", func(w http.ResponseWriter, r *http.Request) {
		h := api.Health{OK: true, Version: api.Version, PublicURL: s.PublicURL(), ServerVersion: s.version}
		if s.latest != nil {
			h.LatestVersion = s.latest()
			h.UpdateAvailable = release.Newer(h.LatestVersion, s.version)
		}
		writeJSON(w, http.StatusOK, h)
	})

	// Administration (admin token).
	m.HandleFunc("POST /v1/agents", s.admin(s.register))
	m.HandleFunc("DELETE /v1/agents/{name}", s.admin(s.removeAgent))
	m.HandleFunc("POST /v1/agents/cleanup", s.admin(s.cleanupAgents))
	s.routesE2E(m)
	m.HandleFunc("POST /v1/agents/{name}/kick", s.admin(s.kickAgent))
	m.HandleFunc("GET /v1/machines", s.admin(s.listMachines))
	m.HandleFunc("PATCH /v1/machines/{id}", s.admin(s.renameMachine))
	m.HandleFunc("POST /v1/machines/{id}/spawn", s.admin(s.spawnOnMachine))
	m.HandleFunc("PUT /v1/me/machine", s.agent(s.reportMachine))
	m.HandleFunc("PUT /v1/me/runtime", s.agent(s.reportRuntime))
	// Console tokens: only the hub's own admin token manages them, so a
	// leaked console token cannot mint more.
	m.HandleFunc("POST /v1/console/tokens", s.masterAdmin(s.createConsoleToken))
	m.HandleFunc("GET /v1/console/tokens", s.masterAdmin(s.listConsoleTokens))
	// The administrator talking to agents from the console app.
	m.HandleFunc("POST /v1/console/messages", s.admin(s.consoleSend))
	m.HandleFunc("POST /v1/console/messages/{id}/steer", s.admin(s.consoleSteer))
	m.HandleFunc("POST /v1/console/messages/{id}/dismiss", s.admin(s.consoleDismiss))
	m.HandleFunc("GET /v1/console/conversations", s.admin(s.consoleConversations))
	m.HandleFunc("GET /v1/console/agents/{name}/peers", s.admin(s.consolePeers))
	m.HandleFunc("GET /v1/console/exchanges", s.admin(s.consoleExchanges))
	m.HandleFunc("GET /v1/console/recent", s.admin(s.consoleRecent))
	m.HandleFunc("DELETE /v1/console/tokens/{id}", s.masterAdmin(s.revokeConsoleToken))
	// Members (people) and the audit log. Console tokens act as their member;
	// handlers check what that member may do.
	m.HandleFunc("GET /v1/console/me", s.admin(s.whoami))
	// Devices: a one-time link adds a device; each device has its own token.
	m.HandleFunc("POST /v1/console/pair", s.pair)
	m.HandleFunc("POST /v1/console/me/pairings", s.admin(s.createPairing))
	m.HandleFunc("GET /v1/console/me/devices", s.admin(s.myDevices))
	m.HandleFunc("DELETE /v1/console/me/devices/{id}", s.admin(s.removeMyDevice))
	m.HandleFunc("GET /v1/members", s.admin(s.listMembers))
	m.HandleFunc("POST /v1/members", s.admin(s.inviteMember))
	m.HandleFunc("PATCH /v1/members/{member}", s.admin(s.updateMember))
	m.HandleFunc("DELETE /v1/members/{member}", s.admin(s.removeMember))
	m.HandleFunc("PUT /v1/agents/{name}/owner", s.admin(s.setAgentOwner))
	m.HandleFunc("PUT /v1/machines/{id}/owner", s.admin(s.setMachineOwner))
	m.HandleFunc("GET /v1/audit", s.admin(s.listAudit))
	m.HandleFunc("POST /v1/enrollments", s.admin(s.createEnrollment))
	m.HandleFunc("GET /v1/enrollments", s.admin(s.listEnrollments))
	m.HandleFunc("DELETE /v1/enrollments/{id}", s.admin(s.revokeEnrollment))

	// Self-service join with an enrollment token.
	m.HandleFunc("POST /v1/enroll", s.enrollSession)
	// A machine joins with a link; /j/{token} serves the link's script/page.
	m.HandleFunc("POST /v1/join", s.join)
	m.HandleFunc("GET /j/{token}", s.joinPage)
	// An app link opened in a browser: say how to use it (never echoes
	// whether the token is valid).
	m.HandleFunc("GET /c/{token}", s.appLinkPage)

	// Discovery (admin or any agent).
	m.HandleFunc("GET /v1/agents", s.adminOrAgent(s.listAgents))
	m.HandleFunc("GET /v1/agents/{name}", s.adminOrAgent(s.getAgent))
	m.HandleFunc("GET /v1/overview", s.handleOverview)
	m.HandleFunc("PUT /v1/agents/{name}/labels", s.admin(s.adminLabels))
	m.HandleFunc("PUT /v1/agents/{name}/skills", s.admin(s.adminSkills))
	m.HandleFunc("PUT /v1/agents/{name}/visibility", s.admin(s.setVisibility))
	m.HandleFunc("GET /v1/agents/{name}/history", s.admin(s.agentHistory))
	m.HandleFunc("PUT /v1/agents/{name}/profile", s.admin(s.adminProfile))
	m.HandleFunc("GET /v1/aliases", s.adminOrAgent(s.listAliases))
	m.HandleFunc("PUT /v1/aliases/{alias}", s.admin(s.setAlias))
	m.HandleFunc("DELETE /v1/aliases/{alias}", s.admin(s.removeAlias))

	// Agent-scoped operations.
	m.HandleFunc("GET /v1/me", s.agent(s.me))
	m.HandleFunc("PUT /v1/me/binding", s.agent(s.bind))
	m.HandleFunc("PUT /v1/me/labels", s.agent(s.myLabels))
	m.HandleFunc("PUT /v1/me/skills", s.agent(s.mySkills))
	m.HandleFunc("PUT /v1/me/profile", s.agent(s.myProfile))
	m.HandleFunc("PUT /v1/me/status", s.agent(s.myStatus))
	m.HandleFunc("POST /v1/broadcasts", s.agent(s.broadcast))
	m.HandleFunc("POST /v1/files", s.agent(s.uploadFile))
	m.HandleFunc("GET /v1/files/{id}", s.agent(s.downloadFile))
	m.HandleFunc("GET /v1/files/{id}/meta", s.agent(s.fileMeta))
	m.HandleFunc("GET /v1/broadcasts/{id}", s.agent(s.broadcastStatus))
	m.HandleFunc("GET /v1/inbox/wait", s.agent(s.waitPending))
	m.HandleFunc("POST /v1/ingress/{kind}", s.handleIngress)
	m.HandleFunc("POST /v1/messages", s.agent(s.send))
	m.HandleFunc("GET /v1/messages/{id}", s.agent(s.getMessage))
	m.HandleFunc("POST /v1/messages/{id}/ack", s.agent(s.transition(app.TransitionAck)))
	m.HandleFunc("POST /v1/messages/{id}/hold", s.agent(s.transition(app.TransitionHold)))
	m.HandleFunc("POST /v1/messages/{id}/done", s.agent(s.transition(app.TransitionComplete)))
	m.HandleFunc("POST /v1/messages/{id}/reply", s.agent(s.reply))
	m.HandleFunc("POST /v1/messages/{id}/progress", s.agent(s.followUp("progress")))
	m.HandleFunc("POST /v1/messages/{id}/steer", s.agent(s.followUp("steer")))
	m.HandleFunc("POST /v1/messages/{id}/fail", s.agent(s.fail))
	m.HandleFunc("POST /v1/messages/{id}/release", s.agent(s.release))
	m.HandleFunc("GET /v1/messages/{id}/reply", s.agent(s.waitReply))
	m.HandleFunc("POST /v1/inbox/pull", s.agent(s.pull))
	m.HandleFunc("GET /v1/threads/{id}", s.threadAny)

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

// isAdmin accepts the hub's admin token or an active console token.
func (s *Server) isAdmin(r *http.Request) bool {
	if s.isMasterAdmin(r) {
		return true
	}
	if s.console == nil {
		return false
	}
	_, ok := s.console.Authenticate(r.Context(), bearer(r))
	return ok
}

// isMasterAdmin accepts only the hub's own admin token.
func (s *Server) isMasterAdmin(r *http.Request) bool {
	t := bearer(r)
	return s.adminToken != "" && t != "" && subtle.ConstantTimeCompare([]byte(t), []byte(s.adminToken)) == 1
}

// masterAdmin allows only the hub's admin token (not console tokens).
func (s *Server) masterAdmin(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.console == nil {
			writeError(w, domain.Errf(domain.CodeNotFound, "console tokens are not enabled on this hub"))
			return
		}
		if s.isMasterAdmin(r) {
			h(w, r)
			return
		}
		const hint = "run it on the hub machine, where the admin token is read from hub.env"
		if s.isAdmin(r) {
			writeError(w, domain.Errf(domain.CodePermissionDenied, "console tokens are managed with the hub's admin token only").WithSuggestion(hint))
			return
		}
		if t := bearer(r); t != "" {
			if _, err := s.agents.Authenticate(r.Context(), t); err == nil {
				writeError(w, domain.Errf(domain.CodePermissionDenied, "admin token required (an agent token cannot do this)").WithSuggestion(hint))
				return
			}
		}
		writeError(w, domain.Errf(domain.CodeUnauthenticated, "admin token missing or invalid").WithSuggestion(hint))
	}
}

// admin allows only the admin token. A missing or unknown token is 401
// (not authenticated); a valid agent token is 403 (authenticated, but this
// route is admin only).
func (s *Server) admin(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.isAdmin(r) {
			h(w, r)
			return
		}
		const hint = "set MYCO_ADMIN_TOKEN to the hub's admin token"
		if t := bearer(r); t != "" {
			if _, err := s.agents.Authenticate(r.Context(), t); err == nil {
				writeError(w, domain.Errf(domain.CodePermissionDenied, "admin token required (an agent token cannot do this)").WithSuggestion(hint))
				return
			}
		}
		writeError(w, domain.Errf(domain.CodeUnauthenticated, "admin token missing or invalid").WithSuggestion(hint))
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
		s.agent(func(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
			h(w, r.WithContext(context.WithValue(r.Context(), callerAgentKey{}, a)))
		})(w, r)
	}
}

// callerAgentKey: the agent behind an adminOrAgent request (absent for the
// admin and console tokens).
type callerAgentKey struct{}

// viewerOf is who is looking at agents: an agent (its own token), a member
// (the app), or someone who sees everything (the admin token, owners and
// admins, hubs without members).
func (s *Server) viewerOf(r *http.Request) domain.Viewer {
	if a, ok := r.Context().Value(callerAgentKey{}).(*domain.Agent); ok {
		return domain.Viewer{Agent: a}
	}
	actor := s.actorOf(r)
	if actor.Manages() || actor.Member == nil {
		return domain.Viewer{All: true}
	}
	return domain.Viewer{Member: actor.Member.ID}
}

// ---- handlers ----

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var req api.RegisterRequest
	if !decode(w, r, &req) {
		return
	}
	if !s.mayManage(w, r, "register agents directly (add nodes through a link instead)") {
		return
	}
	dry := dryRun(r)
	out, err := s.agents.Register(r.Context(), app.RegisterInput{Owner: s.ownerIDFor(r.Context(), s.actorOf(r)),
		Name: req.Name, DisplayName: req.DisplayName, Description: req.Description, Capabilities: req.Capabilities,
		AllowFrom: req.AllowFrom, Labels: req.Labels, Binding: bindingFromAPI(req.Binding), RotateToken: req.RotateToken, DryRun: dry,
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
	if !dry {
		action := "agent.register"
		if !out.Created {
			action = "agent.rotate_token"
		}
		s.record(r, action, string(out.Agent.Name), "")
	}
	writeJSON(w, status, api.RegisterResponse{Agent: s.agentDTO(out.Agent), Token: out.Token, Created: out.Created, DryRun: dry})
}

func (s *Server) removeAgent(w http.ResponseWriter, r *http.Request) {
	if !s.mayOwnAgent(w, r, r.PathValue("name"), "remove") {
		return
	}
	dry := dryRun(r)
	a, err := s.agents.Remove(r.Context(), r.PathValue("name"), dry)
	if err != nil {
		writeError(w, err)
		return
	}
	if !dry {
		s.record(r, "agent.remove", string(a.Name), "")
	}
	writeJSON(w, http.StatusOK, map[string]any{"agent": s.agentDTO(a), "removed": !dry, "dry_run": dry})
}

// cleanupAgents removes the nodes offline for a while that the caller may
// manage (members: their own; owner and admins: all).
func (s *Server) cleanupAgents(w http.ResponseWriter, r *http.Request) {
	var req api.CleanupRequest
	if !decode(w, r, &req) {
		return
	}
	if req.OfflineDays < 1 {
		writeError(w, domain.Errf(domain.CodeInvalidArgument, "offline_days must be at least 1"))
		return
	}
	stale, err := s.agents.Stale(r.Context(), time.Duration(req.OfflineDays)*24*time.Hour)
	if err != nil {
		writeError(w, err)
		return
	}
	actor, dry := s.actorOf(r), dryRun(r)
	out := api.CleanupResponse{Agents: []api.Agent{}, Removed: !dry, DryRun: dry}
	for _, a := range stale {
		if !actor.Owns(a.Owner) {
			continue
		}
		if !dry {
			if _, err := s.agents.Remove(r.Context(), string(a.Name), false); err != nil {
				if domain.IsCode(err, domain.CodeNotFound) {
					continue
				}
				writeError(w, err)
				return
			}
			s.record(r, "agent.remove", string(a.Name), fmt.Sprintf("cleanup: offline %d+ days", req.OfflineDays))
		}
		out.Agents = append(out.Agents, s.agentDTO(a))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) kickAgent(w http.ResponseWriter, r *http.Request) {
	if !s.mayOwnAgent(w, r, r.PathValue("name"), "disconnect") {
		return
	}
	dry := dryRun(r)
	a, err := s.agents.Kick(r.Context(), r.PathValue("name"), dry)
	if err != nil {
		writeError(w, err)
		return
	}
	if !dry {
		s.record(r, "agent.kick", string(a.Name), "")
	}
	writeJSON(w, http.StatusOK, map[string]any{"agent": s.agentDTO(a), "kicked": !dry, "dry_run": dry})
}

func (s *Server) createConsoleToken(w http.ResponseWriter, r *http.Request) {
	var in api.CreateConsoleTokenRequest
	if !decode(w, r, &in) {
		return
	}
	t, plain, err := s.console.CreateFor(r.Context(), s.ownerIDFor(r.Context(), s.actorOf(r)), in.Name)
	if err != nil {
		writeError(w, err)
		return
	}
	s.record(r, "console_token.create", t.Name, "")
	writeJSON(w, http.StatusCreated, api.CreateConsoleTokenResponse{ConsoleToken: consoleDTO(t), Token: plain})
}

func (s *Server) listConsoleTokens(w http.ResponseWriter, r *http.Request) {
	list, err := s.console.List(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	out := api.ConsoleTokenList{Tokens: make([]api.ConsoleToken, 0, len(list))}
	for _, t := range list {
		out.Tokens = append(out.Tokens, consoleDTO(t))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) revokeConsoleToken(w http.ResponseWriter, r *http.Request) {
	t, err := s.console.Revoke(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, consoleDTO(t))
}

func consoleDTO(t *domain.ConsoleToken) api.ConsoleToken {
	return api.ConsoleToken{ID: t.ID, Name: t.Name, Active: t.Active(), CreatedAt: t.CreatedAt,
		LastUsedAt: optTime(t.LastUsedAt), RevokedAt: optTime(t.RevokedAt)}
}

func optTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func (s *Server) listAgents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	list, err := s.agents.List(r.Context(), app.AgentFilter{Capability: q.Get("capability"), OnlineOnly: q.Get("online") == "true", Selector: q.Get("selector")})
	if err != nil {
		writeError(w, err)
		return
	}
	out := api.AgentList{Agents: make([]api.Agent, 0, len(list))}
	v := s.viewerOf(r)
	for _, a := range list {
		if v.Sees(a) {
			out.Agents = append(out.Agents, s.agentDTO(a))
		}
	}
	ptrs := make([]*api.Agent, len(out.Agents))
	for i := range out.Agents {
		ptrs[i] = &out.Agents[i]
	}
	s.enrich(r.Context(), ptrs...)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getAgent(w http.ResponseWriter, r *http.Request) {
	a, err := s.agents.Get(r.Context(), r.PathValue("name"))
	if err == nil && !s.viewerOf(r).Sees(a) {
		err = domain.Errf(domain.CodeNotFound, "agent %s not found", r.PathValue("name")).
			WithSuggestion("list agents with: myco agent list")
	}
	if err != nil {
		writeError(w, err)
		return
	}
	dto := s.agentDTO(a)
	s.enrich(r.Context(), &dto)
	writeJSON(w, http.StatusOK, dto)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	dto := s.agentDTO(a)
	s.enrich(r.Context(), &dto)
	writeJSON(w, http.StatusOK, dto)
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
	actor := s.actorOf(r)
	if req.Harness == "exec" && !actor.Manages() {
		writeError(w, domain.Errf(domain.CodePermissionDenied, "only the hub's owner or an admin can create links for exec nodes"))
		return
	}
	e, tok, err := s.enroll.Create(r.Context(), app.CreateEnrollmentInput{
		Kind: req.Kind, NamePrefix: req.NamePrefix, Harness: req.Harness, AllowFrom: req.AllowFrom, Capabilities: req.Capabilities, Labels: req.Labels,
		Note: req.Note, MaxUses: req.MaxUses, TTL: time.Duration(req.TTLSeconds) * time.Second, DryRun: dry, Owner: s.ownerIDFor(r.Context(), actor),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	if !dry {
		s.record(r, "link.create", e.ID, strings.TrimSpace(e.Kind+" "+e.Harness+" "+e.Note))
	}
	status := http.StatusCreated
	if dry {
		status = http.StatusOK
	}
	resp := api.CreateEnrollmentResponse{Enrollment: enrollmentDTO(e, s.agents.Now()), Token: tok, DryRun: dry}
	if e.Kind == domain.EnrollmentLink && tok != "" {
		resp.JoinURL = s.publicBase(r) + "/j/" + tok
	}
	writeJSON(w, status, resp)
}

// join gives a machine its own enrollment through a join link.
func (s *Server) join(w http.ResponseWriter, r *http.Request) {
	var req api.JoinRequest
	if !decode(w, r, &req) {
		return
	}
	dry := dryRun(r)
	out, err := s.enroll.Join(r.Context(), bearer(r), app.JoinInput{Machine: req.Machine, DryRun: dry})
	if err != nil {
		writeError(w, err)
		return
	}
	m := out.Machine
	resp := api.JoinResponse{Machine: m.Machine, Token: out.Token, Created: out.Created, NamePrefix: m.NamePrefix, Harness: m.Harness,
		AllowFrom: make([]string, 0, len(m.AllowFrom)), Labels: nonNil(m.Labels), DryRun: dry}
	for _, n := range m.AllowFrom {
		resp.AllowFrom = append(resp.AllowFrom, string(n))
	}
	status := http.StatusOK
	if out.Created && !dry {
		status = http.StatusCreated
	}
	writeJSON(w, status, resp)
}

// publicBase is the hub URL as clients see it: --public-url, else derived
// from the request (behind a TLS proxy, X-Forwarded-Proto says https).
func (s *Server) publicBase(r *http.Request) string {
	if u, _ := s.publicURL.Load().(string); u != "" {
		return strings.TrimRight(u, "/")
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := r.Host
	if s.lan.Load() && isLoopbackHost(host) {
		// Asked from this machine (the app, the CLI): a link for others must
		// name this machine's LAN address, not 127.0.0.1.
		if ip := osutil.LANAddr(); ip != "" {
			if _, port, err := net.SplitHostPort(host); err == nil {
				host = net.JoinHostPort(ip, port)
			} else {
				host = ip
			}
		}
	}
	return scheme + "://" + host
}

// isLoopbackHost reports whether a Host header names this machine only.
func isLoopbackHost(host string) bool {
	h := host
	if hh, _, err := net.SplitHostPort(host); err == nil {
		h = hh
	}
	h = strings.Trim(h, "[]")
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// SetLAN says the hub is reachable on the local network (it listens on every
// interface and has no public URL): links made from this machine then carry
// its LAN address.
func (s *Server) SetLAN(on bool) { s.lan.Store(on) }

// SetPublicURL sets the hub URL used in join links.
// It is safe to call while serving (a tunnel may change it).
func (s *Server) SetPublicURL(u string) { s.publicURL.Store(u) }

// SetVersion records the hub's build and where to read the newest known
// release from (a background checker; nil = never checked).
func (s *Server) SetVersion(version string, latest func() string) {
	s.version, s.latest = version, latest
}

// PublicURL is the configured public URL ("" if unknown).
func (s *Server) PublicURL() string { u, _ := s.publicURL.Load().(string); return u }

func (s *Server) listEnrollments(w http.ResponseWriter, r *http.Request) {
	list, err := s.enroll.List(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	actor := s.actorOf(r)
	out := api.EnrollmentList{Enrollments: make([]api.Enrollment, 0, len(list))}
	for _, e := range list {
		if !actor.Owns(e.Owner) { // members see their own links
			continue
		}
		out.Enrollments = append(out.Enrollments, enrollmentDTO(e, s.agents.Now()))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) revokeEnrollment(w http.ResponseWriter, r *http.Request) {
	cur, err := s.enroll.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	if !s.mayOwn(w, r, cur.Owner, "revoke links of") {
		return
	}
	dry := dryRun(r)
	e, err := s.enroll.Revoke(r.Context(), r.PathValue("id"), dry)
	if err != nil {
		writeError(w, err)
		return
	}
	if !dry {
		s.record(r, "link.revoke", e.ID, e.Note)
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
		Name: req.Name, DisplayName: req.DisplayName, Description: req.Description, Capabilities: req.Capabilities, Labels: req.Labels,
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

// by is the agent changing itself (nil: a person); see recordEdit.
func (s *Server) setLabels(w http.ResponseWriter, r *http.Request, name string, by *domain.Agent) {
	var req api.LabelsRequest
	if !decode(w, r, &req) {
		return
	}
	was, ok := s.cardBefore(w, r, name)
	if !ok {
		return
	}
	a, err := s.agents.SetLabels(r.Context(), name, req.Add, req.Remove)
	if err != nil {
		writeError(w, err)
		return
	}
	s.recordEdit(r, by, was, a)
	writeJSON(w, http.StatusOK, s.agentDTO(a))
}

func (s *Server) setSkills(w http.ResponseWriter, r *http.Request, name string, by *domain.Agent) {
	var req api.SkillsRequest
	if !decode(w, r, &req) {
		return
	}
	was, ok := s.cardBefore(w, r, name)
	if !ok {
		return
	}
	a, err := s.agents.SetSkills(r.Context(), name, skillsFromAPI(req.Skills))
	if err != nil {
		writeError(w, err)
		return
	}
	s.recordEdit(r, by, was, a)
	dto := s.agentDTO(a)
	s.enrich(r.Context(), &dto)
	writeJSON(w, http.StatusOK, dto)
}

// setVisibility: only people (the node's owner or the hub's admins) decide
// who can find and use a node; there is no /v1/me form.
func (s *Server) setVisibility(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !s.mayOwnAgent(w, r, name, "change who can see") {
		return
	}
	var req api.VisibilityRequest
	if !decode(w, r, &req) {
		return
	}
	a, err := s.agents.SetVisibility(r.Context(), name, req.Visibility)
	if err != nil {
		writeError(w, err)
		return
	}
	s.record(r, "agent.visibility", string(a.Name), string(a.Visibility))
	dto := s.agentDTO(a)
	s.enrich(r.Context(), &dto)
	writeJSON(w, http.StatusOK, dto)
}

func (s *Server) mySkills(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	s.setSkills(w, r, string(a.Name), a)
}

func (s *Server) adminSkills(w http.ResponseWriter, r *http.Request) {
	if !s.mayOwnAgent(w, r, r.PathValue("name"), "edit the skills of") {
		return
	}
	s.setSkills(w, r, r.PathValue("name"), nil)
}

func (s *Server) myLabels(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	s.setLabels(w, r, string(a.Name), a)
}

func (s *Server) adminLabels(w http.ResponseWriter, r *http.Request) {
	if !s.mayOwnAgent(w, r, r.PathValue("name"), "edit the labels of") {
		return
	}
	s.setLabels(w, r, r.PathValue("name"), nil)
}

// by is the agent changing itself (nil: a person, who may also change
// allow_from).
func (s *Server) updateProfile(w http.ResponseWriter, r *http.Request, name string, by *domain.Agent) {
	admin := by == nil
	var req api.ProfileRequest
	if !decode(w, r, &req) {
		return
	}
	if req.AllowFrom != nil && !admin {
		writeError(w, domain.Errf(domain.CodePermissionDenied, "only the hub admin can change allow_from").
			WithSuggestion("ask the admin to run: myco agent update --name %s --allow-from <agents>", name))
		return
	}
	was, ok := s.cardBefore(w, r, name)
	if !ok {
		return
	}
	a, err := s.agents.UpdateProfile(r.Context(), name, req.DisplayName, req.Description, req.AllowFrom, req.AcceptExecRisk)
	if err != nil {
		writeError(w, err)
		return
	}
	s.recordEdit(r, by, was, a)
	if admin && req.AllowFrom != nil {
		s.record(r, "agent.update", string(a.Name), "allow_from: "+orNone(strings.Join(*req.AllowFrom, ", ")))
	}
	dto := s.agentDTO(a)
	s.enrich(r.Context(), &dto)
	writeJSON(w, http.StatusOK, dto)
}

func (s *Server) myStatus(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	var req api.StatusRequest
	if !decode(w, r, &req) {
		return
	}
	got, err := s.agents.SetStatus(r.Context(), string(a.Name), req.Text)
	if err != nil {
		writeError(w, err)
		return
	}
	dto := s.agentDTO(got)
	s.enrich(r.Context(), &dto)
	writeJSON(w, http.StatusOK, dto)
}

func (s *Server) myProfile(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	s.updateProfile(w, r, string(a.Name), a)
}

func (s *Server) adminProfile(w http.ResponseWriter, r *http.Request) {
	if !s.mayOwnAgent(w, r, r.PathValue("name"), "edit") {
		return
	}
	s.updateProfile(w, r, r.PathValue("name"), nil)
}

func (s *Server) listAliases(w http.ResponseWriter, r *http.Request) {
	list, err := s.agents.ListAliases(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	out := api.AliasList{Aliases: make([]api.Alias, 0, len(list))}
	for _, a := range list {
		out.Aliases = append(out.Aliases, aliasDTO(a))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) setAlias(w http.ResponseWriter, r *http.Request) {
	var req api.AliasRequest
	if !decode(w, r, &req) {
		return
	}
	if !s.mayManage(w, r, "change aliases") {
		return
	}
	dry := dryRun(r)
	a, err := s.agents.SetAlias(r.Context(), r.PathValue("alias"), req.Target, dry)
	if err != nil {
		writeError(w, err)
		return
	}
	if !dry {
		s.record(r, "alias.set", r.PathValue("alias"), "→ "+req.Target)
	}
	writeJSON(w, http.StatusOK, map[string]any{"alias": aliasDTO(a), "dry_run": dry})
}

func (s *Server) removeAlias(w http.ResponseWriter, r *http.Request) {
	if !s.mayManage(w, r, "change aliases") {
		return
	}
	dry := dryRun(r)
	a, err := s.agents.RemoveAlias(r.Context(), r.PathValue("alias"), dry)
	if err != nil {
		writeError(w, err)
		return
	}
	if !dry {
		s.record(r, "alias.remove", r.PathValue("alias"), "")
	}
	writeJSON(w, http.StatusOK, map[string]any{"alias": aliasDTO(a), "removed": !dry, "dry_run": dry})
}

func (s *Server) broadcast(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	var req api.BroadcastRequest
	if !decode(w, r, &req) {
		return
	}
	dry := dryRun(r)
	out, err := s.messages.Broadcast(r.Context(), a, app.BroadcastInput{Selector: req.Selector, Type: req.Type, Body: req.Body,
		IdempotencyKey: req.IdempotencyKey, IncludeSelf: req.IncludeSelf, Attachments: req.Attachments, DryRun: dry})
	if err != nil {
		writeError(w, err)
		return
	}
	resp := api.BroadcastResponse{BroadcastID: out.ID, Messages: messageList(out.Messages).Messages,
		Skipped: []api.BroadcastSkip{}, Duplicate: out.Duplicate, DryRun: dry}
	for _, sk := range out.Skipped {
		resp.Skipped = append(resp.Skipped, api.BroadcastSkip{Agent: string(sk.Agent), Reason: sk.Reason})
	}
	status := http.StatusCreated
	if dry || out.Duplicate {
		status = http.StatusOK
	}
	writeJSON(w, status, resp)
}

func (s *Server) broadcastStatus(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	consume, _ := strconv.ParseBool(r.URL.Query().Get("consume"))
	ds, err := s.messages.BroadcastStatus(r.Context(), a, r.PathValue("id"), consume)
	if err != nil {
		writeError(w, err)
		return
	}
	out := api.BroadcastStatus{BroadcastID: r.PathValue("id"), Total: len(ds), Deliveries: make([]api.BroadcastDelivery, 0, len(ds))}
	for _, d := range ds {
		bd := api.BroadcastDelivery{Message: messageDTO(d.Message)}
		if d.Reply != nil {
			rep := messageDTO(d.Reply)
			bd.Reply = &rep
			out.Answered++
		}
		out.Deliveries = append(out.Deliveries, bd)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	if s.overview == nil {
		writeError(w, domain.Errf(domain.CodeNotFound, "overview not enabled on this hub"))
		return
	}
	var viewer *domain.Agent
	v := api.Viewer{Kind: "admin"}
	if !s.isAdmin(r) {
		a, err := s.agents.Authenticate(r.Context(), bearer(r))
		if err != nil {
			writeError(w, err)
			return
		}
		viewer, v = a, api.Viewer{Kind: "agent", Name: string(a.Name)}
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("recent"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	ov, err := s.overview.Snapshot(r.Context(), viewer, limit)
	if err != nil {
		writeError(w, err)
		return
	}
	out := api.Overview{GeneratedAt: ov.GeneratedAt, Viewer: v, OnlineWindowSeconds: int(s.agents.OnlineWindow().Seconds()),
		Agents: make([]api.AgentStatus, 0, len(ov.Agents))}
	// A member sees the hub's nodes, but only messages they may read.
	sc := app.Scope{All: true}
	if viewer == nil {
		if sc, _, err = s.scopeOf(r); err != nil {
			writeError(w, err)
			return
		}
	}
	out.Recent = messageList(readable(sc, ov.Recent)).Messages
	aliasMap, _ := s.agents.AliasMap(r.Context())
	var where map[domain.AgentName]string
	if viewer == nil && s.machines != nil { // admin view
		if views, w, err := s.machines.List(r.Context()); err == nil {
			where = w
			out.Machines = make([]api.Machine, 0, len(views))
			for _, v := range views {
				out.Machines = append(out.Machines, s.machineDTO(v))
			}
		}
	}
	see := domain.Viewer{Agent: viewer}
	if viewer == nil {
		see = s.viewerOf(r)
	}
	for _, a := range ov.Agents {
		if !see.Sees(a.Agent) {
			continue
		}
		st := api.AgentStatus{Agent: s.agentDTO(a.Agent), State: string(a.State), Queued: a.Queued, InProgress: a.InProgress, Held: a.Held}
		st.Receiving, st.SessionInUse = s.messages.Presence().State(a.Agent.Name, s.agents.OnlineWindow())
		st.Agent.Aliases = aliasMap[a.Agent.Name]
		if id, ok := where[a.Agent.Name]; ok {
			st.Agent.MachineID = id
		}
		if cur := readable(sc, a.Current); len(cur) > 0 {
			st.Current = messageList(cur).Messages
		}
		out.Agents = append(out.Agents, st)
		out.Totals.Agents++
		switch a.State {
		case app.StateWorking:
			out.Totals.Working++
		case app.StatePending:
			out.Totals.Pending++
		case app.StateIdle:
			out.Totals.Idle++
		default:
			out.Totals.Offline++
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) filesEnabled(w http.ResponseWriter) bool {
	if s.files == nil {
		writeError(w, domain.Errf(domain.CodeNotFound, "this hub does not store files"))
		return false
	}
	return true
}

func (s *Server) uploadFile(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	if !s.filesEnabled(w) {
		return
	}
	f, err := s.files.Upload(r.Context(), a, r.URL.Query().Get("name"), r.Header.Get("Content-Type"), r.Body)
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			err = domain.Errf(domain.CodeInvalidArgument, "file larger than %d bytes", s.files.Policy().MaxSize)
		}
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, fileDTO(f))
}

func (s *Server) fileMeta(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	if !s.filesEnabled(w) {
		return
	}
	f, err := s.files.Meta(r.Context(), a, r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, fileDTO(f))
}

func (s *Server) downloadFile(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	if !s.filesEnabled(w) {
		return
	}
	f, rc, err := s.files.Open(r.Context(), a, r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	defer rc.Close()
	ct := f.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Length", strconv.FormatInt(f.Size, 10))
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(f.Name))
	w.Header().Set("X-Myco-SHA256", f.SHA256)
	w.Header().Set("X-Myco-File-Name", url.PathEscape(f.Name))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.Copy(w, rc)
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
		To: req.To, Type: req.Type, Body: req.Body, ThreadID: req.ThreadID, IdempotencyKey: req.IdempotencyKey,
		Attachments: req.Attachments, DryRun: dry,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	status := http.StatusCreated
	if out.Duplicate || dry {
		status = http.StatusOK
	}
	writeJSON(w, status, api.SendResponse{Message: messageDTO(out.Message), Duplicate: out.Duplicate, DryRun: dry, ViaAlias: string(out.ViaAlias)})
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
	m, err := s.messages.ReplyWith(r.Context(), a, r.PathValue("id"), req.Body, req.Attachments, dry)
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

func (s *Server) release(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	var req api.ReleaseRequest
	if r.ContentLength != 0 && !decode(w, r, &req) {
		return
	}
	dry := dryRun(r)
	m, err := s.messages.Release(r.Context(), a, r.PathValue("id"), time.Duration(req.DelaySeconds)*time.Second, dry)
	if err != nil {
		writeError(w, err)
		return
	}
	if req.Reason == "in_use" && !dry {
		if p := s.messages.Presence(); p != nil {
			p.SessionInUse(a.Name)
		}
	}
	writeJSON(w, http.StatusOK, api.MessageResult{Message: messageDTO(m), DryRun: dry})
}

func (s *Server) waitReply(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	// With progress_since (RFC 3339; "0" = from the start) the wait also
	// returns as soon as there are newer progress updates.
	if ps := r.URL.Query().Get("progress_since"); ps != "" {
		var since time.Time
		if ps != "0" {
			t, err := time.Parse(time.RFC3339Nano, ps)
			if err != nil {
				writeError(w, domain.Errf(domain.CodeInvalidArgument, "progress_since must be RFC 3339 or 0"))
				return
			}
			since = t
		}
		m, progress, err := s.messages.WaitUpdate(r.Context(), a, r.PathValue("id"), waitParam(r), since)
		if err != nil {
			writeError(w, err)
			return
		}
		resp := api.WaitResponse{TimedOut: m == nil && len(progress) == 0, Progress: messageList(progress).Messages}
		if m != nil {
			dto := messageDTO(m)
			resp.Reply = &dto
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}
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

// progress and steer create a follow-up on task {id}: an update from its
// recipient, or a correction from its sender.
func (s *Server) followUp(kind string) func(http.ResponseWriter, *http.Request, *domain.Agent) {
	return func(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
		var req api.ReplyRequest
		if !decode(w, r, &req) {
			return
		}
		dry := dryRun(r)
		op := s.messages.Progress
		if kind == "steer" {
			op = s.messages.Steer
		}
		m, err := op(r.Context(), a, r.PathValue("id"), req.Body, dry)
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
}

func (s *Server) pull(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	var req api.PullRequest
	if r.ContentLength != 0 && !decode(w, r, &req) {
		return
	}
	types := make([]domain.MessageType, 0, len(req.Types))
	for _, t := range req.Types {
		types = append(types, domain.MessageType(t))
	}
	pull := s.messages.Pull
	if req.Via == "runner" {
		pull = s.messages.PullForRunner
	}
	msgs, err := pull(r.Context(), a, req.Limit, time.Duration(req.WaitSeconds)*time.Second, types...)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return // client went away
		}
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, messageList(msgs))
}

// threadAny serves participants (agent token) and the admin (any thread).
func (s *Server) threadAny(w http.ResponseWriter, r *http.Request) {
	if s.isAdmin(r) {
		msgs, err := s.messages.ThreadAll(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, err)
			return
		}
		sc, _, err := s.scopeOf(r)
		if err != nil {
			writeError(w, err)
			return
		}
		if !sc.Readable(msgs) {
			writeError(w, domain.Errf(domain.CodeNotFound, "thread %s not found", r.PathValue("id")))
			return
		}
		writeJSON(w, http.StatusOK, messageList(msgs))
		return
	}
	s.agent(s.thread)(w, r)
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

func (s *Server) machinesEnabled(w http.ResponseWriter) bool {
	if s.machines == nil {
		writeError(w, domain.Errf(domain.CodeNotFound, "machines are not enabled on this hub"))
		return false
	}
	return true
}

func (s *Server) listMachines(w http.ResponseWriter, r *http.Request) {
	if !s.machinesEnabled(w) {
		return
	}
	views, _, err := s.machines.List(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	out := api.MachineList{Machines: make([]api.Machine, 0, len(views))}
	for _, v := range views {
		out.Machines = append(out.Machines, s.machineDTO(v))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) renameMachine(w http.ResponseWriter, r *http.Request) {
	if !s.machinesEnabled(w) {
		return
	}
	var in api.RenameMachineRequest
	if !decode(w, r, &in) {
		return
	}
	if !s.mayOwnMachine(w, r, r.PathValue("id"), "rename") {
		return
	}
	m, err := s.machines.Rename(r.Context(), r.PathValue("id"), in.Name)
	if err != nil {
		writeError(w, err)
		return
	}
	s.record(r, "machine.rename", m.ID, "→ "+m.Name)
	writeJSON(w, http.StatusOK, map[string]string{"id": m.ID, "name": m.Name})
}

// spawnOnMachine starts a session on a machine for an administrator and
// returns the machine's answer (api.SpawnResult).
func (s *Server) spawnOnMachine(w http.ResponseWriter, r *http.Request) {
	if !s.machinesEnabled(w) {
		return
	}
	var in api.SpawnRequest
	if !decode(w, r, &in) {
		return
	}
	in.EnrollToken = ""
	if !s.mayOwnMachine(w, r, r.PathValue("id"), "start sessions on") {
		return
	}
	if in.CLI == "exec" {
		// A shell node: the machine joins it with a one-use exec enrollment
		// made here, by the admin, carrying who may send it commands.
		if len(in.AllowFrom) == 0 {
			writeError(w, domain.Errf(domain.CodeInvalidArgument, "a shell node needs at least one agent allowed to send it commands"))
			return
		}
		node, err := s.machines.SpawnNode(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, err)
			return
		}
		_, tok, err := s.enroll.Create(r.Context(), app.CreateEnrollmentInput{
			NamePrefix: strings.TrimSuffix(string(node), "machine"), Harness: "exec", AllowFrom: in.AllowFrom,
			Note: "shell node added from the console", MaxUses: 1, TTL: 10 * time.Minute, Owner: s.ownerIDFor(r.Context(), s.actorOf(r)),
		})
		if err != nil {
			writeError(w, err)
			return
		}
		in.EnrollToken = tok
	}
	body, _ := json.Marshal(in)
	wait := waitParam(r)
	if wait <= 0 {
		wait = 5 * time.Minute
	}
	reply, err := s.machines.Spawn(r.Context(), r.PathValue("id"), body, wait)
	if err != nil {
		writeError(w, err)
		return
	}
	if reply == nil {
		writeError(w, domain.Errf(domain.CodeInvalidState, "the machine did not answer in time").
			WithSuggestion("the session may still be starting; check the node list in a minute"))
		return
	}
	var res api.SpawnResult
	if err := json.Unmarshal([]byte(reply.Body), &res); err != nil {
		// A failure reply (the runner could not run the machine node).
		writeError(w, domain.Errf(domain.CodeInvalidState, "the machine could not start the session: %s", strings.TrimSpace(reply.Body)))
		return
	}
	switch {
	case res.Denied:
		writeError(w, domain.Errf(domain.CodePermissionDenied, "%s", res.Error))
	case res.Error != "":
		writeError(w, domain.Errf(domain.CodeInvalidState, "%s", res.Error))
	default:
		kind := orNone(res.CLI)
		if res.CLI == "exec" {
			kind = "shell node, commands from " + strings.Join(in.AllowFrom, ", ")
		}
		if in.ForkFrom != "" {
			kind += " copy of session " + in.ForkFrom
		}
		s.record(r, "machine.spawn", res.Agent, kind+" in "+res.Workdir+" ("+r.PathValue("id")+")")
		writeJSON(w, http.StatusOK, res)
	}
}

func (s *Server) reportMachine(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	if !s.machinesEnabled(w) {
		return
	}
	var in api.MachineReport
	if !decode(w, r, &in) {
		return
	}
	m, err := s.machines.Report(r.Context(), a, domain.MachineReport{Fingerprint: in.Fingerprint, Name: in.Name,
		Hostname: in.Hostname, OS: in.OS, Arch: in.Arch, MycoVersion: in.MycoVersion})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"machine_id": m.ID, "name": m.Name})
}

func (s *Server) reportRuntime(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	if !s.machinesEnabled(w) {
		return
	}
	var in api.Runtime
	if !decode(w, r, &in) {
		return
	}
	if err := s.machines.SetRuntime(r.Context(), a, domain.Runtime{CLI: in.CLI, CLIVersion: in.CLIVersion, Model: in.Model,
		ModelSource: in.ModelSource, Access: in.Access, Provider: in.Provider, ProviderSource: in.ProviderSource}); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, in)
}

// consoleDevice names the caller for console messages: the console token's
// device, or "admin" for the hub's own admin token.
func (s *Server) consoleDevice(r *http.Request) string {
	if s.console != nil && !s.isMasterAdmin(r) {
		if t, ok := s.console.Authenticate(r.Context(), bearer(r)); ok {
			return t.Name
		}
	}
	return "admin"
}

func (s *Server) consoleSend(w http.ResponseWriter, r *http.Request) {
	var req api.SendRequest
	if !decode(w, r, &req) {
		return
	}
	name, memberID := s.consoleIdentity(r)
	out, err := s.messages.SendFromMember(r.Context(), name, memberID, app.SendInput{
		To: req.To, Type: req.Type, Body: req.Body, ThreadID: req.ThreadID, IdempotencyKey: req.IdempotencyKey,
		Attachments: req.Attachments,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	status := http.StatusCreated
	if out.Duplicate {
		status = http.StatusOK
	}
	if !out.Duplicate {
		s.record(r, "message."+string(out.Message.Type), string(out.Message.To), out.Message.ID)
	}
	writeJSON(w, status, api.SendResponse{Message: messageDTO(out.Message), Duplicate: out.Duplicate, ViaAlias: string(out.ViaAlias)})
}

// consoleIdentity is the sender name and member id for what a console
// sends: the member, or the owner for the hub's admin token.
func (s *Server) consoleIdentity(r *http.Request) (string, string) {
	a := s.actorOf(r)
	if a.Member != nil {
		return a.Member.Name, a.Member.ID
	}
	if s.members != nil {
		if o, err := s.members.Owner(r.Context()); err == nil {
			return o.Name, o.ID
		}
	}
	return s.consoleDevice(r), ""
}

func (s *Server) consoleSteer(w http.ResponseWriter, r *http.Request) {
	var req api.SteerRequest
	if !decode(w, r, &req) {
		return
	}
	name, _ := s.consoleIdentity(r)
	m, err := s.messages.SteerFromConsole(r.Context(), name, s.actorOf(r).Manages(), r.PathValue("id"), req.Body)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, messageDTO(m))
}

// consoleDismiss closes an open message for its recipient (owner of the
// recipient node, or someone who manages the hub), without a reply.
func (s *Server) consoleDismiss(w http.ResponseWriter, r *http.Request) {
	actor := s.actorOf(r)
	id := r.PathValue("id")
	m, err := s.messages.DismissFromConsole(r.Context(), id, func(to domain.AgentName) error {
		if actor.Manages() {
			return nil
		}
		a, err := s.agents.Get(r.Context(), string(to))
		if err != nil {
			return err
		}
		if !actor.Owns(a.Owner) {
			return domain.Errf(domain.CodePermissionDenied, "you can only dismiss messages of your own nodes").
				WithSuggestion("ask the node's owner or a hub admin")
		}
		return nil
	})
	if err != nil {
		writeError(w, err)
		return
	}
	s.record(r, "message.dismiss", string(m.To), id+" from "+string(m.From))
	writeJSON(w, http.StatusOK, messageDTO(m))
}

func (s *Server) consolePeers(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	sc, _, err := s.scopeOf(r)
	if err != nil {
		writeError(w, err)
		return
	}
	ps, err := s.messages.Peers(r.Context(), sc, domain.AgentName(r.PathValue("name")), limit)
	if err != nil {
		writeError(w, err)
		return
	}
	out := api.PeerList{Peers: make([]api.Peer, 0, len(ps))}
	for _, p := range ps {
		pe := api.Peer{Agent: string(p.Agent), Messages: p.Messages}
		if p.Last != nil {
			m := messageDTO(p.Last)
			pe.Last = &m
		}
		out.Peers = append(out.Peers, pe)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) consoleExchanges(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	before, _ := strconv.ParseInt(q.Get("before"), 10, 64)
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	sc, _, err := s.scopeOf(r)
	if err != nil {
		writeError(w, err)
		return
	}
	ms, err := s.messages.Between(r.Context(), sc, domain.AgentName(q.Get("a")), domain.AgentName(q.Get("b")), before, limit+1)
	if err != nil {
		writeError(w, err)
		return
	}
	out := api.Exchange{Messages: make([]api.Message, 0, len(ms))}
	if len(ms) > limit {
		ms, out.More = ms[:limit], true
	}
	for _, m := range ms {
		out.Messages = append(out.Messages, messageDTO(m))
	}
	writeJSON(w, http.StatusOK, out)
}

// scopeOf is what the request may read from the console: the whole hub for
// the admin token, owners and admins; for a member, what their console name
// or the nodes they own take part in. Names are the reader's own (nil on
// hubs without members, where everything is theirs). It also returns the
// hub's agents.
func (s *Server) scopeOf(r *http.Request) (app.Scope, []*domain.Agent, error) {
	agents, err := s.agents.List(r.Context(), app.AgentFilter{})
	if err != nil {
		return app.Scope{}, nil, err
	}
	a := s.actorOf(r)
	sc := app.Scope{All: a.Manages()}
	name, memberID := s.consoleIdentity(r)
	if memberID == "" {
		if !sc.All {
			sc.Names = []domain.AgentName{}
		}
		return sc, agents, nil
	}
	sc.Names = []domain.AgentName{app.ConsoleSender(name)}
	for _, ag := range agents {
		if ag.Owner == memberID {
			sc.Names = append(sc.Names, ag.Name)
		}
	}
	return sc, agents, nil
}

func (s *Server) consoleRecent(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	before, _ := strconv.ParseInt(q.Get("before"), 10, 64)
	limit, _ := strconv.Atoi(q.Get("limit"))
	sc, agents, err := s.scopeOf(r)
	if err != nil {
		writeError(w, err)
		return
	}
	items, more, err := s.messages.Recent(r.Context(), agents, app.RecentInput{Scope: sc, Mine: q.Get("scope") == "mine",
		Label: q.Get("label"), Query: q.Get("q"), Before: before, Limit: limit})
	if err != nil {
		writeError(w, err)
		return
	}
	out := api.RecentList{Items: make([]api.RecentItem, 0, len(items)), More: more}
	for _, it := range items {
		ri := api.RecentItem{Kind: it.Kind, ThreadID: it.ThreadID, Other: string(it.Other), Node: string(it.Node), UpdatedAt: it.UpdatedAt,
			Messages: it.Messages, First: messageDTO(it.First), Last: messageDTO(it.Last), State: it.State, Participants: make([]string, 0, len(it.Participants))}
		for _, p := range it.Participants {
			ri.Participants = append(ri.Participants, string(p))
		}
		if it.Task != nil {
			m := messageDTO(it.Task)
			ri.Task = &m
		}
		if it.Answer != nil {
			m := messageDTO(it.Answer)
			ri.Answer = &m
		}
		out.Items = append(out.Items, ri)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) consoleConversations(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	before, _ := strconv.ParseInt(q.Get("before"), 10, 64)
	limit, _ := strconv.Atoi(q.Get("limit"))
	sc, _, err := s.scopeOf(r)
	if err != nil {
		writeError(w, err)
		return
	}
	cs, err := s.messages.Conversations(r.Context(), sc, q.Get("agent"), q.Get("q"), before, limit)
	if err != nil {
		writeError(w, err)
		return
	}
	out := api.ConversationList{Conversations: make([]api.Conversation, 0, len(cs))}
	for _, c := range cs {
		out.Conversations = append(out.Conversations, api.Conversation{ThreadID: c.ThreadID, Agent: string(c.Agent), UpdatedAt: c.UpdatedAt,
			First: messageDTO(c.First), Last: messageDTO(c.Last), Messages: c.Messages})
	}
	writeJSON(w, http.StatusOK, out)
}

// readable keeps the messages the scope may read.
func readable(sc app.Scope, ms []*domain.Message) []*domain.Message {
	if sc.All {
		return ms
	}
	var out []*domain.Message
	for _, m := range ms {
		if sc.Allows(m.From, m.To) {
			out = append(out, m)
		}
	}
	return out
}
