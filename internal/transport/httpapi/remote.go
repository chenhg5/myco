package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/myco-ai/myco/internal/domain"
	"github.com/myco-ai/myco/internal/remotedesktop"
)

// These routes require a person's credential, never an agent token. Owning a
// computer only permits signaling; the local component independently checks its
// pairing code and whether this user enabled control instead of view-only.
func (s *Server) remoteRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /v1/remote/machines", s.remoteConsole(func(w http.ResponseWriter, r *http.Request) {
		out := []remotedesktop.Device{}
		for _, d := range s.remote.Devices() {
			if strings.HasPrefix(d.Machine, "app:") {
				owner, ok := s.remote.Owner(d.Machine)
				if ok && s.actorOf(r).Owns(owner) {
					out = append(out, d)
				}
				continue
			}
			if s.machines == nil {
				continue
			}
			v, e := s.machines.Get(r.Context(), d.Machine)
			if e == nil && s.actorOf(r).Owns(v.Owner) {
				if v.Name != "" {
					d.Name = v.Name
				}
				if v.OS != "" {
					d.OS = v.OS
				}
				out = append(out, d)
			}
		}
		writeJSON(w, 200, map[string]any{"machines": out})
	}))
	m.HandleFunc("PUT /v1/remote/machines/{machine}", s.remoteConsole(func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("machine")
		if !s.remoteAllowed(w, r, id, true) {
			return
		}
		var in remotedesktop.Device
		if !decode(w, r, &in) {
			return
		}
		in.Machine = id
		if len(in.Name) > 160 || len(in.OS) > 32 {
			writeError(w, domain.Errf(domain.CodeInvalidArgument, "invalid remote computer name or platform"))
			return
		}
		key := s.remote.RegisterOwned(in, s.ownerIDFor(r.Context(), s.actorOf(r)))
		s.record(r, "remote.enable", id, "")
		writeJSON(w, 200, map[string]string{"host_key": key})
	}))
	m.HandleFunc("GET /v1/remote/machines/{machine}/requests", s.remoteConsole(func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("machine")
		if !s.remoteAllowed(w, r, id, false) {
			return
		}
		out, e := s.remote.Requests(id, r.Header.Get("X-Myco-Remote-Host"))
		if e != nil {
			remoteError(w, e)
			return
		}
		writeJSON(w, 200, map[string]any{"sessions": out})
	}))
	m.HandleFunc("DELETE /v1/remote/machines/{machine}", s.remoteConsole(func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("machine")
		if !s.remoteAllowed(w, r, id, false) {
			return
		}
		if e := s.remote.Unregister(id, r.Header.Get("X-Myco-Remote-Host")); e != nil {
			remoteError(w, e)
			return
		}
		s.record(r, "remote.disable", id, "")
		w.WriteHeader(204)
	}))
	m.HandleFunc("POST /v1/remote/sessions", s.remoteConsole(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Machine string              `json:"machine"`
			Offer   remotedesktop.Offer `json:"offer"`
		}
		if !decode(w, r, &in) {
			return
		}
		if !s.remoteAllowed(w, r, in.Machine, false) {
			return
		}
		if in.Offer.Description.Type != "offer" || len(in.Offer.Description.SDP) > 64<<10 || !strings.HasPrefix(in.Offer.Description.SDP, "v=0") || len(in.Offer.PairingCode) > 32 {
			writeError(w, domain.Errf(domain.CodeInvalidArgument, "invalid remote desktop offer"))
			return
		}
		out, e := s.remote.Connect(s.ownerIDFor(r.Context(), s.actorOf(r)), in.Machine, in.Offer)
		if e != nil {
			remoteError(w, e)
			return
		}
		s.record(r, "remote.connect", in.Machine, "")
		writeJSON(w, 201, out)
	}))
	for _, method := range []string{"GET", "DELETE"} {
		m.HandleFunc(method+" /v1/remote/sessions/{session}", s.remoteConsole(func(w http.ResponseWriter, r *http.Request) {
			out, e := s.remote.Session(s.ownerIDFor(r.Context(), s.actorOf(r)), r.PathValue("session"), r.Method == "DELETE")
			if e != nil {
				remoteError(w, e)
				return
			}
			if !s.remoteAllowed(w, r, out.Machine, false) {
				return
			}
			if r.Method == "DELETE" {
				s.record(r, "remote.disconnect", out.Machine, "")
			}
			writeJSON(w, 200, out)
		}))
	}
	m.HandleFunc("PUT /v1/remote/machines/{machine}/sessions/{session}", s.remoteConsole(func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("machine")
		if !s.remoteAllowed(w, r, id, false) {
			return
		}
		var in struct {
			Answer *remotedesktop.Description `json:"answer"`
			Error  string                     `json:"error"`
		}
		if !decode(w, r, &in) {
			return
		}
		if len(in.Error) > 300 || in.Answer != nil && (in.Answer.Type != "answer" || len(in.Answer.SDP) > 64<<10) {
			writeError(w, domain.Errf(domain.CodeInvalidArgument, "invalid remote desktop answer"))
			return
		}
		if e := s.remote.Answer(id, r.Header.Get("X-Myco-Remote-Host"), r.PathValue("session"), in.Answer, in.Error); e != nil {
			remoteError(w, e)
			return
		}
		w.WriteHeader(204)
	}))
}
func remoteError(w http.ResponseWriter, e error) {
	code := domain.CodeNotFound
	if errors.Is(e, remotedesktop.ErrDenied) {
		code = domain.CodePermissionDenied
	}
	if errors.Is(e, remotedesktop.ErrBusy) {
		code = domain.CodeConflict
	}
	writeError(w, domain.Errf(code, "%s", e))
}

// App-only hosts reuse the already paired App device identity. They need no
// CLI, machine.yaml, machine node or runner service to share a desktop.
func (s *Server) remoteAllowed(w http.ResponseWriter, r *http.Request, id string, register bool) bool {
	if !strings.HasPrefix(id, "app:") {
		return s.machinesEnabled(w) && s.mayOwnMachine(w, r, id, "use remote desktop on")
	}
	if register {
		if s.isMasterAdmin(r) {
			return true
		}
		if s.console != nil {
			if token, ok := s.console.Authenticate(r.Context(), bearer(r)); ok && id == "app:"+token.ID {
				return true
			}
		}
		writeError(w, domain.Errf(domain.CodePermissionDenied, "an App can only advertise its own device"))
		return false
	}
	owner, ok := s.remote.Owner(id)
	if !ok {
		remoteError(w, remotedesktop.ErrGone)
		return false
	}
	return s.mayOwn(w, r, owner, "use remote desktop on")
}

func (s *Server) remoteConsole(h http.HandlerFunc) http.HandlerFunc {
	return s.admin(func(w http.ResponseWriter, r *http.Request) {
		if superOf(r) != nil {
			writeError(w, domain.Errf(domain.CodePermissionDenied, "remote desktop requires an App credential, not an agent token"))
			return
		}
		h(w, r)
	})
}
