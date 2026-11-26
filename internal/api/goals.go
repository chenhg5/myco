package api

import (
	"github.com/myco-ai/myco/internal/domain"
	"time"
)

type Goal = domain.Goal
type GoalRound = domain.GoalRound
type GoalDetail struct {
	Goal   Goal        `json:"goal"`
	Rounds []GoalRound `json:"rounds"`
}
type GoalList struct {
	Goals []Goal `json:"goals"`
}
type GoalCreateRequest struct {
	Agent     string `json:"agent"`
	Objective string `json:"objective"`
	DoneWhen  string `json:"done_when"`
	Project   string `json:"project,omitempty"`
	Thread    string `json:"thread,omitempty"`
	EverySec  int    `json:"every_sec,omitempty"`
	MaxRounds int    `json:"max_rounds,omitempty"`
	ForSec    int    `json:"for_sec,omitempty"`
}
type GoalUpdateRequest struct {
	EverySec  *int `json:"every_sec,omitempty"`
	MaxRounds *int `json:"max_rounds,omitempty"`
	ForSec    *int `json:"for_sec,omitempty"`
}
type GoalControlRequest struct {
	Reason string `json:"reason,omitempty"`
}
type GoalReportRequest struct {
	Message    string `json:"message"`
	Summary    string `json:"summary"`
	NextStep   string `json:"next_step,omitempty"`
	Progressed *bool  `json:"progressed"`
	Blocked    bool   `json:"blocked,omitempty"`
	Achieved   bool   `json:"achieved,omitempty"`
	Abandoned  bool   `json:"abandoned,omitempty"`
	Reason     string `json:"reason,omitempty"`
}
type GoalBrief struct {
	ID         string     `json:"id"`
	Objective  string     `json:"objective"`
	Round      int        `json:"round"`
	MaxRounds  int        `json:"max_rounds"`
	NextWakeAt *time.Time `json:"next_wake_at,omitempty"`
	State      string     `json:"state"`
}
