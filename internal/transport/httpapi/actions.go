package httpapi

import (
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/domain"
)

// Agent actions (see api/actions.go): the agent's routes under /v1/me, the
// owner's and the trusted App host's under /v1/console.

// SetActions enables agent actions.
func (s *Server) SetActions(a *app.ActionService) { s.actions = a }

func (s *Server) actionRoutes(m *http.ServeMux) {
	// The agent.
	m.HandleFunc("GET /v1/me/action-targets", s.agent(s.listActionTargets))
	m.HandleFunc("POST /v1/me/action-grants", s.agent(s.requestActionGrant))
	m.HandleFunc("GET /v1/me/action-grants", s.agent(s.listMyActionGrants))
	m.HandleFunc("GET /v1/me/action-grants/{id}", s.agent(s.getActionGrant))
	m.HandleFunc("POST /v1/me/action-grants/{id}/end", s.agent(s.endActionGrant))
	m.HandleFunc("GET /v1/me/action-grants/{id}/actions", s.agent(s.listGrantActions))
	m.HandleFunc("POST /v1/me/action-jobs", s.agent(s.runAction))
	m.HandleFunc("GET /v1/me/action-jobs/{id}", s.agent(s.getActionJob))
	m.HandleFunc("POST /v1/me/action-jobs/{id}/cancel", s.agent(s.cancelActionJob))
	m.HandleFunc("GET /v1/me/action-artifacts/{id}", s.agent(s.getActionArtifact))
	// The owner.
	m.HandleFunc("GET /v1/console/action-grants", s.admin(s.listOwnerActionGrants))
	m.HandleFunc("POST /v1/console/action-grants/{id}/revoke", s.admin(s.revokeActionGrant))
	// The trusted App on the target computer.
	m.HandleFunc("POST /v1/console/action-hosts", s.admin(s.registerActionHost))
	m.HandleFunc("GET /v1/console/action-hosts/{id}/jobs", s.admin(s.actionHostJobs))
	m.HandleFunc("GET /v1/console/action-jobs/{id}", s.admin(s.actionHostJob))
	m.HandleFunc("POST /v1/console/action-jobs/{id}/artifacts", s.admin(s.uploadActionArtifact))
	m.HandleFunc("POST /v1/console/action-jobs/{id}/complete", s.admin(s.completeActionJob))
}

func (s *Server) actionsEnabled(w http.ResponseWriter) bool {
	if s.actions == nil {
		writeError(w, domain.Errf(domain.CodeUnsupported, "agent actions are not available on this hub").WithSuggestion("upgrade the hub to v0.21.21 or later"))
		return false
	}
	return true
}

// actionWait reads ?wait=30s (or ?wait_seconds=30).
func actionWait(r *http.Request) time.Duration {
	if v := r.URL.Query().Get("wait"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
		if n, err := strconv.Atoi(v); err == nil {
			return time.Duration(n) * time.Second
		}
	}
	return waitParam(r)
}

func (s *Server) requestActionGrant(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	if !s.actionsEnabled(w) {
		return
	}
	var req api.ActionGrantRequest
	if !decode(w, r, &req) {
		return
	}
	g, err := s.actions.RequestGrant(r.Context(), a, req, dryRun(r))
	if err != nil {
		writeError(w, err)
		return
	}
	if dryRun(r) {
		writeJSON(w, http.StatusOK, map[string]any{"dry_run": true, "grant": g})
		return
	}
	writeJSON(w, http.StatusCreated, g)
}

func (s *Server) listMyActionGrants(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	if !s.actionsEnabled(w) {
		return
	}
	writeJSON(w, http.StatusOK, api.ActionGrantList{Grants: s.actions.Grants(r.Context(), a)})
}

