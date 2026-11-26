package domain

import (
	"context"
	"time"
)

// Goal schedules bounded turns toward an immutable objective.
type Goal struct {
	ID           string     `json:"id"`
	Agent        string     `json:"agent"`
	Project      string     `json:"project,omitempty"`
	Thread       string     `json:"thread,omitempty"`
	Objective    string     `json:"objective"`
	DoneWhen     string     `json:"done_when"`
	EverySec     int        `json:"every_sec"`
	BaseEverySec int        `json:"base_every_sec"`
	MaxRounds    int        `json:"max_rounds"`
	Deadline     time.Time  `json:"deadline"`
	Round        int        `json:"round"`
	NoProgress   int        `json:"no_progress"`
	LastSummary  string     `json:"last_summary,omitempty"`
	NextStep     string     `json:"next_step,omitempty"`
	NextWakeAt   *time.Time `json:"next_wake_at,omitempty"`
	State        string     `json:"state"`
	StartedAt    time.Time  `json:"started_at"`
	EndedAt      *time.Time `json:"ended_at,omitempty"`
	EndedBy      string     `json:"ended_by,omitempty"`
	Outcome      string     `json:"outcome,omitempty"`
	CreatedBy    string     `json:"created_by"`
}
type GoalRound struct {
	Goal       string     `json:"goal"`
	Round      int        `json:"round"`
	StartedAt  time.Time  `json:"started_at"`
	EndedAt    *time.Time `json:"ended_at,omitempty"`
	Summary    string     `json:"summary"`
	NextStep   string     `json:"next_step"`
	Progressed bool       `json:"progressed"`
	Blocked    bool       `json:"blocked"`
	Message    string     `json:"message"`
}
type GoalRecord struct {
	Goal        Goal              `json:"goal"`
	Rounds      []GoalRound       `json:"rounds"`
	Reports     map[string]string `json:"reports,omitempty"`
	CreatorKind string            `json:"creator_kind"`
	CreatorID   string            `json:"creator_id"`
	SummaryTo   AgentName         `json:"summary_to"`
}

func (g *Goal) Open() bool { return g.State == "active" || g.State == "paused" }
func (g *Goal) End(now time.Time, state, by, reason string) {
	g.State = state
	g.EndedBy = by
	g.Outcome = reason
	g.EndedAt = &now
	g.NextWakeAt = nil
}

// FinishRound applies progress and backoff. Scheduling waits for the task's terminal state.
func (g *Goal) FinishRound(now time.Time, r *GoalRound) {
	g.LastSummary = r.Summary
	g.NextStep = r.NextStep
	if r.Progressed {
		g.NoProgress = 0
		g.EverySec = g.BaseEverySec
	} else {
		g.NoProgress++
		if g.NoProgress >= 2 {
			g.EverySec *= 2
			if g.EverySec > 14400 {
				g.EverySec = 14400
			}
		}
	}
	if g.Open() && (r.Blocked || g.NoProgress >= 4) {
		g.State = "paused"
		g.Outcome = "blocked or four rounds without progress"
		g.NextWakeAt = nil
	}
}

type GoalMutation func(*GoalRecord, *Message, bool) ([]*Message, error)
type GoalRepository interface {
	Create(context.Context, *GoalRecord) error
	Get(context.Context, string) (*GoalRecord, error)
	List(context.Context, bool) ([]*GoalRecord, error)
	// Mutate serializes lifecycle updates and inserts wakes in the same transaction.
	Mutate(context.Context, string, GoalMutation) (*GoalRecord, []*Message, error)
}
