package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/domain"
)

type timingFixture []domain.TaskTiming

func (f timingFixture) TaskTimings(context.Context, domain.CoordinationFilter) ([]domain.TaskTiming, error) {
	return f, nil
}
func TestCoordinationStatistics(t *testing.T) {
	now := time.Unix(1800000000, 0)
	f := domain.CoordinationFilter{Since: now.Add(-time.Hour), Until: now}
	tasks := timingFixture{
		{ID: "one", Thread: "one", From: "@console:demo", To: "coordinator", Created: now.Add(-100 * time.Second), Started: now.Add(-90 * time.Second), Replied: now, Done: true, Followups: 2},
		{ID: "two", Thread: "two", From: "coordinator", To: "worker", Created: now.Add(-100 * time.Second), Started: now.Add(-80 * time.Second), Replied: now, Done: true},
		{ID: "three", Thread: "three", From: "coordinator", To: "worker", Created: now.Add(-50 * time.Second), Updated: now, Stopped: true, Reassigned: true, Redone: true},
		{ID: "four", Thread: "four", From: "coordinator", To: "worker", Created: now.Add(-50 * time.Second), Updated: now, Stopped: true},
		{ID: "five", Thread: "five", From: "coordinator", To: "worker", Created: now.Add(-50 * time.Second), Updated: now, Failed: true},
	}
	svc := NewCoordinationService(tasks)
	out, err := svc.Agents(context.Background(), f, []*domain.Agent{{Name: "worker", Capabilities: []string{"code"}, Labels: []string{"code", "test"}}})
	if err != nil {
		t.Fatal(err)
	}
	s := out.Agents[0]
	if s.Tasks != 4 || s.Done != 1 || s.Failed != 1 || s.Stopped != 2 || s.Redone != 1 || *s.SuccessRate != .25 || *s.RedoneShare != .25 || s.QueueWait.Samples != 1 || *s.QueueWait.MedianSeconds != 20 || *s.ReceiptToReply.MedianSeconds != 100 || len(s.CommonTaskTypes) != 2 {
		t.Fatalf("wrong stats: %+v", s)
	}
	c, err := svc.Coordinator(context.Background(), f, "coordinator")
	if err != nil {
		t.Fatal(err)
	}
	if c.PersonTasks != 1 || c.PersonFollowups != 2 || c.DelegatedTasks != 4 || c.Reassigned != 1 || c.Withdrawn != 1 || c.QueueSamples != 1 || *c.DelegatedQueuedShare != .2 {
		t.Fatalf("wrong coordinator stats: %+v", c)
	}
	missing := SummarizeTimings(nil)
	if missing.MedianSeconds != nil || missing.P90Seconds != nil {
		t.Fatal("missing is not zero")
	}
	summary := SummarizeTimings([]float64{30, 10, 20, 40})
	if *summary.MedianSeconds != 25 || *summary.P90Seconds != 40 {
		t.Fatalf("quantiles: %+v", summary)
	}
	for _, raw := range []string{"0d", "-1d", "367d", "10000h", "garbage"} {
		if _, err := StatsSince(raw, now); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	since, err := StatsSince("7d", now)
	if err != nil || now.Sub(since) != 7*24*time.Hour {
		t.Fatal(since, err)
	}
}
func TestAgentPickAvailabilityAndEvidence(t *testing.T) {
	now := time.Now()
	slots, _ := json.Marshal(api.RunnerSlots{MaxConcurrent: 4, InUse: 2, At: now})
	agents := []*domain.Agent{
		{Name: "idle", Capabilities: []string{"code"}, Binding: domain.Binding{Harness: "codex", Environment: "demo-mac"}, Runtime: domain.Runtime{RunnerSlots: string(slots)}},
		{Name: "busy", Capabilities: []string{"code"}, Binding: domain.Binding{Harness: "codex"}},
		{Name: "offline", Capabilities: []string{"code"}},
		{Name: "limited", Capabilities: []string{"code"}, Runtime: domain.Runtime{Quota: &domain.Quota{At: now, Windows: []domain.QuotaWindow{{Used: 1}}}}},
		{Name: "full", Capabilities: []string{"code"}},
	}
	states := map[domain.AgentName]AgentOverview{"idle": {Online: true}, "busy": {Online: true, InProgress: 1}, "limited": {Online: true}, "full": {Online: true, Queued: 2}}
	out := RankAgents(agents, states, &api.AgentStatsList{}, PickFilter{Skill: "code"}, now)
	if len(out.Candidates) != 5 || out.Candidates[0].Name != "idle" || !out.Candidates[1].Available || out.Candidates[2].Available || *out.Candidates[0].RunnerSlotsFree != 2 {
		t.Fatalf("rank: %+v", out)
	}
	out = RankAgents(agents, states, &api.AgentStatsList{}, PickFilter{Strict: true, CLI: "codex", Machine: "demo-mac"}, now)
	if len(out.Candidates) != 1 || out.Candidates[0].Name != "idle" {
		t.Fatal(out)
	}
	for _, status := range []string{"unsupported", "schema_changed", "transient_error"} {
		if quotaAvailability(&domain.Quota{At: now, Status: status}, now) != "unknown" {
			t.Fatalf("%s incorrectly blocked", status)
		}
	}
	if quotaAvailability(&domain.Quota{At: now, Windows: []domain.QuotaWindow{{Status: "allowed_warning", Used: .8}}}, now) != "available" {
		t.Fatal("warning blocked")
	}
	past := now.Add(-time.Second)
	if quotaAvailability(&domain.Quota{At: now, Windows: []domain.QuotaWindow{{Used: 1, ResetsAt: &past}}}, now) != "available" {
		t.Fatal("expired limit blocked")
	}
}
