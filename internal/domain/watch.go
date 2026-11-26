package domain

import (
	"context"
	"time"
)

const (
	WatchWatching        = "watching"
	WatchMet             = "met"
	WatchExpired         = "expired"
	WatchStopped         = "stopped"
	WatchDefaultDuration = 30 * time.Minute
	WatchMaxDuration     = 24 * time.Hour
)

// Watch is a durable, bounded wait. Terminal states cannot be reopened.
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

func (w *Watch) Extend(now time.Time, d time.Duration) error {
	if w.State != WatchWatching || !now.Before(w.Deadline) {
		return Errf(CodeInvalidState, "only an active watch can be extended")
	}
	if d <= 0 || d > WatchMaxDuration || now.Add(d).After(w.StartedAt.Add(WatchMaxDuration)) {
		return Errf(CodeInvalidArgument, "watch deadline must be within 24 hours of its start")
	}
	w.Deadline = now.Add(d)
	return nil
}

func (w *Watch) End(now time.Time, state, by, outcome string) error {
	if w.State != WatchWatching {
		return Errf(CodeInvalidState, "watch %s is already %s", w.ID, w.State)
	}
	if state != WatchMet && state != WatchExpired && state != WatchStopped {
		return Errf(CodeInvalidArgument, "invalid watch state")
	}
	if state == WatchExpired && now.Before(w.Deadline) {
		return Errf(CodeInvalidState, "watch has not expired")
	}
	w.State, w.EndedAt, w.EndedBy, w.Outcome = state, &now, by, outcome
	return nil
}

// WatchRepository commits a watch outcome with any held-task resumption and
// wake message, guarded by the previous deadline/check counter.
type WatchRepository interface {
	Create(context.Context, *Watch) error
	Get(context.Context, string) (*Watch, error)
	List(context.Context, bool) ([]*Watch, error)
	Hold(context.Context, string, *Watch, func(*Message) error) (*Message, error)
	Save(context.Context, *Watch, time.Time, int, *Message) (bool, *Message, *Message, error)
}

// WatchCheck is a bounded, redacted command check summary.
type WatchCheck struct {
	At       time.Time `json:"at"`
	Result   string    `json:"result"`
	Matched  bool      `json:"matched"`
	ExitCode int       `json:"exit_code"`
}
