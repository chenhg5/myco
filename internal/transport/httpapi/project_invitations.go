package httpapi

import (
	"net/http"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/domain"
)

func (s *Server) SetProjectInvitations(v *app.ProjectInvitationService) { s.projectInvites = v }
func (s *Server) projectInvitationRoutes(m *http.ServeMux) {
	m.HandleFunc("PUT /v1/projects/{name}/agent-invites", s.projectCaller(func(w http.ResponseWriter, r *http.Request, c projectCall) {
		var req struct {
			Allowed bool `json:"allowed"`
		}
		if !decode(w, r, &req) {
			return
		}
		v, err := s.projects.SetCoordinatorInvites(r.Context(), c.by, r.PathValue("name"), req.Allowed)
		if err != nil {
			writeError(w, err)
			return
		}
		s.recordAs(r, c.by, "project.invitation.setting", v.Project.Name, "")
		writeJSON(w, 200, s.projectDTO(r, c, v, false))
	}))
	handler := func(action string) http.HandlerFunc {
		return s.projectCaller(func(w http.ResponseWriter, r *http.Request, c projectCall) {
			w.Header().Set("Cache-Control", "no-store")
			if s.projectInvites == nil {
				writeError(w, domain.Errf(domain.CodeUnsupported, "project invitations unavailable"))
				return
			}
			var v *domain.ProjectInvitation
			var token string
			var err error
			switch action {
			case "create":
				var req api.ProjectInviteRequest
				if !decode(w, r, &req) {
					return
				}
				v, token, err = s.projectInvites.Create(r.Context(), c.by.Agent, r.PathValue("name"), app.ProjectInviteInput{DisplayName: req.DisplayName, Description: req.Description, Note: req.Note}, dryRun(r))
			case "status":
				v, err = s.projectInvites.Status(r.Context(), c.by.Agent, r.PathValue("name"), r.PathValue("invitation"))
			case "claim":
				v, token, err = s.projectInvites.Claim(r.Context(), c.by.Agent, r.PathValue("name"), r.PathValue("invitation"))
			}
			if err != nil {
				writeError(w, err)
				return
			}
			out := api.ProjectInviteResponse{ID: v.ID, ApprovalID: v.ApprovalID, ExpiresAt: v.ExpiresAt, Status: v.Status}
			if token != "" {
				out.Link = s.publicBase(r) + "/j/" + token
				out.Sentence = "Join myco: " + out.Link
			}
			status := 200
			if v.Status == "pending" {
				status = 202
			}
			writeJSON(w, status, out)
		})
	}
	m.HandleFunc("POST /v1/projects/{name}/invite-agent", handler("create"))
	m.HandleFunc("GET /v1/projects/{name}/invite-agent/{invitation}", handler("status"))
	m.HandleFunc("POST /v1/projects/{name}/invite-agent/{invitation}/claim", handler("claim"))
}
