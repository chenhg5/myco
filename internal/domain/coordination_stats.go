package domain

import (
	"context"
	"time"
)

// CoordinationFilter restricts metadata to messages the caller may read.
// Names nil means all; a non-nil empty slice means none.
type CoordinationFilter struct {
	Since, Until time.Time
	Project      string
	Agent        AgentName // optional participant selector for --name
	Names        []AgentName
}

// TaskTiming contains no message content, credentials, or error text.
type TaskTiming struct {
	ID, Thread                                string
	From, To                                  AgentName
	Created, Updated, Replied, Started        time.Time
	Done, Failed, Stopped, Redone, Reassigned bool
	Followups                                 int
}

type CoordinationRepository interface {
	TaskTimings(context.Context, CoordinationFilter) ([]TaskTiming, error)
}
