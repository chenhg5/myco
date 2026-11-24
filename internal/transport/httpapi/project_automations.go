package httpapi

import (
	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/domain"
	"net/http"
)

func (s *Server) SetProjectAutomations(v *app.ProjectAutomationService) { s.projectAutos = v }
func (s *Server) projectAutomationRoutes(m *http.ServeMux) {
	m.HandleFunc("PUT /v1/projects/{name}/automation-permission", s.projectCaller(func(w http.ResponseWriter, r *http.Request, c projectCall) {
		var req struct {
			Allowed bool `json:"allowed"`
		}
		if !decode(w, r, &req) {
			return
		}
		v, e := s.projects.SetCoordinatorAutomations(r.Context(), c.by, r.PathValue("name"), req.Allowed)
		if e != nil {
			writeError(w, e)
			return
		}
		s.recordAs(r, c.by, "project.automation.setting", v.Project.Name, "")
		writeJSON(w, 200, s.projectDTO(r, c, v, false))
	}))
	m.HandleFunc("GET /v1/projects/{name}/automations", s.projectCaller(func(w http.ResponseWriter, r *http.Request, c projectCall) {
		if s.projectAutos == nil {
			writeError(w, domain.Errf(domain.CodeUnsupported, "upgrade Hub for project automations"))
			return
		}
		list, e := s.projectAutos.List(r.Context(), c.by.Agent, r.PathValue("name"))
		if e != nil {
			writeError(w, e)
			return
		}
		out := api.AutomationList{Automations: []api.Automation{}}
		for _, a := range list {
			out.Automations = append(out.Automations, s.automationDTO(r, a))
		}
		writeJSON(w, 200, out)
	}))
	h := func(action string) http.HandlerFunc {
		return s.projectCaller(func(w http.ResponseWriter, r *http.Request, c projectCall) {
			if s.projectAutos == nil {
				writeError(w, domain.Errf(domain.CodeUnsupported, "upgrade Hub for project automations"))
				return
			}
			var req api.AutomationRequest
			if action != "delete" && !decode(w, r, &req) {
				return
			}
			o, e := s.projectAutos.Change(r.Context(), c.by.Agent, r.PathValue("name"), action, r.PathValue("id"), automationInput(req), dryRun(r))
			if e != nil {
				writeError(w, e)
				return
			}
			out := api.ProjectAutomationResponse{Status: o.Status, ApprovalID: o.ApprovalID, Removed: o.Removed}
			if o.Automation != nil {
				a := s.automationDTO(r, o.Automation)
				out.Automation = &a
			}
			status := 200
			if o.Status == "pending" {
				status = 202
			}
			writeJSON(w, status, out)
		})
	}
	m.HandleFunc("POST /v1/projects/{name}/automations", h("create"))
	m.HandleFunc("PATCH /v1/projects/{name}/automations/{id}", h("update"))
	m.HandleFunc("DELETE /v1/projects/{name}/automations/{id}", h("delete"))
}
