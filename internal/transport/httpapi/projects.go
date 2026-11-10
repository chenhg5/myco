package httpapi

import (
	"net/http"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/domain"
)

// Projects: a project groups nodes around a goal with a shared brief,
// documents and links. People (console or admin token) and agents (their
// own token) both use these endpoints; ProjectService decides who may do
// what.

func (s *Server) routeProjects(m *http.ServeMux) {
	m.HandleFunc("GET /v1/projects", s.projectCaller(s.listProjects))
	m.HandleFunc("POST /v1/projects", s.projectCaller(s.createProject))
	m.HandleFunc("GET /v1/projects/{name}", s.projectCaller(s.getProject))
	m.HandleFunc("PATCH /v1/projects/{name}", s.projectCaller(s.updateProject))
	m.HandleFunc("DELETE /v1/projects/{name}", s.projectCaller(s.deleteProject))
	m.HandleFunc("PUT /v1/projects/{name}/members/{agent}", s.projectCaller(s.putProjectMember))
	m.HandleFunc("DELETE /v1/projects/{name}/members/{agent}", s.projectCaller(s.removeProjectMember))
	m.HandleFunc("GET /v1/projects/{name}/docs/{doc}", s.projectCaller(s.getProjectDoc))
	m.HandleFunc("PUT /v1/projects/{name}/docs/{doc}", s.projectCaller(s.putProjectDoc))
	m.HandleFunc("DELETE /v1/projects/{name}/docs/{doc}", s.projectCaller(s.deleteProjectDoc))
}

// projectCall is who calls: a person (console/admin token) or an agent.
type projectCall struct {
	by     app.Actor
	viewer domain.Viewer
}

func (s *Server) projectCaller(h func(w http.ResponseWriter, r *http.Request, c projectCall)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.projects == nil {
			writeError(w, domain.Errf(domain.CodeNotFound, "this hub has no projects"))
			return
		}
		if s.isAdmin(r) {
			h(w, r, projectCall{by: s.actorOf(r), viewer: s.viewerOf(r)})
			return
		}
		s.agent(func(w http.ResponseWriter, r *http.Request, a *domain.Agent) {
			h(w, r, projectCall{by: app.Actor{Agent: a}, viewer: domain.Viewer{Agent: a}})
		})(w, r)
	}
}

// seesProject checks that the person calling (from the app) can see a project.
func (s *Server) seesProject(r *http.Request, name string) error {
	if s.projects == nil {
		return domain.Errf(domain.CodeInvalidArgument, "this hub has no projects")
	}
	_, err := s.projects.Get(r.Context(), s.actorOf(r), name)
	return err
}

func (s *Server) recordAs(r *http.Request, by app.Actor, action, target, detail string) {
	if s.audit != nil {
		s.audit.Record(r.Context(), by, action, target, detail)
	}
}

func (s *Server) projectDTO(r *http.Request, c projectCall, v *app.ProjectView, bodies bool) api.Project {
	p := v.Project
	out := api.Project{Name: p.Name, Title: p.Title, Goal: p.Goal, Brief: p.Brief, Visibility: string(p.Visibility),
		OwnerID: p.Owner, Archived: p.Archived, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
		Links: make([]api.ProjectLink, 0, len(p.Links)), Members: make([]api.ProjectMember, 0, len(v.Members)), Docs: make([]api.ProjectDoc, 0, len(v.Docs))}
	if p.Owner != "" && s.members != nil {
		if m, err := s.members.Get(r.Context(), p.Owner); err == nil {
			out.Owner = m.Name
		}
	}
	for _, l := range p.Links {
		out.Links = append(out.Links, api.ProjectLink{Title: l.Title, URL: l.URL})
	}
	for _, m := range v.Members {
		pm := api.ProjectMember{Name: string(m.Agent), Role: string(m.Role), Note: m.Note, AddedBy: m.AddedBy, AddedAt: m.AddedAt}
		if a, err := s.agents.Get(r.Context(), string(m.Agent)); err == nil && c.viewer.Sees(a) {
			dto := s.agentDTO(a)
			s.enrich(r.Context(), &dto)
			pm.Agent = &dto
		}
		out.Members = append(out.Members, pm)
	}
	for _, d := range v.Docs {
		doc := api.ProjectDoc{Name: d.Name, Size: len(d.Body), UpdatedBy: d.UpdatedBy, UpdatedAt: d.UpdatedAt}
		if bodies {
			doc.Body = d.Body
		}
		out.Docs = append(out.Docs, doc)
	}
	out.CanEdit, out.CanManage = s.projects.Rights(c.by, v)
	return out
}

