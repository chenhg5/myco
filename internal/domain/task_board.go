package domain

import (
	"context"
	"strings"
	"time"
)

// TaskRecord contains only scheduling metadata, never message contents.
type TaskRecord struct {
	ID, ThreadID, Project, SenderOwner string
	Sender, Recipient                  AgentName
	Status                             Status
	CreatedAt, UpdatedAt, ActiveAt     time.Time
	RecipientSeenAt                    time.Time
	StaleAfter                         time.Duration
}

// LastActivity follows Message.LastActive: a fresh state transition also
// establishes activity even if the stored recipient heartbeat is older.
func (t TaskRecord) LastActivity() time.Time {
	at := t.ActiveAt
	if t.UpdatedAt.After(at) {
		at = t.UpdatedAt
	}
	if at.IsZero() {
		return t.CreatedAt
	}
	return at
}

type TaskFilter struct {
	Sender       AgentName
	ConsoleOwner string
	Project      string
}

// TaskFailure is captured atomically when a task first fails, including the
// verified machine/owner at that moment. Later agent moves cannot rewrite it.
type TaskFailure struct {
	ID, Project, Machine, MachineOwner, SenderOwner string
	Sender, Recipient                               AgentName
	At                                              time.Time
	Incident                                        int64
}

type TaskAlert struct {
	Key, Kind, TaskID, Project, SenderOwner string
	Sender, Recipient                       AgentName
	Agents                                  int
	At                                      time.Time
}

// DeliveryKey follows a human account across renames without merging agents
// owned by that person. Agent names remain their existing logical identities.
func (a TaskAlert) DeliveryKey() string {
	if strings.HasPrefix(string(a.Sender), "@console:") && a.SenderOwner != "" {
		return "@member:" + a.SenderOwner
	}
	return string(a.Sender)
}

// TaskMonitorRepository keeps alert decisions and their inbox delivery durable.
// DeliverTaskAlerts checks the cooldown and commits the note, all dedup verdicts
// and the cooldown together. build must not do I/O while the transaction is held.
type TaskMonitorRepository interface {
	ListTasks(context.Context, TaskFilter) ([]TaskRecord, error)
	TaskFailures(context.Context, time.Time) ([]TaskFailure, error)
	TaskAlertThreshold(context.Context, string) (time.Duration, error)
	SetTaskAlertThreshold(context.Context, string, time.Duration) error
	StageTaskAlerts(context.Context, []TaskAlert) error
	PendingTaskAlertSenders(context.Context) ([]string, error)
	DeliverTaskAlerts(context.Context, string, time.Time, time.Duration, time.Duration, func([]TaskAlert) *Message) (*Message, error)
}
