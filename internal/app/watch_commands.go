package app

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/domain"
)

// Runtime is an availability check; the runner independently enforces its
// local permission before claiming or starting a process. Never execute through
// an exec/machine node here: that would bypass its sender/command allowlists.
func commandWatchAgent(a *domain.Agent) error {
	if a == nil || a.MachineID == "" || a.Binding.Environment == "" || a.Binding.Workdir == "" ||
		a.Binding.Harness == "exec" || a.Binding.Harness == "machine" || a.Runtime.Access != "full" ||
		(a.RunSettings.Access != "" && a.RunSettings.Access != "full") || a.Runtime.MaxAccess != "full" ||
		!slices.Contains(a.Runtime.FeatureList(), "command_watch") || a.Runtime.RunnerInstance == "" || a.Delivery.IsPush() {
		return domain.Errf(domain.CodePermissionDenied, "command watches require this agent's locally bound full-access runner on a shell-enabled machine").WithSuggestion("use the agent's permitted computer with a current runner; readonly/workspace and exec/machine nodes cannot run raw watch commands")
	}
	return nil
}

func (s *WatchService) commandCaller(ctx context.Context, by Actor, id, runner string) (*domain.Watch, error) {
	w, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if by.Agent == nil || string(by.Agent.Name) != w.Agent || w.Kind != "command" {
		return nil, domain.Errf(domain.CodePermissionDenied, "only the initiating agent's runner may execute or report a command watch")
	}
	if err := commandWatchAgent(by.Agent); err != nil {
		return nil, err
	}
	if runner == "" || runner != by.Agent.Runtime.RunnerInstance || w.MachineID != by.Agent.MachineID ||
		w.Environment != by.Agent.Binding.Environment || w.Workdir != by.Agent.Binding.Workdir {
		return nil, domain.Errf(domain.CodePermissionDenied, "command watch belongs to another runner, computer, or working directory")
	}
	return w, nil
}

// ClaimCommand leases a due check. Revisions fence concurrent runner instances
// and Hub deadline evaluation without counting a claim as a completed check.
func (s *WatchService) ClaimCommand(ctx context.Context, by Actor, id, runner string) (*domain.Watch, error) {
	w, err := s.commandCaller(ctx, by, id, runner)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	if w.State != domain.WatchWatching || !now.Before(w.Deadline) {
		return nil, domain.Errf(domain.CodeInvalidState, "command watch is no longer active")
	}
	if w.ClaimUntil != nil && now.Before(*w.ClaimUntil) || w.NextCheckAt != nil && now.Before(*w.NextCheckAt) {
		return nil, domain.Errf(domain.CodeConflict, "check is leased or not due yet")
	}
	until := minTime(now.Add(domain.CommandCheckTimeout(w.EverySec)+15*time.Second), w.Deadline)
	next := now.Add(time.Duration(w.EverySec) * time.Second)
	w.ClaimID, w.ClaimRunner, w.ClaimUntil, w.NextCheckAt = s.ids.New("chk"), runner, &until, &next
	changed, _, _, err := s.repo.Save(ctx, w, w.Deadline, w.Checks, nil)
	if err != nil {
		return nil, err
	}
	if !changed {
		return nil, domain.Errf(domain.CodeConflict, "watch changed; claim again when due")
	}
	return w, nil
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

type CommandWatchCheck struct {
	CheckID, Runner, Result string
	Matched                 bool
	ExitCode                int
}

func (s *WatchService) CheckCommand(ctx context.Context, by Actor, id string, in CommandWatchCheck) (*domain.Watch, error) {
	w, err := s.commandCaller(ctx, by, id, in.Runner)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.CheckID) == "" {
		return nil, domain.Errf(domain.CodeInvalidArgument, "check_id is required")
	}
	if w.LastCheckID == in.CheckID || w.State != domain.WatchWatching {
		return w, nil
	}
	if w.ClaimID != in.CheckID || w.ClaimRunner != in.Runner || w.ClaimUntil == nil {
		return nil, domain.Errf(domain.CodeConflict, "check does not own the current lease")
	}
	now := s.clock.Now()
	if now.Before(w.Deadline) && !now.Before(*w.ClaimUntil) {
		return nil, domain.Errf(domain.CodeConflict, "check lease expired")
	}
	oldChecks := w.Checks
	matched := in.Matched
	if w.Match == "" {
		matched = in.ExitCode == 0
	}
	result := domain.SanitizeWatchResult(in.Result)
	w.LastCheckAt, w.LastResult, w.LastCheckID = &now, result, in.CheckID
	w.Checks++
	w.ClaimID, w.ClaimRunner, w.ClaimUntil = "", "", nil
	w.History = append(w.History, domain.WatchCheck{At: now, Result: result, Matched: matched, ExitCode: in.ExitCode})
	if len(w.History) > domain.WatchHistoryLimit {
		w.History = w.History[len(w.History)-domain.WatchHistoryLimit:]
	}
	var wake *domain.Message
	state, outcome := "", ""
	if !now.Before(w.Deadline) {
		state, outcome = domain.WatchExpired, "deadline reached"
	} else if matched {
		state, outcome = domain.WatchMet, "command condition met"
	}
	if state != "" {
		if err := w.End(now, state, "runner", outcome); err != nil {
			return nil, err
		}
		wake = s.wake(w)
	}
	changed, parent, created, err := s.repo.Save(ctx, w, w.Deadline, oldChecks, wake)
	if err != nil {
		return nil, err
	}
	if !changed {
		return nil, domain.Errf(domain.CodeConflict, "watch changed; read it before reporting again")
	}
	s.publish(ctx, parent, created)
	return w, nil
}
