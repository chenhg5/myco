package httpapi

import (
	"context"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/domain"
)

// Backups keeps the backups of the hub's own data (its database and
// files). Only the hub's owner (or its admin token) reaches them: a
// backup holds every conversation and file of the workspace.
type Backups interface {
	Info() (api.Backups, error)
	Create(ctx context.Context) (api.Backup, error)
	Open(name string) (*os.File, error)
}

// SetBackups offers the hub's backups (nil: none).
func (s *Server) SetBackups(b Backups) { s.backups = b }

func (s *Server) routeBackups(m *http.ServeMux) {
	m.HandleFunc("GET /v1/admin/backups", s.admin(s.listBackups))
	m.HandleFunc("POST /v1/admin/backups", s.admin(s.createBackup))
	m.HandleFunc("GET /v1/admin/backups/{name}", s.admin(s.getBackup))
}

// ownsBackups answers 403 unless the caller is the hub's owner in person
// (not admins, not a superagent acting for the owner), or the admin token.
func (s *Server) ownsBackups(w http.ResponseWriter, r *http.Request) bool {
	a := s.actorOf(r)
	if a.IsOwner() && a.Via == nil {
		if s.backups == nil {
			writeError(w, domain.Errf(domain.CodeInvalidState, "this hub keeps no backups").
				WithSuggestion("it runs without a database file, or with backups turned off (myco hub serve --backups)"))
			return false
		}
		return true
	}
	writeError(w, domain.Errf(domain.CodePermissionDenied, "only the hub's owner can make or download its backups"))
	return false
}

func (s *Server) listBackups(w http.ResponseWriter, r *http.Request) {
	if !s.ownsBackups(w, r) {
		return
	}
	out, err := s.backups.Info()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createBackup(w http.ResponseWriter, r *http.Request) {
	if !s.ownsBackups(w, r) {
		return
	}
	b, err := s.backups.Create(context.WithoutCancel(r.Context()))
	if err != nil {
		writeError(w, err)
		return
	}
	s.record(r, "hub.backup", b.Name, strconv.FormatInt(b.Size, 10)+" bytes")
	writeJSON(w, http.StatusCreated, b)
}

func (s *Server) getBackup(w http.ResponseWriter, r *http.Request) {
	if !s.ownsBackups(w, r) {
		return
	}
	f, err := s.backups.Open(r.PathValue("name"))
	if err != nil {
		writeError(w, err)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		writeError(w, err)
		return
	}
	s.record(r, "hub.backup_download", st.Name(), "")
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Length", strconv.FormatInt(st.Size(), 10))
	w.Header().Set("Content-Disposition", `attachment; filename="`+st.Name()+`"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.Copy(w, f)
}
