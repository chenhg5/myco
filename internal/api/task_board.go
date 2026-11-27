package api

import "time"

// TaskBoard omits message contents and credentials, including under --all.
type TaskBoard struct {
	ObservedAt time.Time       `json:"observed_at"`
	Tasks      []TaskBoardItem `json:"tasks"`
}
type TaskBoardItem struct {
	ID             string     `json:"id"`
	From           string     `json:"from"`
	To             string     `json:"to"`
	ThreadID       string     `json:"thread_id"`
	Project        string     `json:"project,omitempty"`
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"created_at"`
	ActiveAt       *time.Time `json:"active_at,omitempty"`
	LastActivityAt time.Time  `json:"last_activity_at"`
	AgeMS          int64      `json:"age_ms"`
	// QueuedForMS is age since creation for currently queued tasks, zero
	// otherwise. It does not reconstruct queue episodes after a requeue.
	QueuedForMS  int64 `json:"queued_for_ms"`
	IdleMS       int64 `json:"idle_ms"`
	StaleAfterMS int64 `json:"stale_after_ms"`
	Stale        bool  `json:"stale"`
}
type TaskAlertPolicy struct {
	Project      string `json:"project"`
	StaleSeconds int    `json:"stale_seconds"`
}
type TaskAlertPolicyRequest struct {
	StaleSeconds int `json:"stale_seconds"`
}
