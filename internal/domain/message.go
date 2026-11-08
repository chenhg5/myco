package domain

import (
	"strings"
	"time"
)

// MessageType classifies a message.
type MessageType string

const (
	// TypeTask expects exactly one reply (or a failure).
	TypeTask MessageType = "task"
	// TypeNote is informational; no reply expected.
	TypeNote MessageType = "note"
	// TypeReply answers a task. Created only via Message.Reply.
	TypeReply MessageType = "reply"
	// TypeEvent is emitted by the hub (e.g. a task failed).
	TypeEvent MessageType = "event"
	// TypeProgress is an interim update on a task from its recipient. It is
	// stored closed: it never wakes the sender, who sees it while waiting
	// for the reply (myco message wait) or in the thread.
	TypeProgress MessageType = "progress"
	// TypeSteer is a correction from a task's sender while the task is
	// open. Runners interrupt the task's run and resume it with the steer.
	TypeSteer MessageType = "steer"
)

// ParseSendableType validates a type an agent may send directly.
func ParseSendableType(s string) (MessageType, error) {
	switch MessageType(s) {
	case "", TypeTask:
		return TypeTask, nil
	case TypeNote:
		return TypeNote, nil
	}
	return "", Errf(CodeInvalidArgument, "invalid message type %q", s).
		WithSuggestion("use task or note; replies are sent with `myco message reply`")
}

// ExpectsReply reports whether the recipient owes the sender an answer.
func (t MessageType) ExpectsReply() bool { return t == TypeTask }

// Status is the delivery/processing state of a message.
//
//	queued ──pull──▶ delivered ──ack──▶ acked ──reply/done──▶ done
//	   ▲                 │  ▲              ├──hold──▶ held ──reply/done──▶ done
//	   └──lease expires──┘  └───release────┘   any open state ──fail──▶ failed
//	queued ──done──▶ done   (dismiss without pulling)
type Status string

const (
	StatusQueued    Status = "queued"
	StatusDelivered Status = "delivered"
	StatusAcked     Status = "acked"
	StatusHeld      Status = "held"
	StatusDone      Status = "done"
	StatusFailed    Status = "failed"
)

// Terminal reports whether no further transitions are possible.
func (s Status) Terminal() bool { return s == StatusDone || s == StatusFailed }

// MaxBodyBytes bounds a message body.
const MaxBodyBytes = 256 * 1024

