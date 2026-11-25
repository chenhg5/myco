package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/domain"
)

type ProjectInviteInput struct {
	DisplayName string `json:"display_name,omitempty"`
	Description string `json:"description,omitempty"`
	Note        string `json:"note,omitempty"`
}
type ProjectInvitationService struct {
	repo      domain.ProjectInvitationRepository
	projects  *ProjectService
	members   domain.MemberRepository
	approvals *SuperagentService
	enroll    *EnrollmentService
	audit     *AuditService
	ids       IDGenerator
	clock     Clock
}

func NewProjectInvitationService(repo domain.ProjectInvitationRepository, projects *ProjectService, members domain.MemberRepository, approvals *SuperagentService, enroll *EnrollmentService, audit *AuditService, ids IDGenerator, clock Clock) *ProjectInvitationService {
	s := &ProjectInvitationService{repo, projects, members, approvals, enroll, audit, ids, clock}
	approvals.SetTyped(domain.ApprovalKindProjectInvite, s)
	enroll.projectInvites = s
	return s
}
func (s *ProjectService) SetCoordinatorInvites(ctx context.Context, by Actor, name string, allow bool) (*ProjectView, error) {
	v, err := s.Get(ctx, by, name)
	if err != nil {
		return nil, err
	}
	if by.Via != nil || !s.manages(by, v) {
		return nil, denied("choose whether coordinators may invite agents")
	}
	v.Project.CoordinatorInvites = allow
	v.Project.UpdatedAt = s.clock.Now()
	return v, s.projects.Save(ctx, v.Project)
}
func (s *ProjectInvitationService) coordinator(ctx context.Context, a *domain.Agent, name string) (*ProjectView, error) {
	if a == nil {
		return nil, domain.Errf(domain.CodePermissionDenied, "only a project coordinator agent may use coordinator permissions")
	}
	v, ok, err := s.projects.Coordinates(ctx, a, name)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, domain.Errf(domain.CodePermissionDenied, "only a coordinator of an active project may use coordinator permissions").WithSuggestion("ask the project owner to assign the coordinator role, then enable the permission or approve the request")
	}
	if v.Project.Owner == "" {
		return nil, domain.Errf(domain.CodeInvalidState, "project has no owner").WithSuggestion("ask an administrator to assign a project owner")
	}
	if _, err = s.members.Get(ctx, v.Project.Owner); err != nil {
		return nil, err
	}
	return v, nil
}
func (s *ProjectInvitationService) Create(ctx context.Context, a *domain.Agent, name string, in ProjectInviteInput, dry bool) (*domain.ProjectInvitation, string, error) {
	p, err := s.coordinator(ctx, a, name)
	if err != nil {
		return nil, "", err
	}
	in.DisplayName, in.Description, in.Note = strings.TrimSpace(in.DisplayName), strings.TrimSpace(in.Description), strings.TrimSpace(in.Note)
	if _, err := domain.ParseDisplayName(in.DisplayName); err != nil {
		return nil, "", err
	}
	if len(in.Description) > 2000 {
		return nil, "", domain.Errf(domain.CodeInvalidArgument, "description longer than 2000 bytes")
	}
	if _, err := domain.CheckNote(in.Note); err != nil {
		return nil, "", err
	}
	now := s.clock.Now()
	v := &domain.ProjectInvitation{ID: s.ids.New("pinv"), Project: p.Project.ID, Requester: a.Name, Owner: p.Project.Owner, DisplayName: in.DisplayName, Description: in.Description, Note: in.Note, Status: "approved", CreatedAt: now, ExpiresAt: now.Add(24 * time.Hour)}
	if !p.Project.CoordinatorInvites {
		v.Status = "pending"
	}
	if dry {
		return v, "", nil
	}
	if err = s.repo.Create(ctx, v); err != nil {
		return nil, "", err
	}
	s.audit.Record(ctx, Actor{Agent: a}, "project.invitation.create", p.Project.Name, v.ID)
	if v.Status == "approved" {
		return s.Claim(ctx, a, name, v.ID)
	}
	owner, err := s.members.Get(ctx, v.Owner)
	if err != nil {
		return nil, "", err
	}
	payload, _ := json.Marshal(map[string]string{"invitation_id": v.ID, "project": p.Project.Name, "project_title": p.Project.Title, "display_name": v.DisplayName, "description": v.Description, "note": v.Note})
	ap, err := s.approvals.AskTyped(ctx, owner, a.Name, domain.ApprovalKindProjectInvite, fmt.Sprintf("Invite an agent to project %s", p.Project.Name), in.Note, string(payload))
	if err != nil {
		return nil, "", err
	}
	v.ApprovalID = ap.ID
	err = s.repo.SetApproval(ctx, v.ID, ap.ID)
	return v, "", err
}
func (s *ProjectInvitationService) Status(ctx context.Context, a *domain.Agent, name, id string) (*domain.ProjectInvitation, error) {
	p, err := s.coordinator(ctx, a, name)
	if err != nil {
		return nil, err
	}
	v, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if v.Project != p.Project.ID || v.Requester != a.Name || v.Owner != p.Project.Owner {
		return nil, domain.Errf(domain.CodeNotFound, "invitation not found")
	}
	if s.clock.Now().Compare(v.ExpiresAt) >= 0 && v.Status != "used" && v.Status != "denied" {
		v.Status = "expired"
	}
	return v, nil
}

