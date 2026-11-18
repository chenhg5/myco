package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/domain"
)

// SetPageData enables pages' records (/v1/pages/{id}/data, /changes).
func (s *Server) SetPageData(d *app.PageDataService) { s.pageData = d }

// SetTemplates enables the template library (/v1/templates).
func (s *Server) SetTemplates(t *app.TemplateService) { s.templates = t }

func (s *Server) pageDataRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /v1/pages/{id}/data", s.pageDataCaller(s.listRecords))
	m.HandleFunc("GET /v1/pages/{id}/data/{collection}/{record}", s.pageDataCaller(s.getRecord))
	m.HandleFunc("PUT /v1/pages/{id}/data/{collection}/{record}", s.pageDataCaller(s.putRecord))
	m.HandleFunc("POST /v1/pages/{id}/data/{collection}", s.pageDataCaller(s.putRecord))
	m.HandleFunc("DELETE /v1/pages/{id}/data/{collection}/{record}", s.pageDataCaller(s.deleteRecord))
	m.HandleFunc("GET /v1/pages/{id}/changes", s.pageDataCaller(s.pageChanges))
	m.HandleFunc("GET /v1/templates", s.templateCaller(s.listTemplates))
	m.HandleFunc("POST /v1/templates", s.templateCaller(s.publishTemplate))
	m.HandleFunc("GET /v1/templates/{id}", s.templateCaller(s.getTemplate))
	m.HandleFunc("DELETE /v1/templates/{id}", s.templateCaller(s.deleteTemplate))
	m.HandleFunc("POST /v1/templates/{id}/use", s.templateCaller(s.useTemplate))
}

func (s *Server) pageDataCaller(h func(http.ResponseWriter, *http.Request, pageCall)) http.HandlerFunc {
	inner := s.pageCaller(h)
	return func(w http.ResponseWriter, r *http.Request) {
		if s.pageData == nil {
			writeError(w, domain.Errf(domain.CodeNotFound, "pages have no records on this hub").
				WithSuggestion("upgrade the hub to v0.20.5 or later; until then use the page's state (myco page state)"))
			return
		}
		inner(w, r)
	}
}

func (s *Server) templateCaller(h func(http.ResponseWriter, *http.Request, pageCall)) http.HandlerFunc {
	inner := s.pageCaller(h)
	return func(w http.ResponseWriter, r *http.Request) {
		if s.templates == nil {
			writeError(w, domain.Errf(domain.CodeNotFound, "templates are not enabled on this hub").
				WithSuggestion("upgrade the hub to v0.20.5 or later"))
			return
		}
		inner(w, r)
	}
}

func recordDTO(r *domain.PageRecord) api.PageRecord {
	return api.PageRecord{Collection: r.Collection, ID: r.ID, Data: json.RawMessage(r.Data), Version: r.Version, UpdatedBy: r.UpdatedBy, UpdatedAt: r.UpdatedAt}
}

