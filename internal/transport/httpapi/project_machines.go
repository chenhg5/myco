package httpapi

import (
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/domain"
)

// Which computers a project's coordinators may start agents on: a setting
// of the project (its owner chooses), apart from the terminals in it. A
// coordinator sees the computers and which are allowed, and asks the owner
// for another (an approval card in its chat).
func (s *Server) projectMachineRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /v1/projects/{name}/machines", s.projectCaller(s.listProjectMachines))
	m.HandleFunc("PUT /v1/projects/{name}/machines/{machine}", s.projectCaller(func(w http.ResponseWriter, r *http.Request, c projectCall) {
		s.setProjectMachine(w, r, c, true)
	}))
	m.HandleFunc("DELETE /v1/projects/{name}/machines/{machine}", s.projectCaller(func(w http.ResponseWriter, r *http.Request, c projectCall) {
		s.setProjectMachine(w, r, c, false)
	}))
	m.HandleFunc("POST /v1/projects/{name}/machines/{machine}/request", s.projectCaller(s.requestProjectMachine))
}

// projectMachines lists the workspace's computers (not legacy groups),
// marking those the project allows.
func (s *Server) projectMachines(r *http.Request, allowed []string) ([]api.ProjectMachine, error) {
	out := []api.ProjectMachine{}
	if s.machines == nil {
		return out, nil
	}
	views, _, err := s.machines.List(r.Context())
	if err != nil {
		return nil, err
	}
	for _, v := range views {
		if v.Legacy {
			continue
		}
		out = append(out, api.ProjectMachine{ID: v.ID, Name: v.Name, OS: v.OS, Online: v.Online, CanSpawn: v.SpawnNode != "",
			SpawnNode: string(v.SpawnNode), Allowed: slices.Contains(allowed, v.ID)})
	}
	return out, nil
}

// machineRef finds a computer by id, name or machine node name.
func (s *Server) machineRef(r *http.Request, ref string) (api.ProjectMachine, error) {
	list, err := s.projectMachines(r, nil)
	if err != nil {
		return api.ProjectMachine{}, err
	}
	ref = strings.TrimSpace(ref)
	for _, m := range list {
		if m.ID == ref || strings.EqualFold(m.Name, ref) || (m.SpawnNode != "" && m.SpawnNode == ref) {
			return m, nil
		}
	}
	return api.ProjectMachine{}, domain.Errf(domain.CodeNotFound, "no computer %q in this workspace", ref).
		WithSuggestion("see them with: myco project machines --name <project>")
}

func (s *Server) listProjectMachines(w http.ResponseWriter, r *http.Request, c projectCall) {
	v, err := s.projects.Get(r.Context(), c.by, r.PathValue("name"))
	if err != nil {
		writeError(w, err)
		return
	}
	list, err := s.projectMachines(r, v.Project.Machines)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, api.ProjectMachineList{Project: v.Project.Name, Machines: list})
}

func (s *Server) setProjectMachine(w http.ResponseWriter, r *http.Request, c projectCall, allow bool) {
	m, err := s.machineRef(r, r.PathValue("machine"))
	if err != nil && allow {
		writeError(w, err)
		return
	}
	id := m.ID
	if id == "" {
		id = r.PathValue("machine") // a computer gone since: still taken off
	}
	v, err := s.projects.SetMachine(r.Context(), c.by, r.PathValue("name"), id, allow)
	if err != nil {
		writeError(w, err)
		return
	}
	action := "project.machine_allow"
	if !allow {
		action = "project.machine_disallow"
	}
	s.recordAs(r, c.by, action, v.Project.Name, id)
	writeJSON(w, http.StatusOK, s.projectDTO(r, c, v, false))
}

// requestProjectMachine: a coordinator asks the project's owner to let the
// project start agents on a computer. The owner sees a card (approve or
// deny) in its chat; approving carries out the PUT above as the owner.
func (s *Server) requestProjectMachine(w http.ResponseWriter, r *http.Request, c projectCall) {
	a := c.by.Agent
	if a == nil {
		writeError(w, domain.Errf(domain.CodeInvalidArgument, "only a coordinator agent asks; people set it with PUT /v1/projects/{name}/machines/{machine}"))
		return
	}
	if !s.superagentsEnabled(w) {
		return
	}
	v, ok, err := s.projects.Coordinates(r.Context(), a, r.PathValue("name"))
	if err != nil {
		writeError(w, err)
		return
	}
	if !ok {
		writeError(w, domain.Errf(domain.CodePermissionDenied, "only a coordinator of project %s asks for computers", v.Project.Name))
		return
	}
	m, err := s.machineRef(r, r.PathValue("machine"))
	if err != nil {
		writeError(w, err)
		return
	}
	if slices.Contains(v.Project.Machines, m.ID) {
		writeJSON(w, http.StatusOK, map[string]any{"allowed": true, "machine": m})
		return
	}
	owner, err := s.members.Get(r.Context(), v.Project.Owner)
	if err != nil || v.Project.Owner == "" {
		writeError(w, domain.Errf(domain.CodeInvalidState, "project %s has no owner to ask", v.Project.Name).
			WithSuggestion("ask the hub's admin to allow it"))
		return
	}
	var in struct {
		Reason string `json:"reason"`
	}
	if r.ContentLength != 0 && !decode(w, r, &in) {
		return
	}
	path := fmt.Sprintf("/v1/projects/%s/machines/%s", url.PathEscape(v.Project.Name), url.PathEscape(m.ID))
	// Asked already and still waiting: the same card.
	if list, err := s.superagents.AgentApprovals(r.Context(), a.Name); err == nil {
		for _, x := range list {
			if x.Status == domain.ApprovalPending && x.Method == http.MethodPut && x.Path == path {
				writeJSON(w, http.StatusAccepted, approvalDTO(x))
				return
			}
		}
	}
	title := v.Project.Title
	if title == "" {
		title = v.Project.Name
	}
	what := fmt.Sprintf("让项目「%s」能在「%s」上开 agent", title, m.Name)
	ap, err := s.superagents.Ask(r.Context(), owner, a.Name, what, in.Reason, http.MethodPut, path, "", nil)
	if err != nil {
		writeError(w, err)
		return
	}
	s.recordAs(r, c.by, "project.machine_request", v.Project.Name, m.ID)
	writeJSON(w, http.StatusAccepted, approvalDTO(ap))
}
