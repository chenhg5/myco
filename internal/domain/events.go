package domain

// Events: what happened in the hub, told after it is stored. Features that
// react to what others do (notices, and later audit trails, integrations)
// subscribe to these instead of being called by the services that cause
// them, so neither knows the other. They are published by the repositories'
// observers (app.ObserveMessages and the like), at the composition root.

// Event is something that happened.
type Event interface{ EventName() string }

// MessageCreated: a message was stored (sent, or made by another: a reply,
// a progress update, a failure event, a steer).
type MessageCreated struct{ Message *Message }

// MessageChanged: a stored message changed (its state, its question).
type MessageChanged struct{ Message *Message }

// ApprovalChanged: a superagent's request was made or decided.
type ApprovalChanged struct{ Approval *Approval }

// StepChanged: a workflow run's step was stored (started, waiting,
// finished).
type StepChanged struct{ Step *RunStep }

// RunChanged: a workflow run was stored.
type RunChanged struct{ Run *WorkflowRun }

func (MessageCreated) EventName() string  { return "message.created" }
func (MessageChanged) EventName() string  { return "message.changed" }
func (ApprovalChanged) EventName() string { return "approval.changed" }
func (StepChanged) EventName() string     { return "workflow.step" }
func (RunChanged) EventName() string      { return "workflow.run" }
