package api

import "time"

type Watch struct {
	ID          string     `json:"id"`
	Agent       string     `json:"agent"`
	Project     string     `json:"project,omitempty"`
	Task        string     `json:"task,omitempty"`
	What        string     `json:"what"`
	Kind        string     `json:"kind"`
	Target      string     `json:"target,omitempty"`
	Command     string     `json:"command,omitempty"`
	Match       string     `json:"match,omitempty"`
	EverySec    int        `json:"every_sec,omitempty"`
	StartedAt   time.Time  `json:"started_at"`
	Deadline    time.Time  `json:"deadline"`
	LastCheckAt *time.Time `json:"last_check_at,omitempty"`
	LastResult  string     `json:"last_result,omitempty"`
	Checks      int        `json:"checks"`
	State       string     `json:"state"`
	EndedAt     *time.Time `json:"ended_at,omitempty"`
	EndedBy     string     `json:"ended_by,omitempty"`
	Outcome     string     `json:"outcome,omitempty"`
}

type WatchStartRequest struct {
	What    string `json:"what"`
	Kind    string `json:"kind"`
	Target  string `json:"target"`
	Project string `json:"project,omitempty"`
	Task    string `json:"task,omitempty"`
	ForSec  int    `json:"for_sec,omitempty"`
}
type WatchList struct {
	Watches []Watch `json:"watches"`
}
type WatchStopRequest struct {
	Reason string `json:"reason,omitempty"`
}
type WatchExtendRequest struct {
	ForSec int `json:"for_sec"`
}
