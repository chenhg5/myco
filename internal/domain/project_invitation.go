package domain

import (
	"context"
	"time"
)

// ProjectInvitation contains no credential. Enrollment holds only its hash.
type ProjectInvitation struct {
	ID           string    `json:"id"`
	Project      string    `json:"project"`
	Requester    AgentName `json:"requester"`
	Owner        string    `json:"owner"`
	DisplayName  string    `json:"display_name"`
	Description  string    `json:"description"`
	Note         string    `json:"note"`
	Status       string    `json:"status"`
	ApprovalID   string    `json:"approval_id,omitempty"`
	EnrollmentID string    `json:"enrollment_id,omitempty"`
	UsedBy       AgentName `json:"used_by,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// ProjectInvitationRepository atomically issues credentials and admits an agent.
type ProjectInvitationRepository interface {
	Create(context.Context, *ProjectInvitation) error
	Get(context.Context, string) (*ProjectInvitation, error)
	SetApproval(context.Context, string, string) error
	Decide(context.Context, string, string, time.Time) error
	Issue(context.Context, string, *Enrollment, time.Time) error
	Use(context.Context, string, *Agent, time.Time) error
}
