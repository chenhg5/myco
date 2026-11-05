package app_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/domain"
	"github.com/chenhg5/myco/internal/infra/notify"
	"github.com/chenhg5/myco/internal/infra/security"
	"github.com/chenhg5/myco/internal/infra/sqlite"
)

type fakeWaker struct {
	mu       sync.Mutex
	wakes    []app.WakeRequest
	consumes bool
	fail     bool
}

func (f *fakeWaker) Mode() string   { return "fake" }
func (f *fakeWaker) Consumes() bool { return f.consumes }
func (f *fakeWaker) Validate(a *domain.Agent) error {
	if a.Binding.SessionID == "" {
		return domain.Errf(domain.CodeInvalidArgument, "fake needs a session")
	}
	return nil
}
func (f *fakeWaker) Wake(_ context.Context, r app.WakeRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.wakes = append(f.wakes, r)
	if f.fail {
		return errors.New("boom")
	}
	return nil
}
func (f *fakeWaker) count() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.wakes) }

func setupPush(t *testing.T, w *fakeWaker) (*app.AgentService, *app.MessageService, *app.Dispatcher) {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ar, mr := sqlite.NewAgentRepo(db), sqlite.NewMessageRepo(db)
	wakers := app.NewWakers(w)
	clock := app.SystemClock{}
	agents := app.NewAgentService(ar, security.Tokens{}, security.IDs{}, clock, time.Minute, wakers)
	msgs := app.NewMessageService(ar, mr, notify.NewMemory(), security.IDs{}, clock, app.DefaultMessagePolicy())
	d := app.NewDispatcher(ar, mr, wakers, clock, app.DispatchPolicy{Cooldown: time.Hour, SweepInterval: 20 * time.Millisecond},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	msgs.SetObserver(d)
	return agents, msgs, d
}

func TestRegistrationValidatesDelivery(t *testing.T) {
	agents, _, _ := setupPush(t, &fakeWaker{})
	ctx := context.Background()
	if _, err := agents.Register(ctx, app.RegisterInput{Name: "x", Delivery: domain.Delivery{Mode: "nope"}}); !domain.IsCode(err, domain.CodeInvalidArgument) {
		t.Fatalf("unknown mode: %v", err)
	}
	if _, err := agents.Register(ctx, app.RegisterInput{Name: "x", Delivery: domain.Delivery{Mode: "fake"}}); !domain.IsCode(err, domain.CodeInvalidArgument) {
		t.Fatalf("waker validation should run: %v", err)
	}
	out, err := agents.Register(ctx, app.RegisterInput{Name: "x", Delivery: domain.Delivery{Mode: "fake", Secret: "s"},
		Binding: domain.Binding{Harness: "claude", SessionID: "sess-1", Environment: "vps1"}})
	if err != nil || out.Agent.Binding.SessionID != "sess-1" {
		t.Fatalf("register: %+v %v", out, err)
	}
	// Bind keeps unspecified fields and persists.
	a, err := agents.Authenticate(ctx, out.Token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := agents.Bind(ctx, a, domain.Binding{SessionID: "sess-2"}); err != nil {
		t.Fatal(err)
	}
	got, _ := agents.Get(ctx, "x")
	if got.Binding.SessionID != "sess-2" || got.Binding.Environment != "vps1" || got.Delivery.Secret != "s" {
		t.Fatalf("bind/persist: %+v", got)
	}
}

func TestDispatcherWakesCoalescesAndConsumes(t *testing.T) {
	w := &fakeWaker{consumes: true}
	agents, msgs, d := setupPush(t, w)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	reg := func(name string, delivery domain.Delivery, b domain.Binding) *domain.Agent {
		out, err := agents.Register(ctx, app.RegisterInput{Name: name, Delivery: delivery, Binding: b})
		if err != nil {
			t.Fatal(err)
		}
		a, _ := agents.Authenticate(ctx, out.Token)
		return a
	}
	alice := reg("alice", domain.Delivery{}, domain.Binding{})
	reg("worker", domain.Delivery{Mode: "fake"}, domain.Binding{SessionID: "s1"})

	for i := range 3 {
		if _, err := msgs.Send(ctx, alice, app.SendInput{To: "worker", Body: "job " + string(rune('a'+i))}); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(2 * time.Second)
	for w.count() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond) // let sweeps run; cooldown must coalesce
	if n := w.count(); n != 1 {
		t.Fatalf("want exactly 1 coalesced wake, got %d", n)
	}
	first := w.wakes[0]
	if len(first.Pending) == 0 || first.Agent.Binding.SessionID != "s1" || first.Text == "" {
		t.Fatalf("wake request: %+v", first)
	}
	// Consuming waker: handed-off messages are acked, so not due anymore.
	time.Sleep(50 * time.Millisecond)
	due, err := msgs.WaitPending(ctx, &domain.Agent{Name: "worker"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) > 3-len(first.Pending) {
		t.Fatalf("handed-off messages should not be pending: %d left", len(due))
	}
	// Ingress-style reply on behalf of the worker reaches alice.
	r, err := msgs.ReplyOnBehalf(ctx, first.Pending[0].ID, "done via github", "github:bot")
	if err != nil || r.To != "alice" {
		t.Fatalf("reply on behalf: %+v %v", r, err)
	}
}

func TestPullAgentsAreNeverWoken(t *testing.T) {
	w := &fakeWaker{}
	agents, msgs, d := setupPush(t, w)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)
	a1, _ := agents.Register(ctx, app.RegisterInput{Name: "a"})
	agents.Register(ctx, app.RegisterInput{Name: "b"})
	a, _ := agents.Authenticate(ctx, a1.Token)
	msgs.Send(ctx, a, app.SendInput{To: "b", Body: "hi"})
	time.Sleep(100 * time.Millisecond)
	if w.count() != 0 {
		t.Fatal("pull agents must not be woken")
	}
}
