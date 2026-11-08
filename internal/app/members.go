package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/chenhg5/myco/internal/domain"
)

// DefaultOwnerName is the handle of the member created for an existing hub
// (they can rename themselves).
const DefaultOwnerName = "owner"

// Actor is who is making a request: a member (through a console token), the
// hub's admin token, or an agent.
type Actor struct {
	Member *domain.Member // nil for the admin token and agents
	Admin  bool           // the hub's own admin token: acts as the owner
	Agent  *domain.Agent
}

// Manages reports whether the actor may manage the hub (owner or admin).
func (a Actor) Manages() bool {
	return a.Admin || (a.Member != nil && a.Member.Role.Manages())
}

// IsOwner reports whether the actor is the hub's owner.
func (a Actor) IsOwner() bool {
	return a.Admin || (a.Member != nil && a.Member.Role == domain.RoleOwner)
}

// Owns reports whether the actor may manage something owned by owner.
func (a Actor) Owns(owner string) bool {
	return a.Manages() || (a.Member != nil && owner != "" && owner == a.Member.ID)
}

// MemberID is the member acting ("" for the admin token and agents).
func (a Actor) MemberID() string {
	if a.Member != nil {
		return a.Member.ID
	}
	return ""
}

// Name is how the actor appears in the audit log.
func (a Actor) Name() string {
	switch {
	case a.Member != nil:
		return a.Member.Name
	case a.Agent != nil:
		return string(a.Agent.Name)
	}
	return "admin"
}

// Kind is member, admin or agent.
func (a Actor) Kind() string {
	switch {
	case a.Member != nil:
		return "member"
	case a.Agent != nil:
		return "agent"
	}
	return "admin"
}

// MemberService manages the people using a hub and their credentials.
type MemberService struct {
	repo    domain.MemberRepository
	console *ConsoleService
	audit   *AuditService
	ids     IDGenerator
	clock   Clock
}

// NewMemberService wires the member use cases.
func NewMemberService(repo domain.MemberRepository, console *ConsoleService, audit *AuditService, ids IDGenerator, clock Clock) *MemberService {
	return &MemberService{repo: repo, console: console, audit: audit, ids: ids, clock: clock}
}

// Bootstrap makes sure the hub has an owner and that everything without an
// owner (from before members existed) belongs to them. Idempotent.
func (s *MemberService) Bootstrap(ctx context.Context, log *slog.Logger) (*domain.Member, error) {
	owner, err := s.Owner(ctx)
	if domain.IsCode(err, domain.CodeNotFound) {
		owner = &domain.Member{ID: s.ids.New("mem"), Name: DefaultOwnerName, Role: domain.RoleOwner, CreatedAt: s.clock.Now()}
		if err := s.repo.Create(ctx, owner); err != nil {
			return nil, err
		}
		if log != nil {
			log.Info("created the hub's owner member; rename it in the app or with: myco member update --name owner --new-name <you>")
		}
	} else if err != nil {
		return nil, err
	}
	return owner, s.repo.AssignUnowned(ctx, owner.ID)
}

// Owner returns the hub's owner.
func (s *MemberService) Owner(ctx context.Context) (*domain.Member, error) {
	list, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, m := range list {
		if m.Role == domain.RoleOwner && m.Active() {
			return m, nil
		}
	}
	return nil, domain.Errf(domain.CodeNotFound, "this hub has no owner yet")
}

// List returns every member (removed ones too, so names stay resolvable).
func (s *MemberService) List(ctx context.Context) ([]*domain.Member, error) { return s.repo.List(ctx) }

// Get returns a member by id.
func (s *MemberService) Get(ctx context.Context, id string) (*domain.Member, error) {
	return s.repo.Get(ctx, id)
}

// Find returns a member by id or name.
func (s *MemberService) Find(ctx context.Context, ref string) (*domain.Member, error) {
	if strings.HasPrefix(ref, "mem_") {
		if m, err := s.repo.Get(ctx, ref); err == nil {
			return m, nil
		}
	}
	return s.repo.GetByName(ctx, strings.ToLower(strings.TrimSpace(ref)))
}

// InviteInput describes a new member.
type InviteInput struct {
	Name        string
	DisplayName string
	Role        string
}

// Invite adds a member and issues their first console token (returned
// once, for the invite link).
func (s *MemberService) Invite(ctx context.Context, by Actor, in InviteInput) (*domain.Member, string, error) {
	if !by.Manages() {
		return nil, "", domain.Errf(domain.CodePermissionDenied, "only the hub's owner or an admin can invite people")
	}
	name, err := domain.ParseMemberName(in.Name)
	if err != nil {
		return nil, "", err
	}
	role := domain.RoleMember
	if in.Role != "" {
		if role, err = domain.ParseRole(in.Role); err != nil {
			return nil, "", err
		}
	}
	if role == domain.RoleOwner {
		return nil, "", domain.Errf(domain.CodeInvalidArgument, "a hub has one owner; invite as admin or member")
	}
	if role == domain.RoleAdmin && !by.IsOwner() && !by.Manages() {
		return nil, "", domain.Errf(domain.CodePermissionDenied, "only the owner or an admin can invite admins")
	}
	m := &domain.Member{ID: s.ids.New("mem"), Name: name, DisplayName: strings.TrimSpace(in.DisplayName), Role: role, CreatedAt: s.clock.Now()}
	if err := s.repo.Create(ctx, m); err != nil {
		return nil, "", err
	}
	// The link carries a one-time code (24 hours): the invitee's app trades
	// it for its own token, so the link is worthless once used.
	plain, _, err := s.console.CreatePairing(ctx, m.ID)
	if err != nil {
		return nil, "", err
	}
	s.audit.Record(ctx, by, "member.invite", name, "role "+string(role))
	return m, plain, nil
}

