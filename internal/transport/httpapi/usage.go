package httpapi

import (
	"net/http"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/domain"
)

func (s *Server) SetUsage(u *app.UsageService) { s.usage = u }
func (s *Server) usageRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /v1/me/usage", s.agent(func(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
		if s.usage == nil {
			writeError(w, domain.Errf(domain.CodeUnsupported, "usage accounting is unavailable"))
			return
		}
		var req api.UsageRequest
		if !decode(w, r, &req) {
			return
		}
		n, err := s.usage.Record(r.Context(), a, req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, 200, api.UsageRecorded{Recorded: n})
	}))
	m.HandleFunc("GET /v1/console/usage", s.admin(func(w http.ResponseWriter, r *http.Request) {
		if s.usage == nil {
			writeError(w, domain.Errf(domain.CodeUnsupported, "usage accounting is unavailable"))
			return
		}
		q := r.URL.Query()
		out, err := s.usage.List(r.Context(), s.actorOf(r), domain.UsageFilter{From: q.Get("from"), To: q.Get("to"), Project: q.Get("project")})
		if err != nil {
			writeError(w, err)
			return
		}
		for i := range out.Agents {
			out.Agents[i].Owner = s.ownerName(out.Agents[i].Owner)
		}
		writeJSON(w, 200, out)
	}))
}