// Message is the aggregate root for a single unit of communication.
type Message struct {
	ID             string
	From           AgentName
	To             AgentName
	ThreadID       string
	ReplyTo        string
	Type           MessageType
	Body           string
	Status         Status
	Attempts       int
	LeaseUntil     time.Time
	Error          string
	IdempotencyKey string
	// BroadcastID groups the per-recipient messages of one broadcast.
	BroadcastID string
	// Attachments reference uploaded files.
	Attachments []Attachment
	// FromOwner is the person behind the sender when it was sent: the
	// member who wrote it in the app, or the sending agent's owner.
	FromOwner string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewMessage builds a queued message. An empty threadID starts a new thread.
func NewMessage(id string, from, to AgentName, typ MessageType, body, threadID, replyTo, idemKey string, now time.Time) (*Message, error) {
	if strings.TrimSpace(body) == "" {
		return nil, Errf(CodeInvalidArgument, "message body is empty")
	}
	if len(body) > MaxBodyBytes {
		return nil, Errf(CodeInvalidArgument, "message body is %d bytes, limit is %d", len(body), MaxBodyBytes).
			WithSuggestion("put large content in a shared location and send a reference")
	}
	if len(idemKey) > 128 {
		return nil, Errf(CodeInvalidArgument, "idempotency key longer than 128 bytes")
	}
	if threadID == "" {
		threadID = id
	}
	return &Message{
		ID: id, From: from, To: to, ThreadID: threadID, ReplyTo: replyTo,
		Type: typ, Body: body, Status: StatusQueued, IdempotencyKey: idemKey,
		CreatedAt: now, UpdatedAt: now,
	}, nil
}

// Involves reports whether agent is the sender or recipient.
func (m *Message) Involves(agent AgentName) bool { return m.From == agent || m.To == agent }

// IsDue reports whether the message can be handed to its recipient now.
func (m *Message) IsDue(now time.Time) bool {
	return m.Status == StatusQueued || (m.Status == StatusDelivered && !now.Before(m.LeaseUntil))
}

// Claim leases the message to its recipient. It returns false (and marks the
// message failed) when delivery attempts are exhausted.
func (m *Message) Claim(now time.Time, lease time.Duration, maxAttempts int) bool {
	if maxAttempts > 0 && m.Attempts >= maxAttempts {
		m.Status = StatusFailed
		m.Error = "delivery attempts exhausted: recipient pulled but never acknowledged"
		m.UpdatedAt = now
		return false
	}
	m.Status = StatusDelivered
	m.Attempts++
	m.LeaseUntil = now.Add(lease)
	m.UpdatedAt = now
	return true
}

func (m *Message) requireRecipient(by AgentName) error {
	if m.To != by {
		return Errf(CodePermissionDenied, "message %s is addressed to %s, not %s", m.ID, m.To, by)
	}
	return nil
}

func (m *Message) requireOpen(op string, allowed ...Status) error {
	for _, s := range allowed {
		if m.Status == s {
			return nil
		}
	}
	e := Errf(CodeInvalidState, "cannot %s message %s in status %s", op, m.ID, m.Status)
	switch {
	case m.Status == StatusQueued:
		return e.WithSuggestion("pull it first: myco inbox pull")
	case m.Status.Terminal():
		return e.WithSuggestion("the message is closed; start a follow-up with: myco message send --thread %s", m.ThreadID)
	}
	return e
}

// Ack confirms the recipient has taken ownership (stops lease redelivery).
func (m *Message) Ack(by AgentName, now time.Time) error {
	if err := m.requireRecipient(by); err != nil {
		return err
	}
	if m.Status == StatusAcked {
		return nil // idempotent
	}
	if err := m.requireOpen("ack", StatusDelivered); err != nil {
		return err
	}
	m.Status, m.UpdatedAt = StatusAcked, now
	return nil
}

// Hold marks the message as accepted but answered later (e.g. while waiting
// on delegated sub-tasks). Runners do not auto-reply to held messages.
func (m *Message) Hold(by AgentName, now time.Time) error {
	if err := m.requireRecipient(by); err != nil {
		return err
	}
	if m.Status == StatusHeld {
		return nil
	}
	if err := m.requireOpen("hold", StatusDelivered, StatusAcked); err != nil {
		return err
	}
	m.Status, m.UpdatedAt = StatusHeld, now
	return nil
}

// MaxReleaseDelay bounds how long a released message can be deferred.
const MaxReleaseDelay = time.Hour

// Release hands a message back for later redelivery without counting it as a
// failed delivery, e.g. when the recipient's session is busy (a human is
// using it). It becomes due again after delay.
func (m *Message) Release(by AgentName, delay time.Duration, now time.Time) error {
	if err := m.requireRecipient(by); err != nil {
		return err
	}
	if err := m.requireOpen("release", StatusDelivered, StatusAcked); err != nil {
		return err
	}
	if delay < 0 || delay > MaxReleaseDelay {
		return Errf(CodeInvalidArgument, "release delay must be between 0 and %s", MaxReleaseDelay)
	}
	m.Status, m.LeaseUntil, m.UpdatedAt = StatusDelivered, now.Add(delay), now
	if m.Attempts > 0 {
		m.Attempts-- // the next claim re-increments it: busy retries are free
	}
	return nil
}

// Complete closes the message without a reply.
func (m *Message) Complete(by AgentName, now time.Time) error {
	if err := m.requireRecipient(by); err != nil {
		return err
	}
	if m.Status == StatusDone {
		return nil
	}
	// Queued too: the recipient may close a message it never pulled (e.g. a
	// late answer nobody waits for any more) without pulling it first.
	if err := m.requireOpen("complete", StatusQueued, StatusDelivered, StatusAcked, StatusHeld); err != nil {
		return err
	}
	m.Status, m.UpdatedAt = StatusDone, now
	return nil
}

// Reply closes the message and returns the reply addressed to the sender.
func (m *Message) Reply(by AgentName, replyID, body string, now time.Time) (*Message, error) {
	if err := m.requireRecipient(by); err != nil {
		return nil, err
	}
	if err := m.requireOpen("reply to", StatusDelivered, StatusAcked, StatusHeld); err != nil {
		return nil, err
	}
	r, err := NewMessage(replyID, m.To, m.From, TypeReply, body, m.ThreadID, m.ID, "", now)
	if err != nil {
		return nil, err
	}
	m.Status, m.UpdatedAt = StatusDone, now
	return r, nil
}

// Progress returns an interim update on task m from its recipient. The task
// itself does not change.
func (m *Message) Progress(by AgentName, id, body string, now time.Time) (*Message, error) {
	if err := m.requireRecipient(by); err != nil {
		return nil, err
	}
	if m.Type != TypeTask {
		return nil, Errf(CodeInvalidArgument, "progress applies to tasks; %s is a %s", m.ID, m.Type)
	}
	if err := m.requireOpen("report progress on", StatusDelivered, StatusAcked, StatusHeld); err != nil {
		return nil, err
	}
	p, err := NewMessage(id, m.To, m.From, TypeProgress, body, m.ThreadID, m.ID, "", now)
	if err != nil {
		return nil, err
	}
	p.Status = StatusDone
	return p, nil
}

// Steer returns a correction to open task m from its sender, delivered to
// the recipient like any message.
func (m *Message) Steer(by AgentName, id, body string, now time.Time) (*Message, error) {
	if m.From != by {
		return nil, Errf(CodePermissionDenied, "only the sender of %s (%s) can steer it", m.ID, m.From)
	}
	if m.Type != TypeTask {
		return nil, Errf(CodeInvalidArgument, "only tasks can be steered; %s is a %s", m.ID, m.Type)
	}
	if m.Status.Terminal() {
		return nil, Errf(CodeInvalidState, "task %s is already %s", m.ID, m.Status).
			WithSuggestion("send a follow-up instead: myco message send --to %s --thread %s --body ...", m.To, m.ThreadID)
	}
	return NewMessage(id, m.From, m.To, TypeSteer, body, m.ThreadID, m.ID, "", now)
}

// Fail closes the message as failed. If the sender expects a reply, an event
// message notifying the sender is returned.
func (m *Message) Fail(by AgentName, eventID, reason string, now time.Time) (*Message, error) {
	if err := m.requireRecipient(by); err != nil {
		return nil, err
	}
	if err := m.requireOpen("fail", StatusDelivered, StatusAcked, StatusHeld); err != nil {
		return nil, err
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "no reason given"
	}
	m.Status, m.Error, m.UpdatedAt = StatusFailed, reason, now
	if !m.Type.ExpectsReply() {
		return nil, nil
	}
	return NewMessage(eventID, m.To, m.From, TypeEvent, "task failed: "+reason, m.ThreadID, m.ID, "", now)
}
