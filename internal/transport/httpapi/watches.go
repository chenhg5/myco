package httpapi

import (
	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/domain"
	"net/http"
	"strings"
)

func (s *Server) SetWatches(w *app.WatchService) { s.watches = w }
func (s *Server) watchRoutes(m *http.ServeMux) {
	for _, route := range []string{"POST /v1/watches", "GET /v1/watches", "GET /v1/watches/{id}", "POST /v1/watches/{id}/stop", "POST /v1/watches/{id}/extend", "POST /v1/watches/{id}/claim", "POST /v1/watches/{id}/checks"} {
		m.HandleFunc(route, s.watchCaller)
	}
}
func (s *Server) watchCaller(w http.ResponseWriter, r *http.Request) {
	if s.watches == nil {
		writeError(w, domain.Errf(domain.CodeNotFound, "watches are not enabled on this Hub"))
		return
	}
	if s.isAdmin(r) {
		s.watchRequest(w, r, s.actorOf(r))
		return
	}
	s.agent(func(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
		s.watchRequest(w, r, app.Actor{Agent: a})
	})(w, r)
}
func (s *Server) watchRequest(w http.ResponseWriter, r *http.Request, by app.Actor) {
	dry := dryRun(r)
	var err error
	id := r.PathValue("id")
	if r.Method == http.MethodGet {
		if id != "" {
			o, err := s.watches.Get(r.Context(), by, id)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, api.Watch(*o))
			return
		}
		state := r.URL.Query().Get("state")
		if state != "" && state != "active" && state != "all" {
			writeError(w, domain.Errf(domain.CodeInvalidArgument, "state must be active or all"))
			return
		}
		list, err := s.watches.List(r.Context(), by, r.URL.Query().Get("agent"), r.URL.Query().Get("project"), state == "all")
		if err != nil {
			writeError(w, err)
			return
		}
		out := api.WatchList{Watches: []api.Watch{}}
		for _, o := range list {
			out.Watches = append(out.Watches, api.Watch(*o))
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	var out *domain.Watch
	if id == "" {
		var req api.WatchStartRequest
		if !decode(w, r, &req) {
			return
		}
		out, err = s.watches.Start(r.Context(), by, app.WatchStart{What: req.What, Kind: req.Kind, Target: req.Target, Project: req.Project, Task: req.Task, ForSec: req.ForSec, Command: req.Command, Match: req.Match, EverySec: req.EverySec}, dry)
	} else if strings.HasSuffix(r.URL.Path, "/claim") {
		var req api.WatchClaimRequest
		if !decode(w, r, &req) {
			return
		}
		out, err = s.watches.ClaimCommand(r.Context(), by, id, req.Runner)
	} else if strings.HasSuffix(r.URL.Path, "/checks") {
		var req api.WatchCheckRequest
		if !decode(w, r, &req) {
			return
		}
		out, err = s.watches.CheckCommand(r.Context(), by, id, app.CommandWatchCheck{CheckID: req.CheckID, Runner: req.Runner, Result: req.Result, Matched: req.Matched, ExitCode: req.ExitCode})
	} else if strings.HasSuffix(r.URL.Path, "/stop") {
		var req api.WatchStopRequest
		if !decode(w, r, &req) {
			return
		}
		out, err = s.watches.Change(r.Context(), by, id, req.Reason, 0, true, dry)
	} else {
		var req api.WatchExtendRequest
		if !decode(w, r, &req) {
			return
		}
		out, err = s.watches.Change(r.Context(), by, id, "", req.ForSec, false, dry)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	status := http.StatusOK
	if id == "" && !dry {
		status = http.StatusCreated
	}
	writeJSON(w, status, api.Watch(*out))
}
