package app

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/domain"
)

// CoordinationService computes content-free aggregates on the Hub.
type CoordinationService struct {
	repo domain.CoordinationRepository
}

func NewCoordinationService(repo domain.CoordinationRepository) *CoordinationService {
	return &CoordinationService{repo}
}

// StatsSince accepts a positive Go duration (also days), capped at 366 days.
func StatsSince(raw string, now time.Time) (time.Time, error) {
	if raw == "" {
		raw = "7d"
	}
	if strings.HasSuffix(raw, "d") {
		raw = strings.TrimSuffix(raw, "d") + "h"
		d, e := time.ParseDuration(raw)
		if e == nil && d > 0 && d <= 366*time.Hour {
			return now.Add(-24 * d), nil
		}
		return time.Time{}, domain.Errf(domain.CodeInvalidArgument, "since must be a duration between 0 and 366d").WithSuggestion("use --since 7d or --since 24h")
	}
	d, e := time.ParseDuration(raw)
	if e != nil || d <= 0 || d > 366*24*time.Hour {
		return time.Time{}, domain.Errf(domain.CodeInvalidArgument, "since must be a duration between 0 and 366d").WithSuggestion("use --since 7d or --since 24h")
	}
	return now.Add(-d), nil
}
func SummarizeTimings(values []float64) api.TimingSummary {
	out := api.TimingSummary{Samples: len(values)}
	if len(values) == 0 {
		return out
	}
	slices.Sort(values)
	n := len(values)
	median := values[n/2]
	if n%2 == 0 {
		median = (median + values[n/2-1]) / 2
	}
	p90 := values[int(math.Ceil(.9*float64(n)))-1]
	out.MedianSeconds = &median
	out.P90Seconds = &p90
	return out
}
func statsRatio(n, d int) *float64 {
	if d == 0 {
		return nil
	}
	v := float64(n) / float64(d)
	return &v
}
func StatsScope(f domain.CoordinationFilter) string {
	if f.Names == nil {
		return "all"
	}
	return "participant_messages"
}
func (s *CoordinationService) Agents(ctx context.Context, f domain.CoordinationFilter, agents []*domain.Agent) (*api.AgentStatsList, error) {
	tasks, err := s.repo.TaskTimings(ctx, f)
	if err != nil {
		return nil, err
	}
	out := &api.AgentStatsList{Version: 1, Since: f.Since, Until: f.Until, Scope: StatsScope(f), Agents: []api.AgentTaskStats{}}
	byAgent := map[domain.AgentName][]domain.TaskTiming{}
	for _, t := range tasks {
		byAgent[t.To] = append(byAgent[t.To], t)
	}
	for _, a := range agents {
		st := api.AgentTaskStats{Name: string(a.Name), CommonTaskTypes: []string{}}
		var elapsed, queue []float64
		for _, t := range byAgent[a.Name] {
			st.Tasks++
			if t.Done {
				st.Done++
			}
			if t.Failed {
				st.Failed++
			}
			if t.Stopped {
				st.Stopped++
			}
			if t.Redone {
				st.Redone++
			}
			if !t.Replied.IsZero() && !t.Replied.Before(t.Created) {
				elapsed = append(elapsed, t.Replied.Sub(t.Created).Seconds())
			}
			if !t.Started.IsZero() && !t.Started.Before(t.Created) {
				queue = append(queue, t.Started.Sub(t.Created).Seconds())
			}
		}
		st.SuccessRate = statsRatio(st.Done, st.Done+st.Failed+st.Stopped)
		st.RedoneShare = statsRatio(st.Redone, st.Tasks)
		st.ReceiptToReply = SummarizeTimings(elapsed)
		st.QueueWait = SummarizeTimings(queue)
		tags := append(slices.Clone(a.Capabilities), a.Labels...)
		for _, skill := range a.Skills {
			tags = append(tags, skill.Name)
		}
		for _, tag := range tags {
			if !slices.Contains(st.CommonTaskTypes, tag) {
				st.CommonTaskTypes = append(st.CommonTaskTypes, tag)
			}
		}
		slices.Sort(st.CommonTaskTypes)
		out.Agents = append(out.Agents, st)
	}
	return out, nil
}
func (s *CoordinationService) Coordinator(ctx context.Context, f domain.CoordinationFilter, name string) (*api.CoordinatorStats, error) {
	tasks, err := s.repo.TaskTimings(ctx, f)
	if err != nil {
		return nil, err
	}
	out := &api.CoordinatorStats{Version: 1, Name: name, Since: f.Since, Until: f.Until, Scope: StatsScope(f), ReassignmentMethod: "stopped_then_sent_to_another_agent_in_same_thread"}
	var elapsed []float64
	var queued, total float64
	seen := map[string]bool{}
	for _, t := range tasks {
		if string(t.To) == name && IsConsoleSender(t.From) && !seen[t.Thread] {
			seen[t.Thread] = true
			out.PersonTasks++
			out.PersonFollowups += t.Followups
			if !t.Replied.IsZero() {
				elapsed = append(elapsed, t.Replied.Sub(t.Created).Seconds())
			}
		}
		if string(t.From) != name {
			continue
		}
		out.DelegatedTasks++
		if t.Stopped {
			if t.Reassigned {
				out.Reassigned++
			} else {
				out.Withdrawn++
			}
		}
		if t.Started.IsZero() {
			continue
		}
		end := t.Replied
		if end.IsZero() && (t.Done || t.Failed || t.Stopped) {
			end = t.Updated
		}
		if end.IsZero() || end.Before(t.Started) {
			continue
		}
		out.QueueSamples++
		queued += t.Started.Sub(t.Created).Seconds()
		total += end.Sub(t.Created).Seconds()
	}
	out.ReceiptToReply = SummarizeTimings(elapsed)
	if total > 0 {
		v := queued / total
		out.DelegatedQueuedShare = &v
	}
	return out, nil
}