func (s *Server) listProjects(w http.ResponseWriter, r *http.Request, c projectCall) {
	list, err := s.projects.List(r.Context(), c.by, r.URL.Query().Get("mine") == "true" || r.URL.Query().Get("mine") == "1")
	if err != nil {
		writeError(w, err)
		return
	}
	out := api.ProjectList{Projects: make([]api.Project, 0, len(list))}
	for _, v := range list {
		out.Projects = append(out.Projects, s.projectDTO(r, c, v, false))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request, c projectCall) {
	var req api.CreateProjectRequest
	if !decode(w, r, &req) {
		return
	}
	v, err := s.projects.Create(r.Context(), c.by, s.ownerIDFor(r.Context(), c.by), req.Name, req.Title, req.Goal, req.Visibility)
	if err != nil {
		writeError(w, err)
		return
	}
	s.recordAs(r, c.by, "project.create", v.Project.Name, v.Project.Title)
	writeJSON(w, http.StatusCreated, s.projectDTO(r, c, v, false))
}

func (s *Server) getProject(w http.ResponseWriter, r *http.Request, c projectCall) {
	v, err := s.projects.Get(r.Context(), c.by, r.PathValue("name"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.projectDTO(r, c, v, r.URL.Query().Get("docs") == "body"))
}

func (s *Server) updateProject(w http.ResponseWriter, r *http.Request, c projectCall) {
	var req api.UpdateProjectRequest
	if !decode(w, r, &req) {
		return
	}
	name := r.PathValue("name")
	var (
		v   *app.ProjectView
		err error
	)
	if req.Visibility != nil || req.Archived != nil {
		if v, err = s.projects.Settle(r.Context(), c.by, name, req.Visibility, req.Archived); err != nil {
			writeError(w, err)
			return
		}
		s.recordAs(r, c.by, "project.settle", v.Project.Name, string(v.Project.Visibility)+map[bool]string{true: ", archived", false: ""}[v.Project.Archived])
	}
	if req.Title != nil || req.Goal != nil || req.Brief != nil || req.Links != nil {
		e := domain.ProjectEdit{Title: req.Title, Goal: req.Goal, Brief: req.Brief}
		if req.Links != nil {
			links := make([]domain.ProjectLink, 0, len(*req.Links))
			for _, l := range *req.Links {
				links = append(links, domain.ProjectLink{Title: l.Title, URL: l.URL})
			}
			e.Links = &links
		}
		if v, err = s.projects.Edit(r.Context(), c.by, name, e); err != nil {
			writeError(w, err)
			return
		}
		var what []string
		for _, f := range []struct {
			set  bool
			name string
		}{{req.Title != nil, "title"}, {req.Goal != nil, "goal"}, {req.Brief != nil, "brief"}, {req.Links != nil, "links"}} {
			if f.set {
				what = append(what, f.name)
			}
		}
		s.recordAs(r, c.by, "project.edit", v.Project.Name, joinComma(what))
	}
	if v == nil {
		if v, err = s.projects.Get(r.Context(), c.by, name); err != nil {
			writeError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, s.projectDTO(r, c, v, false))
}

func joinComma(xs []string) string {
	out := ""
	for i, x := range xs {
		if i > 0 {
			out += ", "
		}
		out += x
	}
	return out
}

func (s *Server) deleteProject(w http.ResponseWriter, r *http.Request, c projectCall) {
	v, err := s.projects.Delete(r.Context(), c.by, r.PathValue("name"))
	if err != nil {
		writeError(w, err)
		return
	}
	s.recordAs(r, c.by, "project.delete", v.Project.Name, "")
	writeJSON(w, http.StatusOK, map[string]any{"project": v.Project.Name, "deleted": true})
}

func (s *Server) putProjectMember(w http.ResponseWriter, r *http.Request, c projectCall) {
	var req api.ProjectMemberRequest
	if !decode(w, r, &req) {
		return
	}
	agent := r.PathValue("agent")
	v, m, err := s.projects.PutMember(r.Context(), c.by, r.PathValue("name"), agent, app.MemberEdit{Role: req.Role, Note: req.Note}, c.by.Name())
	if err != nil {
		writeError(w, err)
		return
	}
	s.recordAs(r, c.by, "project.member", v.Project.Name, string(m.Agent)+" "+string(m.Role))
	writeJSON(w, http.StatusOK, s.projectDTO(r, c, v, false))
}

func (s *Server) removeProjectMember(w http.ResponseWriter, r *http.Request, c projectCall) {
	v, err := s.projects.RemoveMember(r.Context(), c.by, r.PathValue("name"), r.PathValue("agent"))
	if err != nil {
		writeError(w, err)
		return
	}
	s.recordAs(r, c.by, "project.member_remove", v.Project.Name, r.PathValue("agent"))
	writeJSON(w, http.StatusOK, s.projectDTO(r, c, v, false))
}

func (s *Server) getProjectDoc(w http.ResponseWriter, r *http.Request, c projectCall) {
	v, err := s.projects.Get(r.Context(), c.by, r.PathValue("name"))
	if err != nil {
		writeError(w, err)
		return
	}
	n, err := domain.ParseDocName(r.PathValue("doc"))
	if err != nil {
		writeError(w, err)
		return
	}
	for _, d := range v.Docs {
		if d.Name == n {
			writeJSON(w, http.StatusOK, api.ProjectDoc{Name: d.Name, Size: len(d.Body), UpdatedBy: d.UpdatedBy, UpdatedAt: d.UpdatedAt, Body: d.Body})
			return
		}
	}
	writeError(w, domain.Errf(domain.CodeNotFound, "document %s not found in project %s", n, v.Project.Name))
}

func (s *Server) putProjectDoc(w http.ResponseWriter, r *http.Request, c projectCall) {
	var req api.ProjectDocRequest
	if !decode(w, r, &req) {
		return
	}
	d, err := s.projects.PutDoc(r.Context(), c.by, r.PathValue("name"), r.PathValue("doc"), req.Body, c.by.Name())
	if err != nil {
		writeError(w, err)
		return
	}
	s.recordAs(r, c.by, "project.doc", r.PathValue("name"), d.Name)
	writeJSON(w, http.StatusOK, api.ProjectDoc{Name: d.Name, Size: len(d.Body), UpdatedBy: d.UpdatedBy, UpdatedAt: d.UpdatedAt})
}

func (s *Server) deleteProjectDoc(w http.ResponseWriter, r *http.Request, c projectCall) {
	if err := s.projects.DeleteDoc(r.Context(), c.by, r.PathValue("name"), r.PathValue("doc")); err != nil {
		writeError(w, err)
		return
	}
	s.recordAs(r, c.by, "project.doc_remove", r.PathValue("name"), r.PathValue("doc"))
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}
