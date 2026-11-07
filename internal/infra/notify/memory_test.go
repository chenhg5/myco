package notify

import (
	"sync"
	"testing"
	"time"
)

func fired(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func waitFired(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("subscriber not woken")
	}
}

func topicCount(m *Memory) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.topics)
}

func TestPublishWakesAllSubscribersOfTopicOnly(t *testing.T) {
	m := NewMemory()
	a, cancelA := m.Subscribe("inbox:alice")
	defer cancelA()
	a2, cancelA2 := m.Subscribe("inbox:alice")
	defer cancelA2()
	b, cancelB := m.Subscribe("inbox:bob")
	defer cancelB()

	if fired(a) || fired(a2) || fired(b) {
		t.Fatal("no publish yet")
	}
	m.Publish("inbox:alice")
	waitFired(t, a)
	waitFired(t, a2)
	if fired(b) {
		t.Fatal("publish on alice woke bob")
	}
	// The channel is closed, so it stays readable.
	if !fired(a) {
		t.Fatal("woken channel should stay closed")
	}
	// The topic is consumed: a later subscriber needs a later publish.
	c, cancelC := m.Subscribe("inbox:alice")
	defer cancelC()
	if fired(c) {
		t.Fatal("subscriber after publish woken by the earlier publish")
	}
	m.Publish("inbox:alice") // a, a2 already closed: must not panic
	waitFired(t, c)
}

func TestPublishWithoutSubscribers(t *testing.T) {
	m := NewMemory()
	m.Publish("nobody")
	m.Publish("nobody")
	if n := topicCount(m); n != 0 {
		t.Fatalf("publish without subscribers left %d topics", n)
	}
	// A publish before subscribing is not remembered.
	ch, cancel := m.Subscribe("nobody")
	defer cancel()
	if fired(ch) {
		t.Fatal("earlier publish must not wake a later subscriber")
	}
}

func TestCancelStopsDeliveryAndFreesTopic(t *testing.T) {
	m := NewMemory()
	ch, cancel := m.Subscribe("t")
	keep, cancelKeep := m.Subscribe("t")
	cancel()
	cancel() // idempotent
	if n := topicCount(m); n != 1 {
		t.Fatalf("topics = %d, want 1 while one subscriber remains", n)
	}
	m.Publish("t")
	waitFired(t, keep)
	if fired(ch) {
		t.Fatal("cancelled subscriber was woken")
	}
	cancelKeep() // cancel after publish: no panic
	if n := topicCount(m); n != 0 {
		t.Fatalf("topics leaked: %d", n)
	}

	// Cancelling an old subscription must not remove a newer one on the
	// same topic.
	_, oldCancel := m.Subscribe("t")
	m.Publish("t")
	fresh, freshCancel := m.Subscribe("t")
	defer freshCancel()
	oldCancel()
	m.Publish("t")
	waitFired(t, fresh)
}

func TestSubscribeCancelLeavesNoState(t *testing.T) {
	m := NewMemory()
	for i := range 1000 {
		_, cancel := m.Subscribe(string(rune('a' + i%26)))
		cancel()
	}
	if n := topicCount(m); n != 0 {
		t.Fatalf("topics leaked after cancel: %d", n)
	}
}

func TestConcurrentUse(t *testing.T) {
	m := NewMemory()
	topics := []string{"a", "b", "c"}
	stop := make(chan struct{})
	var pubs sync.WaitGroup
	for _, tp := range topics {
		pubs.Add(1)
		go func() {
			defer pubs.Done()
			for {
				select {
				case <-stop:
					return
				default:
					m.Publish(tp)
				}
			}
		}()
	}
	var subs sync.WaitGroup
	for i := range 16 {
		subs.Add(1)
		go func() {
			defer subs.Done()
			tp := topics[i%len(topics)]
			for j := range 200 {
				ch, cancel := m.Subscribe(tp)
				if j%2 == 0 {
					select { // publishers are spinning: expect a wake-up
					case <-ch:
					case <-time.After(2 * time.Second):
						t.Error("no wake-up under concurrent publish")
					}
				}
				cancel()
			}
		}()
	}
	subs.Wait()
	close(stop)
	pubs.Wait()
	if n := topicCount(m); n != 0 {
		t.Fatalf("topics leaked: %d", n)
	}
}
