package httpapi

import (
	"net/http"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/domain"
)

// SetPins enables /v1/pins (what a conversation keeps at hand).
func (s *Server) SetPins(p *app.PinService) { s.pins = p }

func (s *Server) pinRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /v1/pins", s.adminOrAgent(s.listPins))
	m.HandleFunc("POST /v1/pins", s.adminOrAgent(s.addPin))
	m.HandleFunc("PUT /v1/pins/order", s.adminOrAgent(s.orderPins))
	m.HandleFunc("PATCH /v1/pins/{id}", s.adminOrAgent(s.renamePin))
	m.HandleFunc("DELETE /v1/pins/{id}", s.adminOrAgent(s.removePin))
}

// pinParties is whose conversation it is and who acts. An agent pins in
// its own conversations only; a person in their own (their console
// sender) or, by naming A, in one they may read.
func (s *Server) pinParties(r *http.Request, a, with string) (string, string, string, error) {
	if s.pins == nil {
		return "", "", "", domain.Errf(domain.CodeNotFound, "this hub does not keep pins").
			WithSuggestion("upgrade the hub to v0.20.2 or later")
	}
	if with == "" {
		return "", "", "", domain.Errf(domain.CodeInvalidArgument, "with whom: the conversation's other party is required").
			WithSuggestion("pass --with <agent or @console:name>")
	}
	if ag, ok := r.Context().Value(callerAgentKey{}).(*domain.Agent); ok {
		me := string(ag.Name)
		if a != "" && a != me {
			return "", "", "", domain.Errf(domain.CodePermissionDenied, "an agent pins in its own conversations only")
		}
		return me, with, me, nil
	}
	name, _ := s.consoleIdentity(r)
	by := string(app.ConsoleSender(name))
	if a == "" {
		a = by
	}
	sc, _, err := s.scopeOf(r)
	if err != nil {
		return "", "", "", err
	}
	if !sc.Allows(domain.AgentName(a), domain.AgentName(with)) {
		return "", "", "", domain.Errf(domain.CodePermissionDenied, "you may not see this conversation")
	}
	return a, with, by, nil
}

func pinDTO(p app.Pin) api.Pin {
	out := api.Pin{ID: p.ID, Kind: p.Kind, Title: p.Title, Target: p.Target, AddedBy: p.AddedBy, At: p.At}
	if p.File != nil {
		out.File = &api.Attachment{FileID: p.File.ID, Name: p.File.Name, Size: p.File.Size, SHA256: p.File.SHA256, ContentType: p.File.ContentType}
	}
	return out
}

func (s *Server) pinList(w http.ResponseWriter, r *http.Request, a, with string, ps []app.Pin) {
	out := api.PinList{A: a, With: with, Pins: make([]api.Pin, 0, len(ps)), Max: app.MaxPins}
	for _, p := range ps {
		out.Pins = append(out.Pins, pinDTO(p))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) listPins(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	a, with, _, err := s.pinParties(r, q.Get("a"), q.Get("with"))
	if err != nil {
		writeError(w, err)
		return
	}
	ps, err := s.pins.Pins(r.Context(), a, with)
	if err != nil {
		writeError(w, err)
		return
	}
	s.pinList(w, r, a, with, ps)
}

func (s *Server) addPin(w http.ResponseWriter, r *http.Request) {
	var req api.PinCreate
	if !decode(w, r, &req) {
		return
	}
	a, with, by, err := s.pinParties(r, req.A, req.With)
	if err != nil {
		writeError(w, err)
		return
	}
	in := app.AddPin{Kind: req.Kind, Title: req.Title, Target: req.Target, By: by}
	if req.File != nil {
		in.File = &app.PinFile{ID: req.File.FileID, Name: req.File.Name, Size: req.File.Size, SHA256: req.File.SHA256, ContentType: req.File.ContentType}
	}
	p, err := s.pins.Add(r.Context(), a, with, in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, pinDTO(p))
}

func (s *Server) renamePin(w http.ResponseWriter, r *http.Request) {
	var req api.PinUpdate
	if !decode(w, r, &req) {
		return
	}
	a, with, _, err := s.pinParties(r, req.A, req.With)
	if err != nil {
		writeError(w, err)
		return
	}
	p, err := s.pins.Rename(r.Context(), a, with, r.PathValue("id"), req.Title)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, pinDTO(p))
}

func (s *Server) removePin(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	a, with, _, err := s.pinParties(r, q.Get("a"), q.Get("with"))
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.pins.Remove(r.Context(), a, with, r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) orderPins(w http.ResponseWriter, r *http.Request) {
	var req api.PinOrder
	if !decode(w, r, &req) {
		return
	}
	a, with, _, err := s.pinParties(r, req.A, req.With)
	if err != nil {
		writeError(w, err)
		return
	}
	ps, err := s.pins.Order(r.Context(), a, with, req.IDs)
	if err != nil {
		writeError(w, err)
		return
	}
	s.pinList(w, r, a, with, ps)
}
