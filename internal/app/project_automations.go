package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/myco-ai/myco/internal/domain"
)

// ProjectAutomationService restricts the ordinary automation service to an
// active project's coordinators and its owner's ordinary agent members.
type ProjectAutomationService struct {
	invites *ProjectInvitationService
	autos   *AutomationService
}
type projectAutomationIntent struct {
	TaskName  string          `json:"task_name,omitempty"`
	Project   string          `json:"project"`
	ProjectID string          `json:"project_id"`
	Owner     string          `json:"owner"`
	Action    string          `json:"action"`
	ID        string          `json:"id,omitempty"`
	Revision  string          `json:"revision,omitempty"`
	Input     AutomationInput `json:"input"`
}
type ProjectAutomationOut struct {
	Automation *AutomationOut
	ApprovalID string
	Status     string
	Removed    string
}

func NewProjectAutomationService(invites *ProjectInvitationService, autos *AutomationService) *ProjectAutomationService {
	s := &ProjectAutomationService{invites, autos}
	invites.approvals.SetTyped(domain.ApprovalKindProjectAutomation, s)
	autos.projectGuard = s.guard
	return s
}
func (s *ProjectService) SetCoordinatorAutomations(ctx context.Context, by Actor, name string, allow bool) (*ProjectView, error) {
	v, e := s.Get(ctx, by, name)
	if e != nil {
		return nil, e
	}
	if by.Via != nil || !s.manages(by, v) {
		return nil, denied("choose whether coordinators may schedule tasks")
	}
	v.Project.CoordinatorAutomations = allow
	v.Project.UpdatedAt = s.clock.Now()
	return v, s.projects.Save(ctx, v.Project)
}
func automationRevision(a *domain.Automation) string {
	c := *a
	c.LastTask = ""
	c.NextRunAt = time.Time{}
	c.UpdatedAt = time.Time{}
	b, _ := json.Marshal(c)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func (s *ProjectAutomationService) target(ctx context.Context, p *ProjectView, a *domain.Automation) error {
	if a.Project != p.Project.Name || a.OwnerID != p.Project.Owner {
		return domain.Errf(domain.CodePermissionDenied, "automation must belong to this project's owner and project")
	}
	if a.StartsWorkflow() {
		if s.autos.workflows == nil {
			return domain.Errf(domain.CodeUnsupported, "workflows unavailable")
		}
		w, e := s.autos.workflows.repo.Get(ctx, a.Workflow)
		if e != nil {
			return e
		}
		if w.Project != p.Project.Name || w.OwnerID != p.Project.Owner || !w.Enabled {
			return domain.Errf(domain.CodePermissionDenied, "workflow must be enabled and belong to this project and its owner")
		}
		return nil
	}
	target, _, e := (aliasResolver{agents: s.autos.messages.agents, aliases: s.autos.messages.aliases}).resolve(ctx, a.To)
	if e != nil {
		return e
	}
	if target.Kind() != domain.KindAgent {
		return domain.Errf(domain.CodePermissionDenied, "scheduled target must be an ordinary agent member of this project")
	}
	for _, m := range p.Members {
		if m.Agent == target.Name {
			a.To = target.Name
			return nil
		}
	}
	return domain.Errf(domain.CodePermissionDenied, "target is not a member agent of this project").WithSuggestion("ask the project owner to add the agent first")
}
func (s *ProjectAutomationService) guard(ctx context.Context, a *domain.Automation) error {
	p, e := s.invites.projects.load(ctx, a.Project)
	if e != nil {
		return e
	}
	if p.Project.Archived {
		return domain.Errf(domain.CodePermissionDenied, "project archived")
	}
	owner, e := s.invites.members.Get(ctx, p.Project.Owner)
	if e != nil {
		return e
	}
	if !owner.Active() {
		return domain.Errf(domain.CodePermissionDenied, "project owner is inactive")
	}
	return s.target(ctx, p, a)
}
func (s *ProjectAutomationService) List(ctx context.Context, by *domain.Agent, name string) ([]*AutomationOut, error) {
	p, e := s.invites.coordinator(ctx, by, name)
	if e != nil {
		return nil, e
	}
	list, e := s.autos.List(ctx, Caller{MemberID: p.Project.Owner})
	if e != nil {
		return nil, e
	}
	out := []*AutomationOut{}
	for _, v := range list {
		if v.Automation.Project == p.Project.Name && v.Automation.Cron != "" && v.Automation.WebhookHash == "" {
			if v.LastRun != nil {
				v.LastRun.Request = nil
			}
			out = append(out, v)
		}
	}
	return out, nil
}

// prepare validates the full resulting configuration before asking for approval.
// It issues no secrets and writes nothing. Caller holds autos.mu.
func (s *ProjectAutomationService) prepare(ctx context.Context, by *domain.Agent, in *projectAutomationIntent) (*ProjectView, Caller, *domain.Automation, error) {
	p, e := s.invites.coordinator(ctx, by, in.Project)
	if e != nil {
		return nil, Caller{}, nil, e
	}
	owner, e := s.invites.members.Get(ctx, p.Project.Owner)
	if e != nil {
		return nil, Caller{}, nil, e
	}
	if !owner.Active() {
		return nil, Caller{}, nil, domain.Errf(domain.CodePermissionDenied, "project owner is inactive")
	}
	c := Caller{Name: owner.Name, MemberID: owner.ID}
	if in.Owner != "" && (in.Owner != owner.ID || in.ProjectID != p.Project.ID) {
		return nil, c, nil, domain.Errf(domain.CodeInvalidState, "project owner changed; ask again")
	}
	var a *domain.Automation
	if in.Action == "create" {
		a = &domain.Automation{Owner: owner.Name, OwnerID: owner.ID, From: ConsoleSender(owner.Name), Enabled: true}
	} else {
		a, e = s.autos.get(ctx, c, in.ID)
		if e != nil {
			return nil, c, nil, e
		}
		if a.Project != p.Project.Name {
			return nil, c, nil, domain.Errf(domain.CodePermissionDenied, "automation is outside this project")
		}
		if a.Cron == "" || a.WebhookHash != "" {
			return nil, c, nil, domain.Errf(domain.CodePermissionDenied, "coordinators manage scheduled tasks only; ask the project owner to manage webhooks in the App")
		}
		if in.Revision != "" && in.Revision != automationRevision(a) {
			return nil, c, nil, domain.Errf(domain.CodeInvalidState, "automation changed since requested; submit a fresh request")
		}
	}
	if in.Action == "delete" {
		return p, c, a, nil
	}
	if in.Action != "create" && in.Action != "update" {
		return nil, c, nil, domain.Errf(domain.CodeInvalidArgument, "unknown automation action")
	}
	if in.Input.Project != nil && *in.Input.Project != p.Project.Name {
		return nil, c, nil, domain.Errf(domain.CodePermissionDenied, "project is fixed by this route")
	}
	if in.Input.RotateSecret || (in.Input.Webhook != nil && *in.Input.Webhook) {
		return nil, c, nil, domain.Errf(domain.CodePermissionDenied, "coordinators manage scheduled tasks; webhook secrets require the owner's App")
	}
	project := p.Project.Name
	in.Input.Project = &project
	if _, e = s.autos.apply(a, in.Input); e != nil {
		return nil, c, nil, e
	}
	if e = a.Validate(); e != nil {
		return nil, c, nil, e
	}
	if a.Cron == "" || a.WebhookHash != "" {
		return nil, c, nil, domain.Errf(domain.CodePermissionDenied, "project automation must have a schedule and no webhook")
	}
	if e = s.autos.checkTarget(ctx, c, a); e != nil {
		return nil, c, nil, e
	}
	if e = s.target(ctx, p, a); e != nil {
		return nil, c, nil, e
	}
	// Resolve aliases and workflow names now, so approval cannot silently retarget.
	to, wf := string(a.To), a.Workflow
	in.Input.To = &to
	in.Input.Workflow = &wf
	return p, c, a, nil
}
func (s *ProjectAutomationService) Change(ctx context.Context, by *domain.Agent, name, action, id string, input AutomationInput, dry bool) (*ProjectAutomationOut, error) {
	s.autos.mu.Lock()
	defer s.autos.mu.Unlock()
	in := projectAutomationIntent{Project: name, Action: action, ID: id, Input: input}
	p, c, candidate, e := s.prepare(ctx, by, &in)
	if e != nil {
		return nil, e
	}
	in.TaskName = candidate.Name
	if action != "create" {
		base, e := s.autos.repo.Get(ctx, id)
		if e != nil {
			return nil, e
		}
		in.Revision = automationRevision(base)
	}
	if dry {
		return &ProjectAutomationOut{Status: "dry_run"}, nil
	}
	if p.Project.CoordinatorAutomations {
		return s.execute(ctx, by, c, in, "")
	}
	in.Owner, in.ProjectID = p.Project.Owner, p.Project.ID
	payload, _ := json.Marshal(in)
	owner, e := s.invites.members.Get(ctx, p.Project.Owner)
	if e != nil {
		return nil, e
	}
	a, e := s.invites.approvals.AskTyped(ctx, owner, by.Name, domain.ApprovalKindProjectAutomation, fmt.Sprintf("%s scheduled task %q in project %s", action, in.TaskName, name), "", string(payload))
	if e != nil {
		return nil, e
	}
	s.invites.audit.Record(ctx, Actor{Agent: by}, "project.automation.request", name, a.ID+" "+action+" "+id)
	return &ProjectAutomationOut{Status: "pending", ApprovalID: a.ID}, nil
}

// execute is called under the same lock as validation and dispatch.
func (s *ProjectAutomationService) execute(ctx context.Context, by *domain.Agent, c Caller, in projectAutomationIntent, approvedBy string) (*ProjectAutomationOut, error) {
	c.Agent, c.ApprovedBy, c.ProjectScope = by.Name, approvedBy, in.Project
	out := &ProjectAutomationOut{Status: "applied"}
	var e error
	switch in.Action {
	case "create":
		out.Automation, e = s.autos.create(ctx, c, in.Input)
	case "update":
		out.Automation, e = s.autos.update(ctx, c, in.ID, in.Input)
	case "delete":
		e = s.autos.delete(ctx, c, in.ID)
		out.Removed = in.ID
	}
	if e != nil {
		return nil, e
	}
	if a := out.Automation; a != nil {
		in.ID = a.Automation.ID
	}
	s.invites.audit.Record(ctx, Actor{Agent: by}, "project.automation."+in.Action, in.Project, fmt.Sprintf("automation=%s approved_by=%s", in.ID, approvedBy))
	return out, nil
}
func (s *ProjectAutomationService) Approve(ctx context.Context, a *domain.Approval, by *domain.Member, narrow json.RawMessage) (string, error) {
	var in projectAutomationIntent
	if json.Unmarshal([]byte(a.Payload), &in) != nil {
		return "", domain.Errf(domain.CodeInvalidArgument, "invalid automation request")
	}
	if by.ID != a.Owner || by.ID != in.Owner {
		return "", domain.Errf(domain.CodePermissionDenied, "project owner approval required")
	}
	agent, e := s.invites.projects.agents.Get(ctx, a.Agent)
	if e != nil {
		return "", e
	}
	s.autos.mu.Lock()
	defer s.autos.mu.Unlock()
	_, c, _, e := s.prepare(ctx, agent, &in)
	if e != nil {
		return "", e
	}
	if e = s.invites.approvals.approvals.Claim(ctx, a.ID); e != nil {
		return "", e
	}
	out, e := s.execute(ctx, agent, c, in, by.Name)
	if e != nil {
		return "", e
	}
	id := out.Removed
	if out.Automation != nil {
		id = out.Automation.Automation.ID
	}
	s.invites.audit.Record(ctx, Actor{Member: by}, "project.automation.approve", in.Project, a.ID+" "+id)
	return fmt.Sprintf("Project automation %s: %s", id, in.Action), nil
}
func (s *ProjectAutomationService) Deny(ctx context.Context, a *domain.Approval) {
	var in projectAutomationIntent
	_ = json.Unmarshal([]byte(a.Payload), &in)
	if owner, e := s.invites.members.Get(ctx, a.Owner); e == nil {
		s.invites.audit.Record(ctx, Actor{Member: owner}, "project.automation.deny", in.Project, a.ID+" requested_by="+string(a.Agent))
	}
}