// Claim reveals the link once. Approval cards, events and messages never carry it.
func (s *ProjectInvitationService) Claim(ctx context.Context, a *domain.Agent, name, id string) (*domain.ProjectInvitation, string, error) {
	v, err := s.Status(ctx, a, name, id)
	if err != nil {
		return nil, "", err
	}
	if v.Status != "approved" {
		return v, "", domain.Errf(domain.CodeInvalidState, "invitation is %s", v.Status).WithSuggestion("wait for the project owner to approve; a claimed link is shown only once, request a fresh invitation if it was lost")
	}
	plain, err := s.enroll.issueEnrollToken()
	if err != nil {
		return nil, "", err
	}
	now := s.clock.Now()
	e, err := domain.NewEnrollment(s.ids.New("enr"), s.enroll.tokens.Hash(plain), domain.EnrollmentSpec{NamePrefix: "invited-", Owner: v.Owner, MaxUses: 1, TTL: 24 * time.Hour}, now)
	if err != nil {
		return nil, "", err
	}
	e.InvitationID = v.ID
	if err = s.repo.Issue(ctx, v.ID, e, now); err != nil {
		return nil, "", err
	}
	v.Status, v.EnrollmentID, v.ExpiresAt = "ready", e.ID, e.ExpiresAt
	s.audit.Record(ctx, Actor{Agent: a}, "project.invitation.issue", name, v.ID)
	return v, plain, nil
}
func (s *ProjectInvitationService) Approve(ctx context.Context, a *domain.Approval, by *domain.Member, narrow json.RawMessage) (string, error) {
	var in struct {
		ID string `json:"invitation_id"`
	}
	if json.Unmarshal([]byte(a.Payload), &in) != nil || in.ID == "" {
		return "", domain.Errf(domain.CodeInvalidArgument, "invalid invitation approval")
	}
	v, err := s.repo.Get(ctx, in.ID)
	if err != nil {
		return "", err
	}
	if a.Owner != v.Owner || by.ID != v.Owner || a.Agent != v.Requester || a.ID != v.ApprovalID {
		return "", domain.Errf(domain.CodePermissionDenied, "invitation approval does not match its owner")
	}
	agent, err := s.projects.agents.Get(ctx, v.Requester)
	if err != nil {
		return "", err
	}
	p, err := s.coordinator(ctx, agent, v.Project)
	if err != nil {
		return "", err
	}
	if p.Project.Owner != v.Owner {
		return "", domain.Errf(domain.CodeInvalidState, "project owner changed")
	}
	if err = s.repo.Decide(ctx, v.ID, "approved", s.clock.Now()); err != nil {
		return "", err
	}
	s.audit.Record(ctx, Actor{Member: by}, "project.invitation.approve", p.Project.Name, v.ID)
	return fmt.Sprintf("Retrieve the one-use link with: myco project invite-agent --name %s --request %s", p.Project.Name, v.ID), nil
}
func (s *ProjectInvitationService) Deny(ctx context.Context, a *domain.Approval) {
	var in struct {
		ID string `json:"invitation_id"`
	}
	if json.Unmarshal([]byte(a.Payload), &in) != nil {
		return
	}
	if v, err := s.repo.Get(ctx, in.ID); err == nil && v.ApprovalID == a.ID {
		_ = s.repo.Decide(ctx, v.ID, "denied", s.clock.Now())
		if by, err := s.members.Get(ctx, v.Owner); err == nil {
			s.audit.Record(ctx, Actor{Member: by}, "project.invitation.deny", v.Project, v.ID)
		}
	}
}
func (s *ProjectInvitationService) forEnrollment(ctx context.Context, e *domain.Enrollment) (*domain.ProjectInvitation, error) {
	v, err := s.repo.Get(ctx, e.InvitationID)
	if err != nil {
		return nil, err
	}
	if v.EnrollmentID != e.ID || v.Owner != e.Owner || v.Status != "ready" || !s.clock.Now().Before(v.ExpiresAt) {
		return nil, domain.Errf(domain.CodePermissionDenied, "project invitation expired or already used")
	}
	a, err := s.projects.agents.Get(ctx, v.Requester)
	if err != nil {
		return nil, err
	}
	p, err := s.coordinator(ctx, a, v.Project)
	if err != nil {
		return nil, err
	}
	if p.Project.Owner != v.Owner {
		return nil, domain.Errf(domain.CodePermissionDenied, "project owner changed")
	}
	return v, nil
}
