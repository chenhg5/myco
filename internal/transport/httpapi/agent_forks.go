package httpapi

import (
	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/domain"
	"net/http"
)

func (s *Server) SetForks(f *app.ForkService) { s.forks = f }
func (s *Server) forkActor(r *http.Request) app.Actor {
	if a, ok := r.Context().Value(callerAgentKey{}).(*domain.Agent); ok {
		return app.Actor{Agent: a}
	}
	return s.actorOf(r)
}
func (s *Server) forkEnabled(w http.ResponseWriter) bool {
	if s.forks != nil {
		return true
	}
	writeError(w, domain.Errf(domain.CodeUnsupported, "managed agent fork/retire is not enabled").WithSuggestion("upgrade the Hub"))
	return false
}
func (s *Server) createAgentFork(w http.ResponseWriter, r *http.Request) {
	if !s.forkEnabled(w) {
		return
	}
	var in api.AgentForkRequest
	if !decode(w, r, &in) {
		return
	}
	fs, err := s.forks.Fork(r.Context(), s.forkActor(r), s.viewerOf(r), app.ForkInput{From: in.From, Count: in.Count, Worktree: in.Worktree, NamePrefix: in.NamePrefix, ForTask: in.ForTask, Force: in.Force, RequestID: in.RequestID}, dryRun(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, api.AgentForkResult{Forks: fs, DryRun: dryRun(r)})
}
func (s *Server) retireAgentFork(w http.ResponseWriter, r *http.Request) {
	if !s.forkEnabled(w) {
		return
	}
	var in api.AgentRetireRequest
	if !decode(w, r, &in) {
		return
	}
	fs, err := s.forks.Retire(r.Context(), s.forkActor(r), app.RetireInput{Name: in.Name, ForkOf: in.ForkOf, Now: in.Now}, dryRun(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, api.AgentForkResult{Forks: fs, DryRun: dryRun(r)})
}
func (s *Server) getAgentFork(w http.ResponseWriter, r *http.Request) {
	if !s.forkEnabled(w) {
		return
	}
	f, err := s.forks.Get(r.Context(), s.forkActor(r), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, f)
}
func (s *Server) machineForkPlan(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	if !s.forkEnabled(w) {
		return
	}
	f, err := s.forks.MachinePlan(r.Context(), a, r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, f)
}
func (s *Server) completeAgentFork(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	if !s.forkEnabled(w) {
		return
	}
	var in api.AgentForkCompletion
	if !decode(w, r, &in) {
		return
	}
	f, err := s.forks.Complete(r.Context(), a, r.PathValue("id"), app.ForkCompletion{Workdir: in.Workdir, Repository: in.Repository, Branch: in.Branch, Retired: in.Retired, Error: in.Error})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, f)
}
