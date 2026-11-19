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
			got := s.stateOf(a, tt.counts, nil, now)
			if got.State != tt.state || got.Queued != tt.queued || got.InProgress != tt.progress || got.Held != tt.held {
				t.Fatalf("state=%s queued=%d in_progress=%d held=%d", got.State, got.Queued, got.InProgress, got.Held)
			}
		})
	}
}

// An acked task makes its agent "working" only while it shows life: the
// agent is online and the task was touched within 30 minutes (a session)
// or 5 (a runner, which reads its task every few seconds). Otherwise it
// is stale: still open, counted apart, and the agent is idle or offline.
func TestStateOfStaleTasks(t *testing.T) {
	now := time.Now()
	clock := fixedClock{now}
	p := NewPresence(clock, 30*time.Minute)
	s := &OverviewService{onlineWindow: time.Minute, presence: p}
	seen := now.Add(-10 * time.Second)
	online := &domain.Agent{Name: "sess", LastSeenAt: seen}
	away := &domain.Agent{Name: "gone", LastSeenAt: now.Add(-time.Hour)}
	runner := &domain.Agent{Name: "run", LastSeenAt: seen}
	p.RunnerPulled(runner.Name)
	acked := map[domain.Status]int{domain.StatusAcked: 2}
	acts := func(ages ...time.Duration) []domain.Activity {
		var out []domain.Activity
		for i, d := range ages {
			out = append(out, domain.Activity{ID: string(rune('a' + i)), At: now.Add(-d)})
		}
		return out
	}
	for _, tc := range []struct {
		name        string
		a           *domain.Agent
		acts        []domain.Activity
		state       AgentState
		live, stale int
	}{
		{"session, both lively", online, acts(time.Minute, 20*time.Minute), StateWorking, 2, 0},
		{"session, one left open", online, acts(time.Minute, 2*time.Hour), StateWorking, 1, 1},
		{"session, all left open", online, acts(31*time.Minute, 7*time.Hour), StateIdle, 0, 2},
		{"away", away, acts(time.Minute, time.Minute), StateOffline, 0, 2},
		{"runner, quiet for 6 minutes", runner, acts(30*time.Second, 6*time.Minute), StateWorking, 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := s.stateOf(tc.a, acked, tc.acts, now)
			if got.State != tc.state || got.InProgress != tc.live || got.Stale != tc.stale || len(got.StaleIDs) != tc.stale {
				t.Fatalf("state=%s in_progress=%d stale=%d %v", got.State, got.InProgress, got.Stale, got.StaleIDs)
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

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }
