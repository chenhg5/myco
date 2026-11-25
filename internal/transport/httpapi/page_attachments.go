package httpapi

import (
	"bytes"
	"io"
	"mime"
	"net/http"
	"strconv"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/domain"
)

func (s *Server) SetPageAttachments(a *app.PageAttachmentService) { s.pageAttachments = a }
func (s *Server) pageAttachmentRoutes(m *http.ServeMux) {
	wrap := func(h func(http.ResponseWriter, *http.Request, pageCall)) http.HandlerFunc {
		return s.pageCaller(func(w http.ResponseWriter, r *http.Request, c pageCall) {
			if s.pageAttachments == nil {
				writeError(w, domain.Errf(domain.CodeUnsupported, "page attachments are not supported"))
				return
			}
			h(w, r, c)
		})
	}
	m.HandleFunc("GET /v1/pages/{id}/attachments", wrap(s.listPageAttachments))
	m.HandleFunc("POST /v1/pages/{id}/attachments", wrap(s.putPageAttachment))
	m.HandleFunc("GET /v1/pages/{id}/attachments/{attachment}", wrap(s.getPageAttachment))
	m.HandleFunc("GET /v1/pages/{id}/attachments/{attachment}/content", wrap(s.pageAttachmentContent))
	m.HandleFunc("DELETE /v1/pages/{id}/attachments/{attachment}", wrap(s.removePageAttachment))
	m.HandleFunc("POST /v1/page-attachments/migrate", wrap(s.migratePageAttachments))
}
func (s *Server) listPageAttachments(w http.ResponseWriter, r *http.Request, c pageCall) {
	page := r.PathValue("id")
	usage, err := s.pageAttachments.Usage(r.Context(), c.by, page)
	if err != nil {
		writeError(w, err)
		return
	}
	out := api.PageAttachmentList{AttachmentVersion: 1, Usage: usage, Attachments: []*api.PageAttachment{}}
	if name := r.URL.Query().Get("name"); name != "" {
		a, e := s.pageAttachments.Resolve(r.Context(), c.by, page, name)
		if e != nil {
			writeError(w, e)
			return
		}
		out.Attachments = append(out.Attachments, a)
	} else {
		limit := 100
		if raw := r.URL.Query().Get("limit"); raw != "" {
			limit, err = strconv.Atoi(raw)
			if err != nil || limit < 1 || limit > 1000 {
				writeError(w, domain.Errf(domain.CodeInvalidArgument, "limit must be 1..1000"))
				return
			}
		}
		out.Attachments, err = s.pageAttachments.List(r.Context(), c.by, page, r.URL.Query().Get("after"), limit+1)
		if err != nil {
			writeError(w, err)
			return
		}
		if len(out.Attachments) > limit {
			out.Attachments = out.Attachments[:limit]
			out.Next = out.Attachments[limit-1].ID
		}
	}
	writeJSON(w, http.StatusOK, out)
}
func (s *Server) putPageAttachment(w http.ResponseWriter, r *http.Request, c pageCall) {
	q := r.URL.Query()
	a, err := s.pageAttachments.Put(r.Context(), c.by, app.PageAttachmentInput{Page: r.PathValue("id"), Collection: q.Get("collection"), Name: q.Get("name"), ContentType: r.Header.Get("Content-Type"), Key: r.Header.Get("Idempotency-Key"), LegacyName: q.Get("legacy_name"), By: c.by.Name()}, r.Body)
	if err != nil {
		writeError(w, err)
		return
	}
	s.recordAs(r, c.by, "page.attachment_put", a.Page, a.ID)
	writeJSON(w, http.StatusCreated, a)
}
func (s *Server) getPageAttachment(w http.ResponseWriter, r *http.Request, c pageCall) {
	a, err := s.pageAttachments.Get(r.Context(), c.by, r.PathValue("id"), r.PathValue("attachment"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}
func (s *Server) removePageAttachment(w http.ResponseWriter, r *http.Request, c pageCall) {
	if err := s.pageAttachments.Remove(r.Context(), c.by, r.PathValue("id"), r.PathValue("attachment")); err != nil {
		writeError(w, err)
		return
	}
	s.recordAs(r, c.by, "page.attachment_remove", r.PathValue("id"), r.PathValue("attachment"))
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) pageAttachmentContent(w http.ResponseWriter, r *http.Request, c pageCall) {
	a, rc, err := s.pageAttachments.Open(r.Context(), c.by, r.PathValue("id"), r.PathValue("attachment"))
	if err != nil {
		writeError(w, err)
		return
	}
	defer rc.Close()
	var content io.ReadSeeker
	if seek, ok := rc.(io.ReadSeeker); ok {
		content = seek
	} else {
		data, e := io.ReadAll(io.LimitReader(rc, domain.MaxPageAttachment+1))
		if e != nil || int64(len(data)) != a.Size {
			writeError(w, domain.Errf(domain.CodeInvalidState, "cannot read page attachment"))
			return
		}
		content = bytes.NewReader(data)
	}
	w.Header().Set("Content-Type", a.ContentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": a.Name}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	w.Header().Set("ETag", `"`+a.SHA256+`"`)
	w.Header().Set("Cache-Control", "private, no-store")
	http.ServeContent(w, r, a.Name, a.CreatedAt, content)
}
func (s *Server) migratePageAttachments(w http.ResponseWriter, r *http.Request, c pageCall) {
	var in api.PageAttachmentMigrationRequest
	if !decode(w, r, &in) {
		return
	}
	if in.Limit < 0 || in.Limit > 100 {
		writeError(w, domain.Errf(domain.CodeInvalidArgument, "limit must be 1..100"))
		return
	}
	out, err := s.pageAttachments.Migrate(r.Context(), c.by, in.Project, in.DryRun, in.Limit)
	if err != nil {
		writeError(w, err)
		return
	}
	if !in.DryRun {
		s.recordAs(r, c.by, "page.attachments_migrate", in.Project, strconv.Itoa(out.Moved))
	}
	writeJSON(w, http.StatusOK, out)
}
