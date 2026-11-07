package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func newTask(t *testing.T) *Message {
	t.Helper()
	m, err := NewMessage("msg_1", "alice", "bob", TypeTask, "do it", "", "", "", time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestMessageLifecycle(t *testing.T) {
	now := time.Unix(100, 0)
	m := newTask(t)
	if m.ThreadID != m.ID {
		t.Fatalf("new thread should use message id, got %q", m.ThreadID)
	}
	if err := m.Ack("bob", now); !IsCode(err, CodeInvalidState) {
		t.Fatalf("ack before pull: want invalid_state, got %v", err)
	}
	if !m.Claim(now, time.Minute, 3) || m.Status != StatusDelivered || m.Attempts != 1 {
		t.Fatalf("claim: %+v", m)
	}
	if err := m.Ack("mallory", now); !IsCode(err, CodePermissionDenied) {
		t.Fatalf("ack by stranger: want permission_denied, got %v", err)
	}
	if err := m.Ack("bob", now); err != nil {
		t.Fatal(err)
	}
	if err := m.Ack("bob", now); err != nil {
		t.Fatalf("ack must be idempotent: %v", err)
	}
	r, err := m.Reply("bob", "msg_2", "ok", now)
	if err != nil {
		t.Fatal(err)
	}
	if m.Status != StatusDone || r.To != "alice" || r.ReplyTo != "msg_1" || r.ThreadID != "msg_1" || r.Type != TypeReply {
		t.Fatalf("reply: m=%+v r=%+v", m, r)
	}
	if _, err := m.Reply("bob", "msg_3", "again", now); !IsCode(err, CodeInvalidState) {
		t.Fatalf("double reply: want invalid_state, got %v", err)
	}
}

func TestLeaseExpiryAndExhaustion(t *testing.T) {
	m := newTask(t)
	t0 := time.Unix(100, 0)
	m.Claim(t0, time.Minute, 2)
	if m.IsDue(t0.Add(30 * time.Second)) {
		t.Fatal("leased message must not be due")
	}
	if !m.IsDue(t0.Add(2 * time.Minute)) {
		t.Fatal("expired lease must be due")
	}
	m.Claim(t0.Add(2*time.Minute), time.Minute, 2)
	if m.Claim(t0.Add(4*time.Minute), time.Minute, 2) || m.Status != StatusFailed {
		t.Fatalf("third claim should fail the message: %+v", m)
	}
}

func TestFailNotifiesSenderOnlyForTasks(t *testing.T) {
	now := time.Unix(1, 0)
	task := newTask(t)
	task.Claim(now, time.Minute, 0)
	ev, err := task.Fail("bob", "msg_e", "boom", now)
	if err != nil || ev == nil || ev.Type != TypeEvent || ev.To != "alice" {
		t.Fatalf("task fail: ev=%+v err=%v", ev, err)
	}
	note, _ := NewMessage("msg_n", "alice", "bob", TypeNote, "fyi", "", "", "", now)
	note.Claim(now, time.Minute, 0)
	ev, err = note.Fail("bob", "msg_e2", "", now)
	if err != nil || ev != nil {
		t.Fatalf("note fail should not emit event: ev=%+v err=%v", ev, err)
	}
}

func TestValidation(t *testing.T) {
	for _, bad := range []string{"", "Bob", "-x", "a b", "a/b"} {
		if _, err := ParseAgentName(bad); err == nil {
			t.Errorf("name %q should be invalid", bad)
		}
	}
	if _, err := ParseSendableType("reply"); err == nil {
		t.Error("agents must not send raw replies")
	}
	if _, err := NewMessage("m", "a", "b", TypeTask, "   ", "", "", "", time.Now()); err == nil {
		t.Error("empty body should be rejected")
	}
	a := &Agent{AllowFrom: []AgentName{"alice"}}
	if !a.Accepts("alice") || a.Accepts("bob") {
		t.Error("allow-from policy broken")
	}
}

func TestReleaseDefersWithoutCountingAttempt(t *testing.T) {
	m := newTask(t)
	t0 := time.Unix(100, 0)
	m.Claim(t0, time.Minute, 3)
	m.Ack("bob", t0)
	if err := m.Release("bob", 2*time.Minute, t0); err != nil {
		t.Fatal(err)
	}
	if m.IsDue(t0.Add(time.Minute)) || !m.IsDue(t0.Add(3*time.Minute)) {
		t.Fatal("released message must become due only after the delay")
	}
	for i := range 10 { // many busy cycles never exhaust attempts
		now := t0.Add(time.Duration(3+i*3) * time.Minute)
		if !m.Claim(now, time.Minute, 3) {
			t.Fatalf("cycle %d: claim failed, attempts=%d", i, m.Attempts)
		}
		m.Release("bob", 2*time.Minute, now)
	}
	if err := m.Release("mallory", 0, t0); !IsCode(err, CodePermissionDenied) {
		t.Fatalf("stranger release: %v", err)
	}
	if err := m.Release("bob", 2*time.Hour, t0); !IsCode(err, CodeInvalidArgument) {
		t.Fatalf("delay bound: %v", err)
	}
}

func TestUpdateKeepsUnsetFields(t *testing.T) {
	a, _ := NewAgent("id", "x", AgentSpec{Description: "d", Capabilities: []string{"code"},
		Binding:  Binding{Harness: "codex", SessionID: "s1", Environment: "mac"},
		Delivery: Delivery{Mode: "webhook", URL: "https://h", Secret: "k"}}, "", time.Now())
	if err := a.Update(AgentSpec{}); err != nil {
		t.Fatal(err)
	}
	if a.Description != "d" || len(a.Capabilities) != 1 || a.Binding.SessionID != "s1" || a.Delivery.Secret != "k" || a.Delivery.URL != "https://h" {
		t.Fatalf("rotate-style update wiped fields: %+v", a)
	}
	if err := a.Update(AgentSpec{Description: "new", Binding: Binding{Workdir: "/w"}}); err != nil {
		t.Fatal(err)
	}
	if a.Description != "new" || a.Binding.Workdir != "/w" || a.Binding.SessionID != "s1" {
		t.Fatalf("partial update: %+v", a)
	}
}

// messageIn returns a message of type typ (alice -> bob) driven into status s
// through the public transitions.
func messageIn(t *testing.T, typ MessageType, s Status) *Message {
	t.Helper()
	now := time.Unix(10, 0)
	m, err := NewMessage("msg_s", "alice", "bob", typ, "body", "", "", "", now)
	if err != nil {
		t.Fatal(err)
	}
	if s == StatusQueued {
		return m
	}
	m.Claim(now, time.Minute, 0)
	switch s {
	case StatusDelivered:
	case StatusAcked:
		err = m.Ack("bob", now)
	case StatusHeld:
		err = m.Hold("bob", now)
	case StatusDone:
		err = m.Complete("bob", now)
	case StatusFailed:
		_, err = m.Fail("bob", "msg_ev", "setup", now)
	default:
		t.Fatalf("unknown status %s", s)
	}
	if err != nil || m.Status != s {
		t.Fatalf("driving to %s: status=%s err=%v", s, m.Status, err)
	}
	return m
}

type msgOp func(m *Message, by AgentName, now time.Time) error

var recipientOps = map[string]msgOp{
	"ack":  func(m *Message, by AgentName, now time.Time) error { return m.Ack(by, now) },
	"hold": func(m *Message, by AgentName, now time.Time) error { return m.Hold(by, now) },
	"release": func(m *Message, by AgentName, now time.Time) error {
		return m.Release(by, time.Minute, now)
	},
	"complete": func(m *Message, by AgentName, now time.Time) error { return m.Complete(by, now) },
	"reply": func(m *Message, by AgentName, now time.Time) error {
		_, err := m.Reply(by, "msg_r", "answer", now)
		return err
	},
	"fail": func(m *Message, by AgentName, now time.Time) error {
		_, err := m.Fail(by, "msg_e", "boom", now)
		return err
	},
	"progress": func(m *Message, by AgentName, now time.Time) error {
		_, err := m.Progress(by, "msg_p", "50%", now)
		return err
	},
}

// TestTransitionTable pins every (status, operation) pair of the diagram
// above `type Status`: the resulting status, or the error code with the
// status left unchanged.
func TestTransitionTable(t *testing.T) {
	const bad = "invalid_state"
	// want[status][op] is the resulting status, or an error code.
	want := map[Status]map[string]string{
		StatusQueued: {
			"ack": bad, "hold": bad, "release": bad, "complete": bad, "reply": bad, "fail": bad, "progress": bad,
		},
		StatusDelivered: {
			"ack": "acked", "hold": "held", "release": "delivered", "complete": "done", "reply": "done", "fail": "failed", "progress": "delivered",
		},
		StatusAcked: {
			"ack": "acked", "hold": "held", "release": "delivered", "complete": "done", "reply": "done", "fail": "failed", "progress": "acked",
		},
		StatusHeld: {
			"ack": bad, "hold": "held", "release": bad, "complete": "done", "reply": "done", "fail": "failed", "progress": "held",
		},
		StatusDone: {
			"ack": bad, "hold": bad, "release": bad, "complete": "done", "reply": bad, "fail": bad, "progress": bad,
		},
		StatusFailed: {
			"ack": bad, "hold": bad, "release": bad, "complete": bad, "reply": bad, "fail": bad, "progress": bad,
		},
	}
	later := time.Unix(500, 0)
	for from, ops := range want {
		for op, exp := range ops {
			t.Run(string(from)+"/"+op, func(t *testing.T) {
				m := messageIn(t, TypeTask, from)
				before := *m
				err := recipientOps[op](m, "bob", later)
				if exp == bad {
					if !IsCode(err, CodeInvalidState) {
						t.Fatalf("want invalid_state, got %v", err)
					}
					if m.Status != from || !m.UpdatedAt.Equal(before.UpdatedAt) {
						t.Fatalf("failed op changed the message: %+v", m)
					}
					var de *Error
					_ = errors.As(err, &de)
					switch {
					case from == StatusQueued && !strings.Contains(de.Suggestion, "pull"):
						t.Errorf("queued: suggestion should say pull first: %q", de.Suggestion)
					case from.Terminal() && !strings.Contains(de.Suggestion, "--thread "+m.ThreadID):
						t.Errorf("closed: suggestion should point at a follow-up: %q", de.Suggestion)
					}
					return
				}
				if err != nil {
					t.Fatalf("want %s, got error %v", exp, err)
				}
				if m.Status != Status(exp) {
					t.Fatalf("status = %s, want %s", m.Status, exp)
				}
				changed := m.Status != from || op == "release"
				if changed != m.UpdatedAt.Equal(later) {
					t.Errorf("UpdatedAt = %v (changed=%v)", m.UpdatedAt, changed)
				}
			})
		}
	}
}

func TestNonRecipientIsDeniedInEveryStatus(t *testing.T) {
	for _, s := range []Status{StatusQueued, StatusDelivered, StatusAcked, StatusHeld, StatusDone, StatusFailed} {
		for op, f := range recipientOps {
			for _, by := range []AgentName{"alice", "mallory"} { // even the sender
				m := messageIn(t, TypeTask, s)
				if err := f(m, by, time.Unix(500, 0)); !IsCode(err, CodePermissionDenied) {
					t.Errorf("%s/%s by %s: want permission_denied, got %v", s, op, by, err)
				}
				if m.Status != s {
					t.Errorf("%s/%s by %s changed status to %s", s, op, by, m.Status)
				}
			}
		}
	}
}

func TestFailFromEachOpenState(t *testing.T) {
	now := time.Unix(500, 0)
	for _, s := range []Status{StatusDelivered, StatusAcked, StatusHeld} {
		for _, typ := range []MessageType{TypeTask, TypeNote} {
			m := messageIn(t, typ, s)
			ev, err := m.Fail("bob", "msg_ev2", "  disk full \n", now)
			if err != nil || m.Status != StatusFailed || m.Error != "disk full" {
				t.Fatalf("%s %s: status=%s error=%q err=%v", typ, s, m.Status, m.Error, err)
			}
			if typ != TypeTask {
				if ev != nil {
					t.Errorf("%s %s: non-task failure emitted %+v", typ, s, ev)
				}
				continue
			}
			if ev == nil || ev.ID != "msg_ev2" || ev.Type != TypeEvent || ev.From != "bob" || ev.To != "alice" ||
				ev.ThreadID != m.ThreadID || ev.ReplyTo != m.ID || ev.Body != "task failed: disk full" || ev.Status != StatusQueued {
				t.Errorf("%s %s: event = %+v", typ, s, ev)
			}
		}
	}
	m := messageIn(t, TypeTask, StatusDelivered)
	if _, err := m.Fail("bob", "e", "   ", now); err != nil || m.Error != "no reason given" {
		t.Fatalf("blank reason: %q %v", m.Error, err)
	}
}

func TestReplyAndProgressMessages(t *testing.T) {
	now := time.Unix(500, 0)
	m := messageIn(t, TypeTask, StatusHeld)
	p, err := m.Progress("bob", "msg_p", "halfway", now)
	if err != nil || p.Type != TypeProgress || p.Status != StatusDone || p.To != "alice" || p.ReplyTo != m.ID || m.Status != StatusHeld {
		t.Fatalf("progress: p=%+v m=%s err=%v", p, m.Status, err)
	}
	note := messageIn(t, TypeNote, StatusDelivered)
	if _, err := note.Progress("bob", "p", "x", now); !IsCode(err, CodeInvalidArgument) {
		t.Fatalf("progress on a note: %v", err)
	}
	// An empty reply body is rejected and leaves the task open.
	if _, err := m.Reply("bob", "r", " ", now); !IsCode(err, CodeInvalidArgument) || m.Status != StatusHeld {
		t.Fatalf("empty reply: status=%s err=%v", m.Status, err)
	}
	r, err := m.Reply("bob", "msg_r", "done!", now)
	if err != nil || r.Type != TypeReply || r.Status != StatusQueued || r.From != "bob" || r.To != "alice" || m.Status != StatusDone {
		t.Fatalf("reply from held: r=%+v m=%s err=%v", r, m.Status, err)
	}
}

func TestSteer(t *testing.T) {
	now := time.Unix(500, 0)
	for _, s := range []Status{StatusQueued, StatusDelivered, StatusAcked, StatusHeld} {
		m := messageIn(t, TypeTask, s)
		st, err := m.Steer("alice", "msg_st", "use v2", now)
		if err != nil || st.Type != TypeSteer || st.From != "alice" || st.To != "bob" || st.ReplyTo != m.ID || st.ThreadID != m.ThreadID || m.Status != s {
			t.Errorf("steer %s: st=%+v err=%v", s, st, err)
		}
		if _, err := m.Steer("bob", "x", "y", now); !IsCode(err, CodePermissionDenied) {
			t.Errorf("steer %s by recipient: %v", s, err)
		}
	}
	for _, s := range []Status{StatusDone, StatusFailed} {
		if _, err := messageIn(t, TypeTask, s).Steer("alice", "x", "y", now); !IsCode(err, CodeInvalidState) {
			t.Errorf("steer %s: want invalid_state, got %v", s, err)
		}
	}
	if _, err := messageIn(t, TypeNote, StatusDelivered).Steer("alice", "x", "y", now); !IsCode(err, CodeInvalidArgument) {
		t.Errorf("steer note: %v", err)
	}
}

func TestClaimMaxAttempts(t *testing.T) {
	t0 := time.Unix(100, 0)
	m := messageIn(t, TypeTask, StatusQueued)
	for i := 1; i <= 3; i++ {
		now := t0.Add(time.Duration(i) * time.Hour)
		if !m.Claim(now, time.Minute, 3) || m.Status != StatusDelivered || m.Attempts != i || !m.LeaseUntil.Equal(now.Add(time.Minute)) {
			t.Fatalf("claim %d: %+v", i, m)
		}
	}
	now := t0.Add(10 * time.Hour)
	if m.Claim(now, time.Minute, 3) {
		t.Fatal("4th claim with maxAttempts=3 must fail")
	}
	if m.Status != StatusFailed || m.Attempts != 3 || !strings.Contains(m.Error, "exhausted") || !m.UpdatedAt.Equal(now) {
		t.Fatalf("exhausted: %+v", m)
	}
	if m.IsDue(now.Add(time.Hour)) {
		t.Fatal("failed message must never be due")
	}

	// maxAttempts <= 0 means unlimited.
	u := messageIn(t, TypeTask, StatusQueued)
	for i := range 50 {
		if !u.Claim(t0, time.Minute, 0) {
			t.Fatalf("unlimited claim %d failed", i)
		}
	}
}

func TestReleaseDoesNotCountAttempts(t *testing.T) {
	t0 := time.Unix(100, 0)
	for _, from := range []Status{StatusDelivered, StatusAcked} {
		m := messageIn(t, TypeTask, from) // one claim: Attempts == 1
		if err := m.Release("bob", 0, t0); err != nil {
			t.Fatal(err)
		}
		if m.Status != StatusDelivered || m.Attempts != 0 || !m.LeaseUntil.Equal(t0) || !m.IsDue(t0) {
			t.Fatalf("release from %s: %+v", from, m)
		}
		if err := m.Release("bob", -time.Second, t0); !IsCode(err, CodeInvalidArgument) {
			t.Fatalf("negative delay: %v", err)
		}
		if err := m.Release("bob", MaxReleaseDelay, t0); err != nil {
			t.Fatalf("max delay must be allowed: %v", err)
		}
		if m.Attempts != 0 {
			t.Fatalf("attempts must not go negative: %d", m.Attempts)
		}
	}
}

func TestIsDueByStatus(t *testing.T) {
	now := time.Unix(10, 0).Add(time.Hour) // past every lease set by messageIn
	want := map[Status]bool{
		StatusQueued: true, StatusDelivered: true, // lease expired
		StatusAcked: false, StatusHeld: false, StatusDone: false, StatusFailed: false,
	}
	for s, due := range want {
		if got := messageIn(t, TypeTask, s).IsDue(now); got != due {
			t.Errorf("IsDue(%s) = %v, want %v", s, got, due)
		}
	}
	for s, term := range map[Status]bool{StatusQueued: false, StatusDelivered: false, StatusAcked: false, StatusHeld: false, StatusDone: true, StatusFailed: true} {
		if s.Terminal() != term {
			t.Errorf("%s.Terminal() = %v", s, !term)
		}
	}
}
