package api

import "time"

// WorkflowRunView is an explicit, content-limited run projection. It does not
// embed WorkflowRun: new full-run fields must not silently enter these views.
type WorkflowRunView struct {
	ID         string             `json:"id"`
	WorkflowID string             `json:"workflow_id"`
	Name       string             `json:"name"`
	Version    int                `json:"version"`
	Owner      string             `json:"owner"`
	Project    string             `json:"project"`
	Status     string             `json:"status"`
	Inputs     map[string]any     `json:"inputs,omitempty"`
	By         string             `json:"by,omitempty"`
	Trigger    string             `json:"trigger,omitempty"`
	Error      string             `json:"error,omitempty"`
	View       string             `json:"view"`
	Steps      []WorkflowStepView `json:"steps"`
	CreatedAt  time.Time          `json:"created_at"`
	UpdatedAt  time.Time          `json:"updated_at"`
	EndedAt    *time.Time         `json:"ended_at,omitempty"`
}
type WorkflowStepView struct {
	ID           string     `json:"id"`
	Step         string     `json:"step"`
	Attempt      int        `json:"attempt"`
	Status       string     `json:"status"`
	Error        string     `json:"error"`
	StartedAt    time.Time  `json:"started_at"`
	EndedAt      *time.Time `json:"ended_at"`
	OutputSHA256 string     `json:"output_sha256"`
	// A pointer preserves an explicit empty output object in the step view.
	// Compact never sets it. Nil output has no hash and no output field.
	Output *map[string]any `json:"output,omitempty"`
}
