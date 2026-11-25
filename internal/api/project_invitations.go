package api

import "time"

type ProjectInviteRequest struct {
	DisplayName string `json:"display_name,omitempty"`
	Description string `json:"description,omitempty"`
	Note        string `json:"note,omitempty"`
}

// Link and Sentence are credentials, returned only on issuance, never in events.
type ProjectInviteResponse struct {
	ID         string    `json:"id"`
	ApprovalID string    `json:"approval_id,omitempty"`
	Link       string    `json:"link"`
	Sentence   string    `json:"sentence"`
	ExpiresAt  time.Time `json:"expires_at"`
	Status     string    `json:"status"`
}
