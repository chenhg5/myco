package domain

import (
	"context"
	"time"
)

// TraceEvent is one step of the run that handled a task (what the agent
// said, thought and did), uploaded by the recipient's runner for the
// sender's live view. Data is the step as JSON (harness.Event).
type TraceEvent struct {
	Message string // task message id
	Seq     int    // 1, 2, … in run order
	Kind    string
	Data    string
	At      time.Time
}

const (
	// MaxTraceEvent bounds one step's data.
	MaxTraceEvent = 16 << 10
	// MaxTraceBytes bounds a task's whole trace.
	MaxTraceBytes = 2 << 20
	// MaxTraceBatch bounds the steps in one upload.
	MaxTraceBatch = 500
	// TraceRetention is how long traces are kept.
	TraceRetention = 7 * 24 * time.Hour
)

// TraceRepository stores run traces.
type TraceRepository interface {
	// Append adds steps; a (message, seq) already stored is skipped.
	Append(ctx context.Context, events []*TraceEvent) error
	// List returns a message's steps after seq, in order.
	List(ctx context.Context, message string, after, limit int) ([]*TraceEvent, error)
	// Bytes is the size of a message's trace so far.
	Bytes(ctx context.Context, message string) (int64, error)
	// DeleteBefore removes steps older than t; it returns how many.
	DeleteBefore(ctx context.Context, t time.Time) (int, error)
}
