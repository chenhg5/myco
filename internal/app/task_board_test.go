package app

import (
	"github.com/myco-ai/myco/internal/domain"
	"testing"
	"time"
)

func TestTaskAlertCandidatesBoundariesAndDistinctAgents(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	tasks := []domain.TaskRecord{
		{ID: "stale", Sender: "sender", Recipient: "worker", Status: domain.StatusAcked, ActiveAt: now.Add(-15 * time.Minute), UpdatedAt: now.Add(-time.Hour), StaleAfter: 15 * time.Minute, RecipientSeenAt: now},
		{ID: "alive", Sender: "sender", Status: domain.StatusAcked, ActiveAt: now.Add(-15*time.Minute + time.Millisecond), StaleAfter: 15 * time.Minute, RecipientSeenAt: now},
		{ID: "offline", Sender: "sender", Status: domain.StatusAcked, ActiveAt: now, StaleAfter: 15 * time.Minute, RecipientSeenAt: now.Add(-time.Minute - time.Millisecond)},
		{ID: "held", Sender: "sender", Status: domain.StatusHeld, ActiveAt: now, StaleAfter: 15 * time.Minute, RecipientSeenAt: now.Add(-time.Hour)},
		{ID: "never-seen", Sender: "sender", Status: domain.StatusAcked, ActiveAt: now, StaleAfter: 15 * time.Minute},
	}
	alerts := taskAlertCandidates(tasks, nil, now, time.Minute)
	if len(alerts) != 2 || alerts[0].Kind != "stale" || alerts[1].Kind != "offline" {
		t.Fatalf("activity/offline boundaries: %+v", alerts)
	}
	fs := []domain.TaskFailure{{ID: "1", Sender: "sender", Recipient: "a", Machine: "box", MachineOwner: "owner", Incident: 1, At: now.Add(-TaskFailureWindow)}, {ID: "2", Sender: "sender", Recipient: "a", Machine: "box", MachineOwner: "owner", Incident: 1, At: now}, {ID: "3", Sender: "other", Recipient: "b", Machine: "box", MachineOwner: "owner", Incident: 1, At: now}}
	if got := taskAlertCandidates(nil, fs, now, time.Minute); len(got) != 0 {
		t.Fatal("three failures of only two agents alerted", got)
	}
	fs = append(fs, domain.TaskFailure{ID: "4", Sender: "sender", Recipient: "c", Machine: "box", MachineOwner: "owner", Incident: 1, At: now})
	got := taskAlertCandidates(nil, fs, now, time.Minute)
	if len(got) != 2 || got[0].Agents != 3 || got[1].Agents != 3 {
		t.Fatal("one incident per affected sender", got)
	}
	fs[1].At = now.Add(-TaskFailureWindow)
	if got = taskAlertCandidates(nil, fs, now.Add(time.Millisecond), time.Minute); len(got) != 0 {
		t.Fatal("old failure outside window counted", got)
	}
	fs[0].At = now
	fs[3].Incident = 2
	if got = taskAlertCandidates(nil, fs, now, time.Minute); len(got) != 0 {
		t.Fatal("separate episodes combined", got)
	}
}