func (s *Server) getActionGrant(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	if !s.actionsEnabled(w) {
		return
	}
	g, err := s.actions.Grant(r.Context(), a, r.PathValue("id"), actionWait(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (s *Server) endActionGrant(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	if !s.actionsEnabled(w) {
		return
	}
	if dryRun(r) {
		g, err := s.actions.Grant(r.Context(), a, r.PathValue("id"), 0)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"dry_run": true, "grant": g})
		return
	}
	g, err := s.actions.EndGrant(r.Context(), a, r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (s *Server) listGrantActions(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	if !s.actionsEnabled(w) {
		return
	}
	list, err := s.actions.Actions(r.Context(), a, r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, api.ActionDescriptorList{Actions: list})
}

func (s *Server) runAction(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	if !s.actionsEnabled(w) {
		return
	}
	var inv api.ActionInvocation
	if !decode(w, r, &inv) {
		return
	}
	if dryRun(r) {
		writeJSON(w, http.StatusOK, map[string]any{"dry_run": true, "invocation": inv})
		return
	}
	j, err := s.actions.RunAction(r.Context(), a, inv)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, j)
}

func (s *Server) getActionJob(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	if !s.actionsEnabled(w) {
		return
	}
	j, err := s.actions.Job(r.Context(), a, r.PathValue("id"), actionWait(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, j)
}

func (s *Server) cancelActionJob(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	if !s.actionsEnabled(w) {
		return
	}
	j, err := s.actions.CancelJob(r.Context(), a, r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, j)
}

func (s *Server) getActionArtifact(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	if !s.actionsEnabled(w) {
		return
	}
	art, data, err := s.actions.Artifact(r.Context(), a, r.PathValue("id"))
	if err != nil {
		if domain.IsCode(err, domain.CodeNotFound) {
			writeJSON(w, http.StatusGone, api.Error{Code: "not_found", Message: err.Error(), Suggestion: "observe again for a fresh one"})
			return
		}
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", art.MediaType)
	w.Header().Set("Cache-Control", "no-store")
	// A file for the agent, never a page to show (an SVG can hold script).
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", "attachment")
	w.Header().Set("Content-Security-Policy", "sandbox")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	_, _ = w.Write(data)
}

func (s *Server) listOwnerActionGrants(w http.ResponseWriter, r *http.Request) {
	if !s.actionsEnabled(w) {
		return
	}
	list, err := s.actions.OwnerGrants(r.Context(), s.actorOf(r), r.URL.Query().Get("state"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, api.ActionGrantList{Grants: list})
}

func (s *Server) revokeActionGrant(w http.ResponseWriter, r *http.Request) {
	if !s.actionsEnabled(w) {
		return
	}
	g, err := s.actions.Revoke(r.Context(), s.actorOf(r), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (s *Server) registerActionHost(w http.ResponseWriter, r *http.Request) {
	if !s.actionsEnabled(w) {
		return
	}
	var req api.ActionHostRegister
	if !decode(w, r, &req) {
		return
	}
	h, err := s.actions.RegisterHost(r.Context(), s.actorOf(r), req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, h)
}

func (s *Server) actionHostJobs(w http.ResponseWriter, r *http.Request) {
	if !s.actionsEnabled(w) {
		return
	}
	jobs, err := s.actions.HostJobs(r.Context(), s.actorOf(r), r.PathValue("id"), actionWait(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, api.ActionHostJobs{Jobs: jobs})
}

func (s *Server) actionHostJob(w http.ResponseWriter, r *http.Request) {
	if !s.actionsEnabled(w) {
		return
	}
	q := r.URL.Query()
	st, err := s.actions.HostJob(r.Context(), s.actorOf(r), r.PathValue("id"), q.Get("host_id"), q.Get("generation"), actionWait(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) uploadActionArtifact(w http.ResponseWriter, r *http.Request) {
	if !s.actionsEnabled(w) {
		return
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, 4<<20+1))
	if err != nil {
		writeError(w, domain.Errf(domain.CodeInvalidArgument, "could not read the artifact: %v", err))
		return
	}
	q := r.URL.Query()
	art, err := s.actions.UploadArtifact(r.Context(), s.actorOf(r), r.PathValue("id"), q.Get("host_id"), q.Get("generation"), r.Header.Get("Content-Type"), data)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, art)
}

func (s *Server) completeActionJob(w http.ResponseWriter, r *http.Request) {
	if !s.actionsEnabled(w) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 300<<10)
	var c api.ActionJobComplete
	if !decode(w, r, &c) {
		return
	}
	j, err := s.actions.CompleteJob(r.Context(), s.actorOf(r), r.PathValue("id"), c)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, j)
}

func (s *Server) listActionTargets(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	if !s.actionsEnabled(w) {
		return
	}
	ts, err := s.actions.ResourceTargets(r.Context(), a)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, api.ActionTargetList{Targets: ts})
}
