package httpapi

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/domain"
)

func (s *Server) resumeInfo(c *domain.ResumeCredential) api.ResumeInfo {
	return api.ResumeInfo{CreatedAt: c.CreatedAt, LastUsedAt: timePtr(c.LastUsedAt), LastUsedEnv: c.LastUsedEnv}
}

func (s *Server) attachResume(r *http.Request, a *domain.Agent, dto *api.Agent) {
	if s.resume == nil {
		return
	}
	if by, ok := r.Context().Value(callerAgentKey{}).(*domain.Agent); ok {
		if by.Name != a.Name {
			return
		}
		if c, err := s.resume.Get(r.Context(), a.Name); err == nil {
			info := s.resumeInfo(c)
			dto.Resume = &info
		}
		return
	}
	if !s.actorOf(r).Owns(a.Owner) {
		return
	}
	if c, err := s.resume.Get(r.Context(), a.Name); err == nil {
		info := s.resumeInfo(c)
		dto.Resume = &info
	}
}

func (s *Server) createMyResume(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	s.createResume(w, r, a.Name, app.Actor{Agent: a})
}

func (s *Server) createAgentResume(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !s.mayOwnAgent(w, r, name, "create a resume link for") {
		return
	}
	s.createResume(w, r, domain.AgentName(name), s.actorOf(r))
}

func (s *Server) createResume(w http.ResponseWriter, r *http.Request, name domain.AgentName, actor app.Actor) {
	if s.resume == nil {
		writeError(w, domain.Errf(domain.CodeNotFound, "resume links are unavailable"))
		return
	}
	c, token, err := s.resume.Create(r.Context(), name, r.URL.Query().Get("if_missing") == "true")
	if err != nil {
		writeError(w, err)
		return
	}
	out := api.ResumeLink{Token: token, Resume: s.resumeInfo(c)}
	if token != "" {
		out.URL = s.publicBase(r) + "/r/" + token
		out.Command = "curl -fsSL " + out.URL + " | sh"
		s.recordAs(r, actor, "agent.resume.create", string(name), "")
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) revokeAgentResume(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !s.mayOwnAgent(w, r, name, "revoke a resume link for") {
		return
	}
	if s.resume == nil {
		writeError(w, domain.Errf(domain.CodeNotFound, "resume links are unavailable"))
		return
	}
	if err := s.resume.Revoke(r.Context(), domain.AgentName(name)); err != nil {
		writeError(w, err)
		return
	}
	s.record(r, "agent.resume.revoke", name, "")
	writeJSON(w, http.StatusOK, map[string]any{"revoked": true})
}

func (s *Server) revokeMyResume(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	if s.resume == nil {
		writeError(w, domain.Errf(domain.CodeNotFound, "resume links are unavailable"))
		return
	}
	if err := s.resume.Revoke(r.Context(), a.Name); err != nil {
		writeError(w, err)
		return
	}
	s.recordAs(r, app.Actor{Agent: a}, "agent.resume.revoke", string(a.Name), "")
	writeJSON(w, http.StatusOK, map[string]any{"revoked": true})
}

func (s *Server) useResume(w http.ResponseWriter, r *http.Request) {
	if s.resume == nil {
		writeError(w, domain.Errf(domain.CodeNotFound, "resume links are unavailable"))
		return
	}
	var req api.ResumeRequest
	if !decode(w, r, &req) {
		return
	}
	a, newToken, err := s.resume.Use(r.Context(), req.Token, bindingFromAPI(req.Binding))
	if err != nil {
		writeError(w, err)
		return
	}
	s.recordAs(r, app.Actor{Agent: a}, "agent.resume.use", string(a.Name), "")
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, api.RegisterResponse{Agent: s.agentDTO(a), Token: newToken})
}

func (s *Server) resumePage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Robots-Tag", "noindex")
	if s.resume == nil {
		http.NotFound(w, r)
		return
	}
	token := r.PathValue("token")
	if _, err := s.resume.Peek(r.Context(), token); err != nil {
		http.NotFound(w, r)
		return
	}
	hub := s.publicBase(r)
	if r.URL.Query().Get("format") == "md" {
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		fmt.Fprintf(w, "# Reconnect this myco agent\n\nKeep this link private: it is an agent credential. Run:\n\n    curl -fsSL %s/r/%s | sh\n\nThe command installs myco, then restores the same agent identity.\n", hub, token)
		return
	}
	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	// Never echo the token, including in errors. It is embedded only in the
	// resume command and is never passed as an installer argument.
	fmt.Fprintf(w, `#!/usr/bin/env sh
set -eu
curl -fsSL --retry 5 --retry-delay 2 https://raw.githubusercontent.com/chenhg5/myco/main/deploy/install-client.sh | MYCO_NO_DOCTOR=1 MYCO_ADD_PATH=1 bash
MYCO="$(command -v myco 2>/dev/null || true)"
if [ -z "$MYCO" ]; then MYCO="${MYCO_INSTALL_DIR:-$HOME/.local/bin}/myco"; fi
"$MYCO" agent resume --token %s --hub %s --format text
`, shq(token), shq(strings.TrimRight(hub, "/")))
}
