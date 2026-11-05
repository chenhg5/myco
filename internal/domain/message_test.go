package domain

import (
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
