package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/app"
)

// A runner yields to a session that keeps `inbox wait` running, and takes
// over again after the grace period.
func TestRunnerYieldsToLiveSession(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	p := app.NewPresence(app.SystemClock{}, 150*time.Millisecond)
	f.messages.SetPresence(p)
	alice, bob := f.agent(t, "alice"), f.agent(t, "bob")

	if r, _ := p.State(bob.Name, time.Minute); r != "" {
		t.Fatalf("nothing pulled yet, got %q", r)
	}
	// Bob's session starts waiting; a message arrives while it waits.
	got := make(chan int, 1)
	go func() {
		ms, _ := f.messages.WaitPending(ctx, bob, 2*time.Second)
		got <- len(ms)
	}()
	time.Sleep(30 * time.Millisecond)
	if r, _ := p.State(bob.Name, time.Minute); r != app.ReceivingSession {
		t.Fatalf("receiving = %q, want session", r)
	}
	if _, err := f.messages.Send(ctx, alice, app.SendInput{To: "bob", Body: "hi"}); err != nil {
		t.Fatal(err)
	}
	if n := <-got; n != 1 {
		t.Fatalf("wait saw %d messages", n)
	}
	// Within the grace period the runner gets nothing.
	ms, err := f.messages.PullForRunner(ctx, bob, 10, 20*time.Millisecond)
	if err != nil || len(ms) != 0 {
		t.Fatalf("runner pull while live: %v %v", ms, err)
	}
	time.Sleep(200 * time.Millisecond)
	ms, err = f.messages.PullForRunner(ctx, bob, 10, 0)
	if err != nil || len(ms) != 1 {
		t.Fatalf("runner pull after grace: %v %v", ms, err)
	}
	if r, inUse := p.State(bob.Name, time.Minute); r != app.ReceivingRunner || inUse {
		t.Fatalf("state = %q %v", r, inUse)
	}
	p.SessionInUse(bob.Name)
	if _, inUse := p.State(bob.Name, time.Minute); !inUse {
		t.Fatal("in-use report not kept")
	}
}
