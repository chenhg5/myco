package runner

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/domain"
	"github.com/myco-ai/myco/internal/harness"
	"github.com/myco-ai/myco/internal/harness/execnode"
)

// Optional port: older Hubs/runners continue to serve ordinary messages.
type commandWatchHub interface {
	Me(context.Context) (*api.Agent, error)
	Watches(context.Context, string, string, bool) ([]api.Watch, error)
	ClaimWatch(context.Context, string, string) (*api.Watch, error)
	CheckWatch(context.Context, string, api.WatchCheckRequest) (*api.Watch, error)
}

func (r *Runner) commandPermission(a *api.Agent, w *api.Watch) bool {
	if r.opts.CommandWatchAllowed == nil || !r.opts.CommandWatchAllowed() || a == nil || a.Name != r.opts.Self ||
		a.Runtime == nil || a.Runtime.Access != "full" || a.Runtime.MaxAccess != "full" ||
		a.Runtime.RunnerInstance != r.opts.CommandRunnerID || !slices.Contains(a.Runtime.Features, "command_watch") ||
		a.MachineID == "" || a.Binding.Environment != r.opts.CommandEnvironment || a.Binding.Workdir != r.opts.Workdir {
		return false
	}
	access := string(r.opts.Access)
	if a.RunSettings != nil && a.RunSettings.Access != "" {
		access = a.RunSettings.Access
	}
	if access != "full" {
		return false
	}
	return w == nil || w.Agent == r.opts.Self && w.MachineID == a.MachineID && w.Environment == a.Binding.Environment && w.Workdir == r.opts.Workdir
}

func (r *Runner) runCommandWatches(ctx context.Context) {
	hub, ok := r.hub.(commandWatchHub)
	if !ok || r.opts.CommandWatchAllowed == nil || r.opts.Once {
		return
	}
	tick := r.commandPoll
	if tick <= 0 {
		tick = time.Second
	}
	t := time.NewTicker(tick)
	defer t.Stop()
	var mu sync.Mutex
	running := map[string]context.CancelFunc{}
	var wg sync.WaitGroup
	var idleUntil time.Time
	defer func() {
		mu.Lock()
		for _, stop := range running {
			stop()
		}
		mu.Unlock()
		wg.Wait()
	}()
	poll := func() {
		if time.Now().Before(idleUntil) {
			return
		}
		callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		var me *api.Agent
		if r.opts.CommandWatchAllowed() {
			var err error
			me, err = hub.Me(callCtx)
			if err != nil {
				if !rejectedHubCredential(err) && !IsReplaced(err) {
					return
				}
			}
		}
		allowed := r.commandPermission(me, nil)
		var watches []api.Watch
		if allowed {
			var err error
			watches, err = hub.Watches(callCtx, r.opts.Self, "", false)
			if err != nil {
				return
			} // a failed read is not revocation
		}
		active := map[string]bool{}
		for _, w := range watches {
			if w.Kind == "command" && w.State == "watching" && r.commandPermission(me, &w) {
				active[w.ID] = true
			}
		}
		if len(active) == 0 && r.commandPoll <= 0 {
			idleUntil = time.Now().Add(5 * time.Second)
		}
		mu.Lock()
		for id, stop := range running {
			if !active[id] {
				stop()
			}
		}
		mu.Unlock()
		for _, w := range watches {
			if !active[w.ID] || !time.Now().Before(w.Deadline) || w.NextCheckAt != nil && time.Now().Before(*w.NextCheckAt) {
				continue
			}
			mu.Lock()
			if _, busy := running[w.ID]; busy || len(running) >= 2 {
				mu.Unlock()
				continue
			}
			runCtx, stop := context.WithDeadline(ctx, w.Deadline)
			running[w.ID] = stop
			mu.Unlock()
			wg.Go(func() {
				defer func() { stop(); mu.Lock(); delete(running, w.ID); mu.Unlock() }()
				r.commandWatchCheck(runCtx, hub, w)
			})
		}
	}
	poll()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			poll()
		}
	}
}

func (r *Runner) commandWatchCheck(ctx context.Context, hub commandWatchHub, w api.Watch) {
	done, ok := r.opts.Activity.Work(ctx)
	if !ok {
		return
	}
	defer done()
	claimCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	claimed, err := hub.ClaimWatch(claimCtx, w.ID, r.opts.CommandRunnerID)
	cancel()
	if err != nil {
		return
	}
	// Re-read permission immediately before process creation, including local
	// config changes while the claim was in flight.
	checkCtx, stop := context.WithTimeout(ctx, 5*time.Second)
	me, err := hub.Me(checkCtx)
	stop()
	if err != nil || !r.commandPermission(me, claimed) {
		return
	}
	report := executeWatchCommand(ctx, *claimed, r.opts.Workdir)
	if ctx.Err() != nil {
		return
	} // cancelled work may be retried after its lease
	report.CheckID, report.Runner = claimed.ClaimID, r.opts.CommandRunnerID
	// Retry this result, never the process, while the lease is valid. Its ID
	// makes a lost HTTP response safe to replay without adding another check.
	until := *claimed.ClaimUntil
	sendCtx, done := context.WithDeadline(ctx, until)
	defer done()
	for {
		_, err = hub.CheckWatch(sendCtx, w.ID, report)
		if err == nil || !retryableFinishError(err) {
			return
		}
		select {
		case <-sendCtx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}

func executeWatchCommand(ctx context.Context, w api.Watch, workdir string) api.WatchCheckRequest {
	budget := domain.CommandCheckTimeout(w.EverySec)
	if budget <= 0 {
		budget = 500 * time.Millisecond
	}
	runCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	var mu sync.Mutex
	last := ""
	line := func(b []byte) {
		if s := domain.SanitizeWatchResult(string(b)); s != "" {
			mu.Lock()
			last = s
			mu.Unlock()
		}
	}
	shell := harness.DefaultShell()
	result, err := harness.Exec(runCtx, harness.Process{Binary: shell, Args: harness.ShellArgs(shell, w.Command), Workdir: workdir,
		Env: commandWatchEnv(), CleanEnv: true, MaxOutput: 32 << 10, OnStdoutLine: line, OnStderrLine: line})
	code := result.ExitCode
	matched := code == 0 && err == nil
	if w.Match != "" {
		re, e := regexp.Compile(w.Match)
		matched = e == nil && re.MatchString(strings.TrimSpace(result.Stdout+"\n"+result.Stderr))
	}
	if runCtx.Err() != nil {
		code, matched = 124, false
		line([]byte("command timed out"))
	} else if !result.Started {
		code, matched = 126, false
		line([]byte("command could not start"))
	}
	mu.Lock()
	summary := last
	mu.Unlock()
	if summary == "" {
		summary = fmt.Sprintf("exit %d", code)
	}
	if result.Truncated {
		summary = domain.SanitizeWatchResult(summary + " [output truncated]")
	}
	return api.WatchCheckRequest{Result: strings.TrimSpace(summary), Matched: matched, ExitCode: code}
}

func commandWatchEnv() []string {
	var env []string
	for _, key := range execnode.DefaultInheritEnv {
		if value, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+value)
		}
	}
	return env
}