func (s *Server) listRecords(w http.ResponseWriter, r *http.Request, c pageCall) {
	recs, seq, err := s.pageData.Records(r.Context(), c.by, r.PathValue("id"), r.URL.Query().Get("collection"))
	if err != nil {
		writeError(w, err)
		return
	}
	out := api.PageRecords{Records: make([]api.PageRecord, 0, len(recs)), Seq: seq}
	for _, x := range recs {
		out.Records = append(out.Records, recordDTO(x))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getRecord(w http.ResponseWriter, r *http.Request, c pageCall) {
	rec, err := s.pageData.Record(r.Context(), c.by, r.PathValue("id"), r.PathValue("collection"), r.PathValue("record"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, recordDTO(rec))
}

func (s *Server) putRecord(w http.ResponseWriter, r *http.Request, c pageCall) {
	var req api.PageRecordWrite
	body := http.MaxBytesReader(w, r.Body, domain.MaxPageRecord+4096)
	if err := json.NewDecoder(body).Decode(&req); err != nil {
		writeError(w, domain.Errf(domain.CodeInvalidArgument, "the body is {\"data\": {…}, \"if_version\": n}"))
		return
	}
	in := app.PutInput{Collection: r.PathValue("collection"), ID: r.PathValue("record"), Data: string(req.Data), IfVersion: -1, By: c.name}
	if req.IfVersion != nil {
		in.IfVersion = *req.IfVersion
	}
	rec, err := s.pageData.Put(r.Context(), c.by, r.PathValue("id"), in)
	if err != nil {
		var de *domain.Error
		if rec != nil && errors.As(err, &de) && de.Code == domain.CodeConflict {
			writeJSON(w, http.StatusConflict, api.PageRecordConflict{
				Error:  api.Error{Code: string(de.Code), Message: de.Message, Suggestion: de.Suggestion},
				Latest: recordDTO(rec),
			})
			return
		}
		writeError(w, err)
		return
	}
	status := http.StatusOK
	if rec.Version == 1 {
		status = http.StatusCreated
	}
	writeJSON(w, status, recordDTO(rec))
}

func (s *Server) deleteRecord(w http.ResponseWriter, r *http.Request, c pageCall) {
	ifVersion := -1
	if raw := r.URL.Query().Get("if_version"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 {
			writeError(w, domain.Errf(domain.CodeInvalidArgument, "if_version is the version you read"))
			return
		}
		ifVersion = v
	}
	if err := s.pageData.Remove(r.Context(), c.by, r.PathValue("id"), r.PathValue("collection"), r.PathValue("record"), ifVersion, c.name); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// pageChanges: GET /v1/pages/{id}/changes?after=<seq>&wait=<seconds, up to 60>.
func (s *Server) pageChanges(w http.ResponseWriter, r *http.Request, c pageCall) {
	q := r.URL.Query()
	after, _ := strconv.ParseInt(q.Get("after"), 10, 64)
	wait, _ := strconv.Atoi(q.Get("wait"))
	wait = min(max(wait, 0), 60)
	cl, err := s.pageData.Changes(r.Context(), c.by, r.PathValue("id"), after, time.Duration(wait)*time.Second)
	if err != nil {
		writeError(w, err)
		return
	}
	out := api.PageChanges{Changes: make([]api.PageChange, 0, len(cl.Changes)), Seq: cl.Seq, Reset: cl.Reset}
	for _, x := range cl.Changes {
		ch := api.PageChange{Seq: x.Seq, Collection: x.Collection, Record: x.Record, Op: x.Op, Version: x.Version, By: x.By, At: x.At}
		if x.Data != "" {
			ch.Data = json.RawMessage(x.Data)
		}
		out.Changes = append(out.Changes, ch)
	}
	writeJSON(w, http.StatusOK, out)
}

func collectionsDTO(m map[string]domain.ManifestCollection) map[string]api.TemplateCollection {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]api.TemplateCollection, len(m))
	for k, v := range m {
		out[k] = api.TemplateCollection{Fields: v.Fields, Single: v.Single}
	}
	return out
}

// pageTemplateDTO: what a page made from a template says of it.
func pageTemplateDTO(p *domain.Page) *api.PageTemplate {
	if p.Template == "" || p.Manifest == "" {
		return nil
	}
	m, err := domain.ParseManifest([]byte(p.Manifest))
	if err != nil {
		return nil
	}
	return &api.PageTemplate{ID: p.Template, Slug: m.ID, Name: m.Name, Version: p.TemplateVersion, Data: collectionsDTO(m.Data), Permissions: m.Permissions}
}

func (s *Server) templateDTO(by app.Actor, tv *app.TemplateView) api.Template {
	t := tv.Template
	out := api.Template{ID: t.ID, Slug: t.Slug, Name: t.Name, Description: t.Description, Kind: t.Kind, Icon: t.Icon, Version: t.Latest,
		Publisher: t.Publisher, Guide: tv.Guide, Versions: []api.TemplateVersion{}, CanManage: s.templates.Manages(by, t), Official: t.Official,
		CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt}
	if tv.Manifest != nil {
		out.Data, out.Permissions, out.Export = collectionsDTO(tv.Manifest.Data), tv.Manifest.Permissions, tv.Manifest.Export
		out.Category = tv.Manifest.Category
	}
	for _, v := range tv.Versions {
		out.Versions = append(out.Versions, api.TemplateVersion{Version: v.Version, Files: v.Files, Size: v.Size, By: v.By, At: v.At})
	}
	return out
}

func (s *Server) listTemplates(w http.ResponseWriter, r *http.Request, c pageCall) {
	tvs, err := s.templates.List(r.Context(), c.by)
	if err != nil {
		writeError(w, err)
		return
	}
	out := api.TemplateList{Templates: make([]api.Template, 0, len(tvs))}
	for _, tv := range tvs {
		t := s.templateDTO(c.by, tv)
		t.Guide = "" // (the list stays short: get one for its guide)
		out.Templates = append(out.Templates, t)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getTemplate(w http.ResponseWriter, r *http.Request, c pageCall) {
	tv, err := s.templates.Get(r.Context(), c.by, r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.templateDTO(c.by, tv))
}

// publishTemplate: POST /v1/templates; the body is a zip of the template's
// folder (myco.json at its root).
func (s *Server) publishTemplate(w http.ResponseWriter, r *http.Request, c pageCall) {
	body := http.MaxBytesReader(w, r.Body, domain.MaxPageSize+1)
	tv, err := s.templates.Publish(r.Context(), c.by, body, c.name)
	if err != nil {
		writeError(w, err)
		return
	}
	status := http.StatusOK
	if len(tv.Versions) == 1 {
		status = http.StatusCreated
	}
	writeJSON(w, status, s.templateDTO(c.by, tv))
}

func (s *Server) deleteTemplate(w http.ResponseWriter, r *http.Request, c pageCall) {
	if _, err := s.templates.Delete(r.Context(), c.by, r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) useTemplate(w http.ResponseWriter, r *http.Request, c pageCall) {
	var req api.TemplateUse
	if !decode(w, r, &req) {
		return
	}
	pv, tv, err := s.templates.Use(r.Context(), c.by, r.PathValue("id"), app.UseInput{Name: req.Name, Project: req.Project, Version: req.Version, Sample: req.Sample, By: c.name})
	if err != nil && pv == nil {
		writeError(w, err)
		return
	}
	out := api.TemplateUsed{Page: s.pageDTO(r, c.by, pv), Template: s.templateDTO(c.by, tv)}
	writeJSON(w, http.StatusCreated, out)
}
