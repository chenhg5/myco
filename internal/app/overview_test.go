package app

import (
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/domain"
)

func TestStateOfOnlyAckedIsWorking(t *testing.T) {
	now := time.Now()
	s := &OverviewService{onlineWindow: time.Minute}
	a := &domain.Agent{}
	tests := []struct {
		name                   string
		counts                 map[domain.Status]int
		state                  AgentState
		queued, progress, held int
	}{
		{"queued", map[domain.Status]int{domain.StatusQueued: 1}, StatePending, 1, 0, 0},
		{"pulled but not acked", map[domain.Status]int{domain.StatusDelivered: 1}, StatePending, 1, 0, 0},
		{"acked", map[domain.Status]int{domain.StatusAcked: 1}, StateWorking, 0, 1, 0},
		{"held", map[domain.Status]int{domain.StatusHeld: 1}, StatePending, 0, 0, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.stateOf(a, tt.counts, now)
			if got.State != tt.state || got.Queued != tt.queued || got.InProgress != tt.progress || got.Held != tt.held {
				t.Fatalf("state=%s queued=%d in_progress=%d held=%d", got.State, got.Queued, got.InProgress, got.Held)
			}
		})
	}
}

func TestTaskStateOnlyAckedIsWorking(t *testing.T) {
	for _, tc := range []struct {
		status domain.Status
		want   string
	}{
		{domain.StatusQueued, "queued"},
		{domain.StatusDelivered, "queued"},
		{domain.StatusAcked, "working"},
		{domain.StatusHeld, "queued"},
	} {
		if got := taskState(nil, nil, &domain.Message{Status: tc.status}); got != tc.want {
			t.Errorf("%s: got %s, want %s", tc.status, got, tc.want)
		}
	}
}
