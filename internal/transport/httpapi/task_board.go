package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/domain"
)

func (s *Server) SetTaskMonitor(v *app.TaskMonitorService) { s.taskMonitor = v }
func (s *Server) taskMonitorRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /v1/tasks/board", s.taskMonitorCaller)
	m.HandleFunc("GET /v1/projects/{name}/task-alerts", s.taskMonitorCaller)
	m.HandleFunc("PUT /v1/projects/{name}/task-alerts", s.taskMonitorCaller)
}
func (s *Server) taskMonitorCaller(w http.ResponseWriter, r *http.Request) {
	if s.taskMonitor == nil {
		writeError(w, domain.Errf(domain.CodeUnsupported, "task board and alerts are not enabled on this Hub").WithSuggestion("upgrade the workspace Hub"))
		return
	}
	if s.isAdmin(r) {
		by := s.actorOf(r)
		if by.Member == nil && by.Admin && s.members != nil {
			owner, err := s.members.Owner(r.Context())
			if err != nil {
				writeError(w, err)
				return
			}
			by.Member = owner
		}
		s.taskMonitorRequest(w, r, by)
		return
	}
	s.agent(func(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
		s.taskMonitorRequest(w, r, app.Actor{Agent: a})
	})(w, r)
}
func (s *Server) taskMonitorRequest(w http.ResponseWriter, r *http.Request, by app.Actor) {
	if r.URL.Path == "/v1/tasks/board" {
		all := false
		var err error
		if raw := r.URL.Query().Get("all"); raw != "" {
			all, err = strconv.ParseBool(raw)
			if err != nil {
				writeError(w, domain.Errf(domain.CodeInvalidArgument, "all must be true or false").WithSuggestion("use all=true or all=false"))
				return
			}
		}
		var stale time.Duration
		if raw := r.URL.Query().Get("stale"); raw != "" {
			stale, err = time.ParseDuration(raw)
			if err != nil || stale <= 0 {
				writeError(w, domain.Errf(domain.CodeInvalidArgument, "stale must be a positive duration such as 15m").WithSuggestion("use a whole-second duration from 1s to 168h"))
				return
			}
		}
		out, err := s.taskMonitor.Board(r.Context(), by, r.URL.Query().Get("project"), all, stale)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	var set *time.Duration
	if r.Method == http.MethodPut {
		var req api.TaskAlertPolicyRequest
		if !decode(w, r, &req) {
			return
		}
		d := time.Duration(req.StaleSeconds) * time.Second
		if req.StaleSeconds < 1 || req.StaleSeconds > 7*24*3600 {
			writeError(w, domain.Errf(domain.CodeInvalidArgument, "stale_seconds must be from 1 to 604800").WithSuggestion("use myco task alerts set --project <name> --stale 15m"))
			return
		}
		set = &d
	}
	out, err := s.taskMonitor.AlertPolicy(r.Context(), by, r.PathValue("name"), set, dryRun(r))
	if err != nil {
		writeError(w, err)
		return
	}
	if set != nil && !dryRun(r) {
		s.recordAs(r, by, "project.task-alerts", out.Project, "updated stale threshold")
	}
	writeJSON(w, http.StatusOK, out)
}
