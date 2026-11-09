package httpapi

import (
	"net/http"
	"strings"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/domain"
)

// SetE2E enables the end-to-end encryption endpoints (public keys, readers,
// the hub's setting).
func (s *Server) SetE2E(e *app.E2EService) { s.e2e = e }

func (s *Server) routesE2E(m *http.ServeMux) {
	m.HandleFunc("GET /v1/hub/settings", s.adminOrAgent(s.getHubSettings))
	m.HandleFunc("PUT /v1/hub/settings", s.admin(s.putHubSettings))
	m.HandleFunc("PUT /v1/me/keys", s.agent(s.putAgentKeys))
	m.HandleFunc("PUT /v1/console/me/keys", s.admin(s.putDeviceKeys))
	m.HandleFunc("GET /v1/keys", s.adminOrAgent(s.getKeys))
	m.HandleFunc("GET /v1/keys/readers", s.adminOrAgent(s.getReaders))
}

func (s *Server) e2eOn(w http.ResponseWriter) bool {
	if s.e2e == nil {
		writeError(w, domain.Errf(domain.CodeNotFound, "this hub does not support end-to-end encryption").WithSuggestion("upgrade the hub: sudo myco upgrade"))
		return false
	}
	return true
}

func (s *Server) getHubSettings(w http.ResponseWriter, r *http.Request) {
	if !s.e2eOn(w) {
		return
	}
	writeJSON(w, http.StatusOK, api.HubSettings{E2E: s.e2e.Enabled(r.Context())})
}

// putHubSettings: only the hub's owner (or the hub's own admin token).
func (s *Server) putHubSettings(w http.ResponseWriter, r *http.Request) {
	if !s.e2eOn(w) {
		return
	}
	var req api.HubSettings
	if !decode(w, r, &req) {
		return
	}
	if !s.isMasterAdmin(r) && !s.actorOf(r).IsOwner() {
		writeError(w, domain.Errf(domain.CodePermissionDenied, "only the hub's owner can change end-to-end encryption"))
		return
	}
	if err := s.e2e.SetEnabled(r.Context(), req.E2E); err != nil {
		writeError(w, err)
		return
	}
	detail := "off"
	if req.E2E {
		detail = "on"
	}
	s.record(r, "hub.e2e", "", detail)
	writeJSON(w, http.StatusOK, req)
}

func (s *Server) putAgentKeys(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
	if !s.e2eOn(w) {
		return
	}
	var req api.PutKeysRequest
	if !decode(w, r, &req) {
		return
	}
	if err := s.e2e.PutKey(r.Context(), domain.KeyAgent, string(a.Name), req.EncKey, req.SignKey); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, req)
}

// putDeviceKeys records the calling device's keys (a console token: the
// app, or a CLI that accepted an invite).
func (s *Server) putDeviceKeys(w http.ResponseWriter, r *http.Request) {
	if !s.e2eOn(w) {
		return
	}
	var req api.PutKeysRequest
	if !decode(w, r, &req) {
		return
	}
	if s.console == nil || s.isMasterAdmin(r) {
		writeError(w, domain.Errf(domain.CodeInvalidArgument, "only a device (an app or a member's CLI) has keys; the hub's admin token does not"))
		return
	}
	t, ok := s.console.Authenticate(r.Context(), bearer(r))
	if !ok {
		writeError(w, domain.Errf(domain.CodeUnauthenticated, "console token missing or invalid"))
		return
	}
	if err := s.e2e.PutKey(r.Context(), domain.KeyDevice, t.ID, req.EncKey, req.SignKey); err != nil {
		writeError(w, err)
		return
	}
	s.record(r, "device.keys", t.Name, "")
	writeJSON(w, http.StatusOK, req)
}

func holderDTO(h app.KeyHolder) api.KeyHolder {
	return api.KeyHolder{Who: h.Who, Role: h.Role, Device: h.Device, EncKey: h.Key.Enc, SignKey: h.Key.Sign}
}

func (s *Server) getKeys(w http.ResponseWriter, r *http.Request) {
	if !s.e2eOn(w) {
		return
	}
	out := api.KeyList{E2E: s.e2e.Enabled(r.Context()), Keys: map[string][]api.KeyHolder{}}
	for _, who := range strings.Split(r.URL.Query().Get("who"), ",") {
		if who = strings.TrimSpace(who); who == "" {
			continue
		}
		hs, err := s.e2e.KeysOf(r.Context(), who)
		if err != nil {
			writeError(w, err)
			return
		}
		list := make([]api.KeyHolder, 0, len(hs))
		for _, h := range hs {
			list = append(list, holderDTO(h))
		}
		out.Keys[who] = list
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getReaders(w http.ResponseWriter, r *http.Request) {
	if !s.e2eOn(w) {
		return
	}
	q := r.URL.Query()
	from, to := strings.TrimSpace(q.Get("from")), strings.TrimSpace(q.Get("to"))
	if from == "" || to == "" {
		writeError(w, domain.Errf(domain.CodeInvalidArgument, "from and to are required"))
		return
	}
	readers, missing, err := s.e2e.Readers(r.Context(), from, to)
	if err != nil {
		writeError(w, err)
		return
	}
	out := api.Readers{E2E: s.e2e.Enabled(r.Context()), Readers: make([]api.KeyHolder, 0, len(readers)), Missing: missing}
	for _, h := range readers {
		out.Readers = append(out.Readers, holderDTO(h))
	}
	writeJSON(w, http.StatusOK, out)
}
