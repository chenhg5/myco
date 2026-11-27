package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/domain"
)

func (s *Server) SetCoordinationStats(v *app.CoordinationService) { s.coordStats = v }
func (s *Server) routeCoordinationStats(m *http.ServeMux) {
	m.HandleFunc("GET /v1/stats/agents", s.adminOrAgent(s.agentStats))
	m.HandleFunc("GET /v1/stats/agent-pick", s.adminOrAgent(s.agentPick))
	m.HandleFunc("GET /v1/stats/coordinator", s.adminOrAgent(s.coordinatorStats))
}
func (s *Server) coordinationInput(r *http.Request) (domain.CoordinationFilter, []*domain.Agent, error) {
	f := domain.CoordinationFilter{Until: time.Now().UTC(), Project: r.URL.Query().Get("project")}
	if s.coordStats == nil {
		return f, nil, domain.Errf(domain.CodeUnsupported, "this Hub has no coordination statistics").WithSuggestion("upgrade the Hub")
	}
	var err error
	f.Since, err = app.StatsSince(r.URL.Query().Get("since"), f.Until)
	if err != nil {
		return f, nil, err
	}
	agents, err := s.agents.List(r.Context(), app.AgentFilter{})
	if err != nil {
		return f, nil, err
	}
	// Match the existing participant-message read boundary, independently of
	// the public agent directory. A public profile does not expose its history.
	if a, ok := r.Context().Value(callerAgentKey{}).(*domain.Agent); ok {
		f.Names = []domain.AgentName{a.Name}
	} else {
		sc, _, e := s.scopeOf(r)
		if e != nil {
			return f, nil, e
		}
		if !sc.All {
			f.Names = sc.Names
			if f.Names == nil {
				f.Names = []domain.AgentName{}
			}
		}
	}
	members := map[domain.AgentName]bool{}
	if f.Project != "" {
		by := s.actorOf(r)
		if a, ok := r.Context().Value(callerAgentKey{}).(*domain.Agent); ok {
			by = app.Actor{Agent: a}
		}
		if s.projects == nil {
			return f, nil, domain.Errf(domain.CodeUnsupported, "projects are unavailable").WithSuggestion("upgrade the Hub")
		}
		p, e := s.projects.Get(r.Context(), by, f.Project)
		if e != nil {
			return f, nil, e
		}
		f.Project = p.Project.Name
		for _, m := range p.Members {
			members[m.Agent] = true
		}
	}
	var wanted *domain.Agent
	if name := r.URL.Query().Get("name"); name != "" {
		wanted, err = s.agents.Get(r.Context(), name)
		if err != nil {
			return f, nil, err
		}
	}
	if wanted != nil {
		f.Agent = wanted.Name
	}
	out := []*domain.Agent{}
	v := s.viewerOf(r)
	for _, a := range agents {
		if !v.Sees(a) {
			continue
		}
		if wanted != nil && wanted.Name != a.Name {
			continue
		}
		if f.Project != "" && !members[a.Name] {
			continue
		}
		out = append(out, a)
	}
	if wanted != nil && len(out) == 0 {
		return f, nil, domain.Errf(domain.CodeNotFound, "agent not found in this scope").WithSuggestion("list visible agents or check --project")
	}
	return f, out, nil
}
func (s *Server) agentStats(w http.ResponseWriter, r *http.Request) {
	f, agents, err := s.coordinationInput(r)
	if err != nil {
		writeError(w, err)
		return
	}
	out, err := s.coordStats.Agents(r.Context(), f, agents)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
func (s *Server) agentPick(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	count := 3
	var err error
	if raw := q.Get("count"); raw != "" {
		count, err = strconv.Atoi(raw)
	}
	if err != nil || count < 1 || count > 100 {
		writeError(w, domain.Errf(domain.CodeInvalidArgument, "count must be 1..100").WithSuggestion("use --count 3"))
		return
	}
	strict := false
	if raw := q.Get("strict"); raw != "" {
		strict, err = strconv.ParseBool(raw)
		if err != nil {
			writeError(w, domain.Errf(domain.CodeInvalidArgument, "strict must be true or false"))
			return
		}
	}
	f, agents, err := s.coordinationInput(r)
	if err != nil {
		writeError(w, err)
		return
	}
	stats, err := s.coordStats.Agents(r.Context(), f, agents)
	if err != nil {
		writeError(w, err)
		return
	}
	if s.overview == nil {
		writeError(w, domain.Errf(domain.CodeUnsupported, "agent states unavailable").WithSuggestion("upgrade the Hub"))
		return
	}
	states, err := s.overview.States(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	// Pick cannot grant send permission: do not recommend someone who rejects
	// this caller, even if their public profile matches the requested skill.
	if by, ok := r.Context().Value(callerAgentKey{}).(*domain.Agent); ok {
		fit := agents[:0]
		for _, a := range agents {
			if domain.MaySendTo(by, a) && a.Accepts(by.Name) {
				fit = append(fit, a)
			}
		}
		agents = fit
	}
	// Apply paid-account visibility before ranking; an inaccessible quota
	// remains unknown and cannot affect the score through a side channel.
	for i, a := range agents {
		copy := *a
		copy.Runtime.Quota = quotaFromAPI(s.visibleQuota(r, a))
		agents[i] = &copy
	}
	machine := q.Get("machine")
	if machine != "" && s.machines != nil {
		views, _, e := s.machines.List(r.Context())
		if e != nil {
			writeError(w, e)
			return
		}
		for _, v := range views {
			if !v.Legacy && v.Name == machine {
				machine = v.ID
				break
			}
		}
	}
	out := app.RankAgents(agents, states, stats, app.PickFilter{Skill: q.Get("skill"), CLI: q.Get("cli"), Machine: machine, Count: count, Strict: strict}, f.Until)
	writeJSON(w, 200, out)
}
func (s *Server) coordinatorStats(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("name") == "" {
		if a, ok := r.Context().Value(callerAgentKey{}).(*domain.Agent); ok {
			q := r.URL.Query()
			q.Set("name", string(a.Name))
			r.URL.RawQuery = q.Encode()
		} else {
			writeError(w, domain.Errf(domain.CodeInvalidArgument, "name is required with admin/member credentials").WithSuggestion("use --name <coordinator>"))
			return
		}
	}
	f, agents, err := s.coordinationInput(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if len(agents) != 1 {
		writeError(w, domain.Errf(domain.CodeNotFound, "coordinator not found"))
		return
	}
	out, err := s.coordStats.Coordinator(r.Context(), f, string(agents[0].Name))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
