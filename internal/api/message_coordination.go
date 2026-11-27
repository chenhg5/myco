package api

import "time"

type TaskTreeWait struct {
	Kind string `json:"kind"`
	ID   string `json:"id,omitempty"`
}
type TaskTreeNode struct {
	ID         string         `json:"id"`
	From       string         `json:"from"`
	Agent      string         `json:"agent"`
	Status     string         `json:"status"`
	AgeSeconds int64          `json:"age_seconds"`
	ThreadID   string         `json:"thread_id"`
	Project    string         `json:"project,omitempty"`
	ParentID   string         `json:"parent_id,omitempty"`
	Relation   string         `json:"relation"`
	Reference  bool           `json:"reference,omitempty"`
	WaitingOn  []TaskTreeWait `json:"waiting_on"`
	Children   []TaskTreeNode `json:"children"`
}
type TaskTreeResponse struct {
	Root      TaskTreeNode `json:"root"`
	AsOf      time.Time    `json:"as_of"`
	Truncated bool         `json:"truncated"`
}
type ReassignRequest struct {
	To   string `json:"to"`
	Note string `json:"note,omitempty"`
}
type ReassignResponse struct {
	ID         string `json:"id"`
	From       string `json:"from"`
	To         string `json:"to"`
	PreviousTo string `json:"previous_to"`
	ThreadID   string `json:"thread_id"`
	Project    string `json:"project,omitempty"`
	Status     string `json:"status"`
	DryRun     bool   `json:"dry_run,omitempty"`
}
