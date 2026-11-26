package httpapi

import (
	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/domain"
	"net/http"
	"strings"
)

func (s *Server) SetGoals(g *app.GoalService) { s.goals = g }
func (s *Server) goalRoutes(m *http.ServeMux) {
	for _, p := range []string{"POST /v1/goals", "GET /v1/goals", "GET /v1/goals/{id}", "PATCH /v1/goals/{id}", "POST /v1/goals/{id}/pause", "POST /v1/goals/{id}/resume", "POST /v1/goals/{id}/stop", "POST /v1/goals/{id}/rounds"} {
		m.HandleFunc(p, s.goalCaller)
	}
}
func (s *Server) goalCaller(w http.ResponseWriter, r *http.Request) {
	if s.goals == nil {
		writeError(w, domain.Errf(domain.CodeNotFound, "goals are not enabled on this Hub"))
		return
	}
	if s.isAdmin(r) {
		s.goalRequest(w, r, s.actorOf(r))
		return
	}
	s.agent(func(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
		s.goalRequest(w, r, app.Actor{Agent: a})
	})(w, r)
}
func (s *Server) goalRequest(w http.ResponseWriter, r *http.Request, by app.Actor) {
	id := r.PathValue("id")
	dry := dryRun(r)
	if r.Method == http.MethodGet {
		if id != "" {
			o, e := s.goals.Get(r.Context(), by, id)
			if e != nil {
				writeError(w, e)
				return
			}
			writeJSON(w, 200, api.GoalDetail{Goal: o.Goal, Rounds: o.Rounds})
			return
		}
		state := r.URL.Query().Get("state")
		if state != "" && state != "active" && state != "all" {
			writeError(w, domain.Errf(domain.CodeInvalidArgument, "state must be active or all"))
			return
		}
		o, e := s.goals.List(r.Context(), by, r.URL.Query().Get("agent"), r.URL.Query().Get("project"), state == "all")
		if e != nil {
			writeError(w, e)
			return
		}
		writeJSON(w, 200, api.GoalList{Goals: o})
		return
	}
	var o *domain.Goal
	var e error
	switch {
	case id == "":
		var in api.GoalCreateRequest
		if !decode(w, r, &in) {
			return
		}
		o, e = s.goals.Create(r.Context(), by, app.GoalCreate{Agent: in.Agent, Objective: in.Objective, DoneWhen: in.DoneWhen, Project: in.Project, Thread: in.Thread, EverySec: in.EverySec, MaxRounds: in.MaxRounds, ForSec: in.ForSec}, dry)
	case strings.HasSuffix(r.URL.Path, "/rounds"):
		var in api.GoalReportRequest
		if !decode(w, r, &in) {
			return
		}
		o, e = s.goals.Report(r.Context(), by, id, app.GoalReport(in), dry)
	case r.Method == http.MethodPatch:
		var in api.GoalUpdateRequest
		if !decode(w, r, &in) {
			return
		}
		o, e = s.goals.Change(r.Context(), by, id, "update", "", app.GoalUpdate(in), dry)
	default:
		var in api.GoalControlRequest
		if !decode(w, r, &in) {
			return
		}
		parts := strings.Split(r.URL.Path, "/")
		o, e = s.goals.Change(r.Context(), by, id, parts[len(parts)-1], in.Reason, app.GoalUpdate{}, dry)
	}
	if e != nil {
		writeError(w, e)
		return
	}
	status := 200
	if id == "" && !dry {
		status = 201
	}
	writeJSON(w, status, o)
}
