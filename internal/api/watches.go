package api

import (
	"github.com/myco-ai/myco/internal/domain"
	"time"
)

type Watch struct {
	ID          string       `json:"id"`
	Agent       string       `json:"agent"`
	Project     string       `json:"project,omitempty"`
	Task        string       `json:"task,omitempty"`
	What        string       `json:"what"`
	Kind        string       `json:"kind"`
	Target      string       `json:"target,omitempty"`
	Command     string       `json:"command,omitempty"`
	Match       string       `json:"match,omitempty"`
	EverySec    int          `json:"every_sec,omitempty"`
	MachineID   string       `json:"machine_id,omitempty"`
	Environment string       `json:"environment,omitempty"`
	Workdir     string       `json:"workdir,omitempty"`
	Revision    int          `json:"revision,omitempty"`
	ClaimID     string       `json:"claim_id,omitempty"`
	ClaimRunner string       `json:"claim_runner,omitempty"`
	ClaimUntil  *time.Time   `json:"claim_until,omitempty"`
	NextCheckAt *time.Time   `json:"next_check_at,omitempty"`
	LastCheckID string       `json:"last_check_id,omitempty"`
	History     []WatchCheck `json:"history,omitempty"`

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
	What     string `json:"what"`
	Kind     string `json:"kind"`
	Target   string `json:"target,omitempty"`
	Command  string `json:"command,omitempty"`
	Match    string `json:"match,omitempty"`
	EverySec int    `json:"every_sec,omitempty"`
	Project  string `json:"project,omitempty"`
	Task     string `json:"task,omitempty"`
	ForSec   int    `json:"for_sec,omitempty"`
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

type WatchCheck = domain.WatchCheck

type WatchClaimRequest struct {
	Runner string `json:"runner"`
}
type WatchCheckRequest struct {
	CheckID  string `json:"check_id"`
	Runner   string `json:"runner"`
	Result   string `json:"result"`
	Matched  bool   `json:"matched"`
	ExitCode int    `json:"exit_code"`
}
