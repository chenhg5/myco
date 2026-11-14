package httpapi

import (
	"net/http"
	"strings"

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
	// Access is told apart from absent: older apps and the agent itself
	// leave it out, and it stays as it is.
	var in struct {
		Model  string  `json:"model"`
		Effort string  `json:"effort"`
		Access *string `json:"access"`
	}
	if !decode(w, r, &in) {
		return
	}
	cur, err := s.agents.Get(r.Context(), string(name))
	if err != nil {
		writeError(w, err)
		return
	}
	rs := domain.RunSettings{Model: strings.TrimSpace(in.Model), Effort: strings.TrimSpace(in.Effort), Access: cur.RunSettings.Access}
	if in.Access != nil && strings.TrimSpace(*in.Access) != cur.RunSettings.Access {
		// What an agent may do on someone's computer is that person's call:
		// not the agent's own, and not a hub admin's.
		if !ownsComputer(actor, cur) {
			writeError(w, domain.Errf(domain.CodePermissionDenied, "only the owner of %s can change its access", name).
				WithSuggestion("ask its owner; the machine itself also caps it (spawn.max_access in ~/.myco/machine.yaml)"))
			return
		}
		rs.Access = strings.TrimSpace(*in.Access)
	}
	a, err := s.agents.SetRunSettings(r.Context(), name, rs)
	if err != nil {
		writeError(w, err)
		return
	}
	detail := "model=" + orDash(rs.Model) + " effort=" + orDash(rs.Effort) + " access=" + orDash(rs.Access)
	s.recordAs(r, actor, "agent.run_settings", string(name), detail)
	writeJSON(w, http.StatusOK, s.agentDTO(a))
}

// ownsComputer: the member who owns agent a (or, for a node nobody owns,
// the hub's owner).
func ownsComputer(actor app.Actor, a *domain.Agent) bool {
	if actor.Agent != nil {
		return false
	}
	if a.Owner == "" {
		return actor.IsOwner()
	}
	return actor.MemberID() == a.Owner
}