// UpdateInput changes a member; empty fields are left alone.
type UpdateInput struct {
	Name        string
	DisplayName *string
	Role        string
}

// Update renames a member or changes their role. Members may rename
// themselves; roles are changed by the owner (any) or admins (not the
// owner's, not to owner). The owner is transferred by making someone else
// owner, which turns the old owner into an admin.
func (s *MemberService) Update(ctx context.Context, by Actor, ref string, in UpdateInput) (*domain.Member, error) {
	m, err := s.Find(ctx, ref)
	if err != nil {
		return nil, err
	}
	if !m.Active() {
		return nil, domain.Errf(domain.CodeInvalidState, "member %s was removed", m.Name)
	}
	self := by.Member != nil && by.Member.ID == m.ID
	if !self && !by.Manages() {
		return nil, domain.Errf(domain.CodePermissionDenied, "you can only change your own name")
	}
	var notes []string
	if in.Name != "" {
		name, err := domain.ParseMemberName(in.Name)
		if err != nil {
			return nil, err
		}
		if name != m.Name {
			notes = append(notes, "renamed from "+m.Name)
			m.Name = name
		}
	}
	if in.DisplayName != nil {
		m.DisplayName = strings.TrimSpace(*in.DisplayName)
	}
	var demote *domain.Member
	if in.Role != "" {
		role, err := domain.ParseRole(in.Role)
		if err != nil {
			return nil, err
		}
		if role != m.Role {
			switch {
			case !by.Manages():
				return nil, domain.Errf(domain.CodePermissionDenied, "only the owner or an admin can change roles")
			case m.Role == domain.RoleOwner:
				return nil, domain.Errf(domain.CodeInvalidArgument, "the owner's role changes only by making someone else owner")
			case role == domain.RoleOwner && !by.IsOwner():
				return nil, domain.Errf(domain.CodePermissionDenied, "only the owner can hand over ownership")
			}
			if role == domain.RoleOwner {
				if demote, err = s.Owner(ctx); err != nil {
					return nil, err
				}
				demote.Role = domain.RoleAdmin
			}
			notes = append(notes, "role "+string(m.Role)+" → "+string(role))
			m.Role = role
		}
	}
	if err := s.repo.Save(ctx, m); err != nil {
		return nil, err
	}
	if demote != nil {
		if err := s.repo.Save(ctx, demote); err != nil {
			return nil, err
		}
	}
	if len(notes) > 0 || in.DisplayName != nil {
		s.audit.Record(ctx, by, "member.update", m.Name, strings.Join(notes, "; "))
	}
	return m, nil
}

// Remove takes a member off the hub and revokes all their console tokens.
// What they own stays (an admin can give it to someone else).
func (s *MemberService) Remove(ctx context.Context, by Actor, ref string) (*domain.Member, error) {
	m, err := s.Find(ctx, ref)
	if err != nil {
		return nil, err
	}
	switch {
	case !by.Manages():
		return nil, domain.Errf(domain.CodePermissionDenied, "only the owner or an admin can remove people")
	case m.Role == domain.RoleOwner:
		return nil, domain.Errf(domain.CodeInvalidArgument, "the owner cannot be removed; hand over ownership first")
	case m.Role == domain.RoleAdmin && !by.IsOwner():
		return nil, domain.Errf(domain.CodePermissionDenied, "only the owner can remove an admin")
	}
	if !m.Active() {
		return m, nil
	}
	m.RemovedAt = s.clock.Now()
	if err := s.repo.Save(ctx, m); err != nil {
		return nil, err
	}
	n, err := s.console.RevokeMember(ctx, m.ID)
	if err != nil {
		return nil, err
	}
	s.audit.Record(ctx, by, "member.remove", m.Name, plural(n, "app credential")+" revoked")
	return m, nil
}

func plural(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	return fmt.Sprintf("%d %ss", n, what)
}

// AuditService appends to and reads the audit log.
type AuditService struct {
	repo  domain.AuditRepository
	ids   IDGenerator
	clock Clock
	log   *slog.Logger
}

// NewAuditService wires the audit log.
func NewAuditService(repo domain.AuditRepository, ids IDGenerator, clock Clock, log *slog.Logger) *AuditService {
	return &AuditService{repo: repo, ids: ids, clock: clock, log: log}
}

// Record appends one entry. Failing to write the log is logged, not
// returned: the action itself already happened.
func (s *AuditService) Record(ctx context.Context, by Actor, action, target, detail string) {
	if s == nil {
		return
	}
	e := &domain.AuditEntry{ID: s.ids.New("aud"), At: s.clock.Now(), Actor: by.MemberID(), ActorName: by.Name(), ActorKind: by.Kind(),
		Action: action, Target: target, Detail: detail}
	if err := s.repo.Append(context.WithoutCancel(ctx), e); err != nil && s.log != nil {
		s.log.Error("writing the audit log failed", "action", action, "target", target, "err", err)
	}
}

// List reads the audit log, newest first.
func (s *AuditService) List(ctx context.Context, f domain.AuditFilter) ([]*domain.AuditEntry, error) {
	return s.repo.List(ctx, f)
}
