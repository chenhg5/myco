package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/domain"
)

type overviewWaker struct{}

func (overviewWaker) Mode() string { return "test_push" }
func (overviewWaker) Validate(a *domain.Agent) error {
	if a.Delivery.URL == "" {
		return errors.New("missing endpoint")
	}
	return nil
}
func (overviewWaker) Wake(context.Context, WakeRequest) error {
	panic("overview must never wake a node")
}
func (overviewWaker) Consumes() bool { return false }

func TestStandbyRequiresRecentValidatedPush(t *testing.T) {
	now := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, mode, url     string
		age                 time.Duration
		never, unregistered bool
		want                AgentState
	}{
		{name: "configured", mode: "test_push", url: "https://example.com/wake", age: time.Hour, want: StateStandby},
		{name: "seven day boundary", mode: "test_push", url: "https://example.com/wake", age: StandbyWindow, want: StateStandby},
		{name: "expired", mode: "test_push", url: "https://example.com/wake", age: StandbyWindow + time.Second, want: StateOffline},
		{name: "never seen", mode: "test_push", url: "https://example.com/wake", never: true, want: StateOffline},
		{name: "invalid settings", mode: "test_push", age: time.Hour, want: StateOffline},
		{name: "unsupported", mode: "unknown", url: "https://example.com/wake", age: time.Hour, want: StateOffline},
		{name: "no registry", mode: "test_push", url: "https://example.com/wake", age: time.Hour, unregistered: true, want: StateOffline},
		{name: "cloud pull", mode: "pull", age: time.Hour, want: StateOffline},
		{name: "default pull", age: time.Hour, want: StateOffline},
		{name: "online", mode: "test_push", url: "https://example.com/wake", age: time.Second, want: StateIdle},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &OverviewService{onlineWindow: 90 * time.Second}
			if !tc.unregistered {
				s.SetWakers(NewWakers(overviewWaker{}))
			}
			a := &domain.Agent{LastSeenAt: now.Add(-tc.age), Delivery: domain.Delivery{Mode: tc.mode, URL: tc.url}, Binding: domain.Binding{Harness: "claude-cloud"}}
			if tc.never {
				a.LastSeenAt = time.Time{}
			}
			if got := s.stateOf(a, nil, nil, now); got.State != tc.want {
				t.Fatalf("got %s, want %s", got.State, tc.want)
			}
		})
	}
}

func TestStandbyKeepsQueuedCounters(t *testing.T) {
	now := time.Now()
	s := &OverviewService{onlineWindow: 90 * time.Second, wakers: NewWakers(overviewWaker{})}
	a := &domain.Agent{LastSeenAt: now.Add(-time.Hour), Delivery: domain.Delivery{Mode: "test_push", URL: "https://example.com/wake"}}
	got := s.stateOf(a, map[domain.Status]int{domain.StatusQueued: 2, domain.StatusDelivered: 1, domain.StatusHeld: 1}, nil, now)
	if got.State != StateStandby || got.Online || got.Queued != 3 || got.Held != 1 {
		t.Fatalf("unexpected overview: %+v", got)
	}
	a.Delivery.URL = ""
	if got := s.stateOf(a, map[domain.Status]int{domain.StatusQueued: 2}, nil, now); got.State != StateOffline {
		t.Fatalf("invalid push must stay offline: %s", got.State)
	}
}
