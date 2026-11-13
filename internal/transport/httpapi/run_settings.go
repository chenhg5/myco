package httpapi

import (
	"net/http"
	"strings"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/domain"
)

// setAgentRunSettings: PUT /v1/agents/{name}/run-settings from the console.
func (s *Server) setAgentRunSettings(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !s.mayOwnAgent(w, r, name, "choose the model of") {
		return
	}
	s.setRunSettings(w, r, domain.AgentName(name), s.actorOf(r))
}

// setMyRunSettings: PUT /v1/me/run-settings, the agent itself.
func (s *Server) setMyRunSettings(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	s.setRunSettings(w, r, a.Name, app.Actor{Agent: a})
}

func (s *Server) setRunSettings(w http.ResponseWriter, r *http.Request, name domain.AgentName, actor app.Actor) {
	var in api.RunSettings
	if !decode(w, r, &in) {
		return
	}
	rs := domain.RunSettings{Model: strings.TrimSpace(in.Model), Effort: strings.TrimSpace(in.Effort)}
	a, err := s.agents.SetRunSettings(r.Context(), name, rs)
	if err != nil {
		writeError(w, err)
		return
	}
	detail := "model=" + orDash(rs.Model) + " effort=" + orDash(rs.Effort)
	s.recordAs(r, actor, "agent.run_settings", string(name), detail)
	writeJSON(w, http.StatusOK, s.agentDTO(a))
}
