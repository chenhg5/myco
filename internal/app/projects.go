package app

import (
	"context"
	"slices"

	"github.com/chenhg5/myco/internal/domain"
)

// ProjectService: projects, their members (nodes) and shared documents, and
// who may see or change them.
//
// People see a project they own, one with a node of theirs in it, or one
// open to the team; the hub's owner and admins see every one. An agent sees
// projects it is in, those of its owner, and team ones. A project's owner
// and the hub's managers edit it; so do its coordinator agents (its
// content, its members' cards, and adding nodes of their own owner). Only
// people change who coordinates, the visibility, or archive it.
type ProjectService struct {
	projects domain.ProjectRepository
	agents   domain.AgentRepository
	ids      IDGenerator
	clock    Clock
}

// NewProjectService wires the project use cases.
func NewProjectService(projects domain.ProjectRepository, agents domain.AgentRepository, ids IDGenerator, clock Clock) *ProjectService {
	return &ProjectService{projects: projects, agents: agents, ids: ids, clock: clock}
}

// ProjectView is a project with its members and documents.
type ProjectView struct {
	Project *domain.Project
	Members []*domain.ProjectMember
	Docs    []*domain.ProjectDoc
}

func (s *ProjectService) load(ctx context.Context, name string) (*ProjectView, error) {
	p, err := s.projects.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	ms, err := s.projects.Members(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	docs, err := s.projects.Docs(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	return &ProjectView{Project: p, Members: ms, Docs: docs}, nil
}

func (v *ProjectView) member(name domain.AgentName) *domain.ProjectMember {
	for _, m := range v.Members {
		if m.Agent == name {
			return m
		}
	}
	return nil
}

// sees reports whether by may see the project.
func (s *ProjectService) sees(ctx context.Context, by Actor, v *ProjectView) bool {
	p := v.Project
	switch {
	case by.Manages(), p.Visibility == domain.ProjectTeam:
		return true
	case by.Agent != nil:
		return v.member(by.Agent.Name) != nil || (p.Owner != "" && by.Agent.Owner == p.Owner)
	case by.Member != nil:
		if p.Owner == by.Member.ID {
			return true
		}
		for _, m := range v.Members {
			if a, err := s.agents.Get(ctx, m.Agent); err == nil && a.Owner == by.Member.ID {
				return true
			}
		}
	}
	return false
}

// coordinates reports whether agent coordinates the project.
func (v *ProjectView) coordinates(a *domain.Agent) bool {
	m := v.member(a.Name)
	return m != nil && m.Role == domain.ProjectRoleCoordinator
}

// edits reports whether by may change the project's content.
func (s *ProjectService) edits(by Actor, v *ProjectView) bool {
	if v.Project.Archived {
		return s.manages(by, v)
	}
	if by.Agent != nil {
		return v.coordinates(by.Agent)
	}
	return s.manages(by, v)
}

// manages reports whether by may change roles, visibility, or archive it:
// people only.
func (s *ProjectService) manages(by Actor, v *ProjectView) bool {
	if by.Agent != nil {
		return false
	}
	return by.Manages() || (by.Member != nil && v.Project.Owner == by.Member.ID)
}

func denied(what string) error {
	return domain.Errf(domain.CodePermissionDenied, "you may not %s", what).
		WithSuggestion("ask the project's owner, a coordinator of it, or the hub's admin")
}

func hidden(name string) error {
	return domain.Errf(domain.CodeNotFound, "project %s not found", name).
		WithSuggestion("list projects with: myco project list")
}

// Create starts a project owned by the person creating it.
func (s *ProjectService) Create(ctx context.Context, by Actor, ownerID, name, title, goal, visibility string) (*ProjectView, error) {
	if by.Agent != nil {
		return nil, denied("create projects (people create them in the app or with their own credentials)")
	}
	n, err := domain.ParseProjectName(name)
	if err != nil {
		return nil, err
	}
	vis, err := domain.ParseProjectVisibility(visibility)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	p := &domain.Project{ID: s.ids.New("prj"), Name: n, Visibility: vis, Owner: ownerID, CreatedAt: now, UpdatedAt: now}
	if title == "" {
		title = n
	}
	if err := p.Apply(domain.ProjectEdit{Title: &title, Goal: &goal}, now); err != nil {
		return nil, err
	}
	if err := s.projects.Create(ctx, p); err != nil {
		return nil, err
	}
	return &ProjectView{Project: p}, nil
}

// List returns the projects by may see; mine keeps those it takes part in
// (an agent: is a member; a person: owns, or has a node in).
func (s *ProjectService) List(ctx context.Context, by Actor, mine bool) ([]*ProjectView, error) {
	all, err := s.projects.List(ctx)
	if err != nil {
		return nil, err
	}
	var out []*ProjectView
	for _, p := range all {
		v, err := s.load(ctx, p.ID)
		if err != nil {
			return nil, err
		}
		if !s.sees(ctx, by, v) {
			continue
		}
		if mine && !s.involved(ctx, by, v) {
			continue
		}
		out = append(out, v)
	}
	return out, nil
}

func (s *ProjectService) involved(ctx context.Context, by Actor, v *ProjectView) bool {
	switch {
	case by.Agent != nil:
		return v.member(by.Agent.Name) != nil
	case by.Member != nil:
		if v.Project.Owner == by.Member.ID {
			return true
		}
		for _, m := range v.Members {
			if a, err := s.agents.Get(ctx, m.Agent); err == nil && a.Owner == by.Member.ID {
				return true
			}
		}
		return false
	}
	return true
}

// Get returns a project by may see.
func (s *ProjectService) Get(ctx context.Context, by Actor, name string) (*ProjectView, error) {
	v, err := s.load(ctx, name)
	if err != nil {
		return nil, err
	}
	if !s.sees(ctx, by, v) {
		return nil, hidden(name)
	}
	return v, nil
}

// Edit changes a project's title, goal, brief or links.
func (s *ProjectService) Edit(ctx context.Context, by Actor, name string, e domain.ProjectEdit) (*ProjectView, error) {
	v, err := s.Get(ctx, by, name)
	if err != nil {
		return nil, err
	}
	if !s.edits(by, v) {
		return nil, denied("edit project " + v.Project.Name)
	}
	if err := v.Project.Apply(e, s.clock.Now()); err != nil {
		return nil, err
	}
	return v, s.projects.Save(ctx, v.Project)
}

// Settle changes what only people decide: visibility and archiving.
func (s *ProjectService) Settle(ctx context.Context, by Actor, name string, visibility *string, archived *bool) (*ProjectView, error) {
	v, err := s.Get(ctx, by, name)
	if err != nil {
		return nil, err
	}
	if !s.manages(by, v) {
		return nil, denied("change the visibility of project " + v.Project.Name + " or archive it")
	}
	if visibility != nil {
		vis, err := domain.ParseProjectVisibility(*visibility)
		if err != nil {
			return nil, err
		}
		v.Project.Visibility = vis
	}
	if archived != nil {
		v.Project.Archived = *archived
	}
	v.Project.UpdatedAt = s.clock.Now()
	return v, s.projects.Save(ctx, v.Project)
}

// Delete removes a project with its members and documents.
func (s *ProjectService) Delete(ctx context.Context, by Actor, name string) (*ProjectView, error) {
	v, err := s.Get(ctx, by, name)
	if err != nil {
		return nil, err
	}
	if !s.manages(by, v) {
		return nil, denied("delete project " + v.Project.Name)
	}
	return v, s.projects.Delete(ctx, v.Project.ID)
}

// MemberEdit adds a node to a project or changes its role or note; nil
// fields stay as they are (a new member defaults to member, no note).
type MemberEdit struct {
	Role *string
	Note *string
}

// PutMember adds agent to the project, or changes its role or note.
//
// Adding a node takes owning it (a person who owns it, the hub's managers),
// or being it (an agent joining a project it can see), or coordinating the
// project with the same owner as the node. Making a coordinator is for
// people who manage the project. A note can be changed by those who edit
// the project and by the node's owner.
func (s *ProjectService) PutMember(ctx context.Context, by Actor, name string, agent string, e MemberEdit, byName string) (*ProjectView, *domain.ProjectMember, error) {
	v, err := s.Get(ctx, by, name)
	if err != nil {
		return nil, nil, err
	}
	a, err := s.agents.Get(ctx, domain.AgentName(agent))
	if err != nil {
		return nil, nil, err
	}
	if v.Project.Archived {
		return nil, nil, domain.Errf(domain.CodeInvalidState, "project %s is archived", v.Project.Name)
	}
	m := v.member(a.Name)
	if m == nil {
		if !s.mayAdd(by, v, a) {
			return nil, nil, denied("add " + string(a.Name) + " to project " + v.Project.Name + " (a node joins through its owner)")
		}
		m = &domain.ProjectMember{Project: v.Project.ID, Agent: a.Name, Role: domain.ProjectRoleMember, AddedBy: byName, AddedAt: s.clock.Now()}
		v.Members = append(v.Members, m)
	}
	if e.Role != nil {
		role, err := domain.ParseProjectRole(*e.Role)
		if err != nil {
			return nil, nil, err
		}
		if role != m.Role && !s.manages(by, v) {
			return nil, nil, denied("change who coordinates project " + v.Project.Name)
		}
		m.Role = role
	}
	if e.Note != nil {
		note, err := domain.CheckNote(*e.Note)
		if err != nil {
			return nil, nil, err
		}
		if note != m.Note && !s.edits(by, v) && !owns(by, a) {
			return nil, nil, denied("change what " + string(a.Name) + " does in project " + v.Project.Name)
		}
		m.Note = note
	}
	return v, m, s.projects.PutMember(ctx, m)
}

func owns(by Actor, a *domain.Agent) bool {
	if by.Agent != nil {
		return by.Agent.Name == a.Name
	}
	return by.Owns(a.Owner)
}

func (s *ProjectService) mayAdd(by Actor, v *ProjectView, a *domain.Agent) bool {
	if by.Agent != nil {
		if by.Agent.Name == a.Name {
			return true // joining a project it can see
		}
		return v.coordinates(by.Agent) && by.Agent.Owner != "" && by.Agent.Owner == a.Owner
	}
	return by.Owns(a.Owner)
}

// RemoveMember takes a node out of a project: its owner, the node itself,
// or someone who edits the project.
func (s *ProjectService) RemoveMember(ctx context.Context, by Actor, name, agent string) (*ProjectView, error) {
	v, err := s.Get(ctx, by, name)
	if err != nil {
		return nil, err
	}
	m := v.member(domain.AgentName(agent))
	if m == nil {
		return nil, domain.Errf(domain.CodeNotFound, "%s is not in project %s", agent, v.Project.Name)
	}
	a, _ := s.agents.Get(ctx, domain.AgentName(agent))
	allowed := s.edits(by, v) || (a != nil && owns(by, a))
	// A coordinator does not remove another coordinator: people decide that.
	if by.Agent != nil && m.Role == domain.ProjectRoleCoordinator && by.Agent.Name != m.Agent {
		allowed = false
	}
	if !allowed {
		return nil, denied("remove " + agent + " from project " + v.Project.Name)
	}
	if err := s.projects.DeleteMember(ctx, v.Project.ID, m.Agent); err != nil {
		return nil, err
	}
	v.Members = slices.DeleteFunc(v.Members, func(x *domain.ProjectMember) bool { return x.Agent == m.Agent })
	return v, nil
}

// PutDoc writes a shared document.
func (s *ProjectService) PutDoc(ctx context.Context, by Actor, name, doc, body, byName string) (*domain.ProjectDoc, error) {
	v, err := s.Get(ctx, by, name)
	if err != nil {
		return nil, err
	}
	if !s.edits(by, v) {
		return nil, denied("change the documents of project " + v.Project.Name)
	}
	n, err := domain.ParseDocName(doc)
	if err != nil {
		return nil, err
	}
	if err := domain.CheckDocBody(body); err != nil {
		return nil, err
	}
	exists := false
	for _, d := range v.Docs {
		exists = exists || d.Name == n
	}
	if !exists && len(v.Docs) >= domain.MaxProjectDocs {
		return nil, domain.Errf(domain.CodeInvalidArgument, "project %s already has %d documents", v.Project.Name, domain.MaxProjectDocs).
			WithSuggestion("merge or remove some, or keep longer material in the repository")
	}
	d := &domain.ProjectDoc{Project: v.Project.ID, Name: n, Body: body, UpdatedBy: byName, UpdatedAt: s.clock.Now()}
	return d, s.projects.PutDoc(ctx, d)
}

// DeleteDoc removes a shared document.
func (s *ProjectService) DeleteDoc(ctx context.Context, by Actor, name, doc string) error {
	v, err := s.Get(ctx, by, name)
	if err != nil {
		return err
	}
	if !s.edits(by, v) {
		return denied("change the documents of project " + v.Project.Name)
	}
	n, err := domain.ParseDocName(doc)
	if err != nil {
		return err
	}
	return s.projects.DeleteDoc(ctx, v.Project.ID, n)
}

// MayEditCard reports whether editor (an agent) may change target's card:
// it coordinates an active project target is in.
func (s *ProjectService) MayEditCard(ctx context.Context, editor, target *domain.Agent) (bool, error) {
	if editor.Name == target.Name {
		return false, nil
	}
	mine, err := s.projects.MembershipsOf(ctx, editor.Name)
	if err != nil {
		return false, err
	}
	for _, m := range mine {
		if m.Role != domain.ProjectRoleCoordinator {
			continue
		}
		v, err := s.load(ctx, m.Project)
		if err != nil || v.Project.Archived {
			continue
		}
		if v.member(target.Name) != nil {
			return true, nil
		}
	}
	return false, nil
}

// Of lists the projects an agent is in (for its card).
func (s *ProjectService) Of(ctx context.Context, agent domain.AgentName) ([]*domain.ProjectMember, map[string]*domain.Project, error) {
	ms, err := s.projects.MembershipsOf(ctx, agent)
	if err != nil {
		return nil, nil, err
	}
	byID := map[string]*domain.Project{}
	for _, m := range ms {
		if p, err := s.projects.Get(ctx, m.Project); err == nil {
			byID[m.Project] = p
		}
	}
	return ms, byID, nil
}

// ForgetAgent takes a removed agent out of every project.
func (s *ProjectService) ForgetAgent(ctx context.Context, agent domain.AgentName) error {
	return s.projects.DeleteAgent(ctx, agent)
}

// Rights says what by may do with a project it sees: edit its content,
// manage it (roles, visibility, archive, delete).
func (s *ProjectService) Rights(by Actor, v *ProjectView) (edit, manage bool) {
	return s.edits(by, v), s.manages(by, v)
}
