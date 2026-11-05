package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/domain"
	"github.com/chenhg5/myco/internal/infra/notify"
	"github.com/chenhg5/myco/internal/infra/security"
	"github.com/chenhg5/myco/internal/infra/sqlite"
)

type fixture struct {
	agents   *app.AgentService
	messages *app.MessageService
}

func setup(t *testing.T) *fixture {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ar, mr := sqlite.NewAgentRepo(db), sqlite.NewMessageRepo(db)
	clock := app.SystemClock{}
	p := app.DefaultMessagePolicy()
	p.Lease = 50 * time.Millisecond
	p.MaxAttempts = 3
	return &fixture{
		agents:   app.NewAgentService(ar, security.Tokens{}, security.IDs{}, clock, time.Minute, nil),
		messages: app.NewMessageService(ar, mr, notify.NewMemory(), security.IDs{}, clock, p),
	}
}

func (f *fixture) agent(t *testing.T, name string, allow ...string) *domain.Agent {
	t.Helper()
	ctx := context.Background()
	out, err := f.agents.Register(ctx, app.RegisterInput{Name: name, AllowFrom: allow})
	if err != nil {
		t.Fatal(err)
	}
	a, err := f.agents.Authenticate(ctx, out.Token)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestSendPullReplyWait(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	alice, bob := f.agent(t, "alice"), f.agent(t, "bob")

	sent, err := f.messages.Send(ctx, alice, app.SendInput{To: "bob", Body: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	// Bob long-polls; the message is already there.
	msgs, err := f.messages.Pull(ctx, bob, 10, time.Second)
	if err != nil || len(msgs) != 1 {
		t.Fatalf("pull: %v %v", msgs, err)
	}
	go func() {
		time.Sleep(50 * time.Millisecond)
		if _, err := f.messages.Reply(ctx, bob, msgs[0].ID, "world", false); err != nil {
			t.Error(err)
		}
	}()
	reply, err := f.messages.WaitReply(ctx, alice, sent.Message.ID, 2*time.Second)
	if err != nil || reply == nil || reply.Body != "world" || reply.Status != domain.StatusDone {
		t.Fatalf("wait reply: %+v %v", reply, err)
	}
	// The consumed reply is not redelivered to Alice's inbox.
	again, _ := f.messages.Pull(ctx, alice, 10, 0)
	if len(again) != 0 {
		t.Fatalf("reply should be consumed by wait, got %+v", again)
	}
}

func TestLongPollWakesOnSend(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	alice, bob := f.agent(t, "alice"), f.agent(t, "bob")
	done := make(chan []*domain.Message)
	go func() {
		m, _ := f.messages.Pull(ctx, bob, 10, 5*time.Second)
		done <- m
	}()
	time.Sleep(50 * time.Millisecond)
	start := time.Now()
	if _, err := f.messages.Send(ctx, alice, app.SendInput{To: "bob", Body: "wake"}); err != nil {
		t.Fatal(err)
	}
	msgs := <-done
	if len(msgs) != 1 || time.Since(start) > time.Second {
		t.Fatalf("long-poll not woken promptly: %v after %s", msgs, time.Since(start))
	}
}

func TestPolicyIdempotencyAndRedelivery(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	alice, bob, eve := f.agent(t, "alice"), f.agent(t, "bob", "alice"), f.agent(t, "eve")

	if _, err := f.messages.Send(ctx, eve, app.SendInput{To: "bob", Body: "hi"}); !domain.IsCode(err, domain.CodePermissionDenied) {
		t.Fatalf("allow-from: want permission_denied, got %v", err)
	}
	a, err := f.messages.Send(ctx, alice, app.SendInput{To: "bob", Body: "x", IdempotencyKey: "k1"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := f.messages.Send(ctx, alice, app.SendInput{To: "bob", Body: "x", IdempotencyKey: "k1"})
	if err != nil || !b.Duplicate || b.Message.ID != a.Message.ID {
		t.Fatalf("idempotency: %+v %v", b, err)
	}
	// Pull without ack: redelivered after the lease expires.
	first, _ := f.messages.Pull(ctx, bob, 10, 0)
	time.Sleep(80 * time.Millisecond)
	second, _ := f.messages.Pull(ctx, bob, 10, 0)
	if len(first) != 1 || len(second) != 1 || second[0].Attempts != 2 {
		t.Fatalf("redelivery: first=%v second=%v", first, second)
	}
	// Dry-run send does not persist.
	if _, err := f.messages.Send(ctx, alice, app.SendInput{To: "bob", Body: "dry", DryRun: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.messages.Transition(ctx, bob, second[0].ID, app.TransitionAck, false); err != nil {
		t.Fatal(err)
	}
	time.Sleep(80 * time.Millisecond)
	if left, _ := f.messages.Pull(ctx, bob, 10, 0); len(left) != 0 {
		t.Fatalf("acked/dry-run messages must not be delivered: %+v", left)
	}
}

func TestThreadAccessAndRegistration(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	alice, _, eve := f.agent(t, "alice"), f.agent(t, "bob"), f.agent(t, "eve")
	s, _ := f.messages.Send(ctx, alice, app.SendInput{To: "bob", Body: "x"})
	if _, err := f.messages.Thread(ctx, eve, s.Message.ThreadID); !domain.IsCode(err, domain.CodeNotFound) {
		t.Fatalf("outsider must not read thread: %v", err)
	}
	if _, err := f.messages.Send(ctx, eve, app.SendInput{To: "alice", Body: "x", ThreadID: s.Message.ThreadID}); !domain.IsCode(err, domain.CodeNotFound) {
		t.Fatalf("outsider must not join thread: %v", err)
	}
	if _, err := f.agents.Register(ctx, app.RegisterInput{Name: "alice"}); !domain.IsCode(err, domain.CodeConflict) {
		t.Fatalf("duplicate register: want conflict, got %v", err)
	}
	out, err := f.agents.Register(ctx, app.RegisterInput{Name: "alice", RotateToken: true, Description: "new"})
	if err != nil || out.Token == "" || out.Agent.Description != "new" {
		t.Fatalf("rotate: %+v %v", out, err)
	}
	if _, err := f.agents.Authenticate(ctx, "myco_wrong"); !domain.IsCode(err, domain.CodeUnauthenticated) {
		t.Fatalf("bad token: %v", err)
	}
}