type PickFilter struct {
	Skill, CLI, Machine string
	Count               int
	Strict              bool
}

func skillMatch(a *domain.Agent, skill string) bool {
	if skill == "" {
		return true
	}
	if slices.Contains(a.Capabilities, skill) || slices.Contains(a.Labels, skill) {
		return true
	}
	for _, s := range a.Skills {
		if s.Name == skill {
			return true
		}
	}
	return false
}
func quotaAvailability(q *domain.Quota, now time.Time) string {
	if q == nil {
		return "unknown"
	}
	at := q.CheckedAt
	if at.IsZero() {
		at = q.At
	}
	if at.IsZero() || now.Sub(at) > 15*time.Minute {
		return "unknown"
	}
	blocked := func(s string) bool {
		switch s {
		case "blocked", "rejected", "limit_reached", "rate_limit", "rate_limited":
			return true
		}
		return false
	}
	if blocked(q.Status) {
		return "blocked"
	}
	freshWindows := !q.At.IsZero() && now.Sub(q.At) <= 15*time.Minute
	if !freshWindows {
		return "unknown"
	}
	for _, w := range q.Windows {
		if w.ResetsAt != nil && !w.ResetsAt.After(now) {
			continue
		}
		if blocked(w.Status) || (w.Used >= 1 && w.Status != "allowed" && w.Status != "allowed_warning") {
			return "blocked"
		}
	}
	if q.Status != "" && q.Status != "ok" && q.Status != "allowed" && q.Status != "allowed_warning" && q.Status != "available" {
		return "unknown"
	}
	return "available"
}
func RankAgents(agents []*domain.Agent, states map[domain.AgentName]AgentOverview, stats *api.AgentStatsList, f PickFilter, now time.Time) *api.AgentPickList {
	out := &api.AgentPickList{Version: 1, GeneratedAt: now, Scope: stats.Scope, Candidates: []api.AgentPick{}}
	by := map[string]api.AgentTaskStats{}
	for _, v := range stats.Agents {
		by[v.Name] = v
	}
	for _, a := range agents {
		cli := a.Runtime.CLI
		if cli == "" {
			cli = a.Binding.Harness
		}
		if f.CLI != "" && cli != f.CLI {
			continue
		}
		if f.Machine != "" && a.MachineID != f.Machine && a.Binding.Environment != f.Machine {
			continue
		}
		if !skillMatch(a, f.Skill) {
			continue
		}
		state := states[a.Name]
		c := api.AgentPick{Name: string(a.Name), Online: state.Online, Queued: state.Queued, InProgress: state.InProgress, Quota: quotaAvailability(a.Runtime.Quota, now), Stats: by[string(a.Name)]}
		if a.Runtime.RunnerSlots != "" {
			var slots api.RunnerSlots
			if json.Unmarshal([]byte(a.Runtime.RunnerSlots), &slots) == nil && slots.MaxConcurrent > 0 && !slots.At.IsZero() && now.Sub(slots.At) <= 90*time.Second {
				free := max(0, slots.MaxConcurrent-slots.InUse)
				c.RunnerSlotsFree = &free
			}
		}
		// One running task per agent; two waiting tasks is a full queue.
		c.Available = c.Online && a.Runtime.Away == "" && c.Quota != "blocked" && c.Queued < 2
		if f.Strict && !c.Available {
			continue
		}
		c.Score = 100 - 20*float64(c.Queued) - 15*float64(c.InProgress+state.Held+state.Stale)
		if f.Skill != "" {
			c.Score += 10
		}
		if c.RunnerSlotsFree != nil {
			if *c.RunnerSlotsFree == 0 {
				c.Score -= 20
			} else {
				c.Score += float64(min(*c.RunnerSlotsFree, 4)) * 2
			}
		}
		if c.Quota == "unknown" {
			c.Score -= 5
		}
		if c.Stats.SuccessRate != nil {
			c.Score += 20 * (*c.Stats.SuccessRate - .5)
		}
		if d := c.Stats.ReceiptToReply.MedianSeconds; d != nil {
			c.Score -= math.Min(10, math.Log1p(*d)/2)
		}
		slots := "unknown"
		if c.RunnerSlotsFree != nil {
			slots = fmt.Sprint(*c.RunnerSlotsFree)
		}
		c.Reason = fmt.Sprintf("online=%t queued=%d active=%d slots_free=%s quota=%s held=%d stale=%d", c.Online, c.Queued, c.InProgress, slots, c.Quota, state.Held, state.Stale)
		if a.Runtime.Away != "" {
			c.Reason += " away=" + a.Runtime.Away
		}
		if c.Queued >= 2 {
			c.Reason += " queue_full=true"
		}
		if f.Skill != "" {
			c.Reason += " skill=" + f.Skill
		}
		if c.Stats.SuccessRate != nil {
			c.Reason += fmt.Sprintf(" success=%.0f%%", 100**c.Stats.SuccessRate)
		}
		if d := c.Stats.ReceiptToReply.MedianSeconds; d != nil {
			c.Reason += fmt.Sprintf(" median=%.0fs", *d)
		}
		out.Candidates = append(out.Candidates, c)
	}
	sort.Slice(out.Candidates, func(i, j int) bool {
		a, b := out.Candidates[i], out.Candidates[j]
		if a.Available != b.Available {
			return a.Available
		}
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		return a.Name < b.Name
	})
	if f.Count > 0 && len(out.Candidates) > f.Count {
		out.Candidates = out.Candidates[:f.Count]
	}
	return out
}
