package domain

import (
	"context"
	"strings"
	"time"
)

// Role is what a member may do on the hub.
type Role string

const (
	// RoleOwner is the hub's creator: everything, including managing admins.
	RoleOwner Role = "owner"
	// RoleAdmin manages members, every node and machine, links and aliases.
	RoleAdmin Role = "admin"
	// RoleMember sees everything and talks to every node, but manages only
	// the nodes and machines they own.
	RoleMember Role = "member"
)

// ParseRole validates a role.
func ParseRole(s string) (Role, error) {
	switch r := Role(strings.TrimSpace(s)); r {
	case RoleOwner, RoleAdmin, RoleMember:
		return r, nil
	}
	return "", Errf(CodeInvalidArgument, "invalid role %q", s).WithSuggestion("use owner, admin or member")
}

// Manages reports whether r is owner or admin.
func (r Role) Manages() bool { return r == RoleOwner || r == RoleAdmin }

// Member is a person using this hub (through the console app). Members are
// local to the hub: there is no global account. A member's credentials are
// console tokens; nodes, machines and links they add are theirs.
type Member struct {
	ID string
	// Name is the member's handle, shown as the sender of what they send
	// ("@glenn"); same rules as agent names. DisplayName is cosmetic.
	Name        string
	DisplayName string
	Role        Role
	CreatedAt   time.Time
	RemovedAt   time.Time
}

// Active reports whether the member has not been removed.
func (m *Member) Active() bool { return m.RemovedAt.IsZero() }

// Label is the member's display name, or handle.
func (m *Member) Label() string {
	if m.DisplayName != "" {
		return m.DisplayName
	}
	return m.Name
}

// ParseMemberName validates a member handle (as agent names).
func ParseMemberName(s string) (string, error) {
	n, err := ParseAgentName(strings.ToLower(strings.TrimSpace(s)))
	if err != nil {
		return "", Errf(CodeInvalidArgument, "invalid member name %q", s).
			WithSuggestion("use 1-63 chars of lowercase letters, digits, '.', '_' or '-', e.g. glenn")
	}
	return string(n), nil
}

// MemberRepository persists members.
type MemberRepository interface {
	Create(ctx context.Context, m *Member) error
	Get(ctx context.Context, id string) (*Member, error)
	GetByName(ctx context.Context, name string) (*Member, error)
	List(ctx context.Context) ([]*Member, error)
	Save(ctx context.Context, m *Member) error
	// AssignUnowned gives memberID everything that has no owner yet:
	// agents, machines, enrollments and console tokens (for hubs upgraded
	// from before members existed).
	AssignUnowned(ctx context.Context, memberID string) error
}

// AuditEntry records one administrative action: who did what to what.
// Entries are only ever appended.
type AuditEntry struct {
	ID string
	At time.Time
	// Actor is a member id ("" for the hub's admin token, or an agent).
	Actor     string
	ActorName string // member or agent name at the time ("admin" for the admin token)
	ActorKind string // member | admin | agent
	// Action is a dotted verb, e.g. agent.remove, member.invite.
	Action string
	// Target is what was acted on (an agent name, member name, machine id).
	Target string
	// Detail is a short human-readable note (never message bodies or secrets).
	Detail string
}

// AuditFilter narrows an audit listing.
type AuditFilter struct {
	Before time.Time // entries strictly before (zero = now)
	Actor  string    // actor name
	Target string
	Limit  int
}

// AuditRepository stores the audit log (append-only).
type AuditRepository interface {
	Append(ctx context.Context, e *AuditEntry) error
	List(ctx context.Context, f AuditFilter) ([]*AuditEntry, error)
}
