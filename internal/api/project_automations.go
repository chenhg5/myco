package api

// ProjectAutomationResponse is either an applied change or a specific request
// awaiting its project owner's approval. Approval never grants blanket access.
type ProjectAutomationResponse struct {
	Status     string      `json:"status"`
	ApprovalID string      `json:"approval_id,omitempty"`
	Automation *Automation `json:"automation,omitempty"`
	Removed    string      `json:"removed,omitempty"`
}
