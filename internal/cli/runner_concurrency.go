package cli

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/config"
	"github.com/myco-ai/myco/internal/runner"
)

// How often the runner looks at machine.yaml's runner.max_concurrent, and
// how often it says it is still there (runner-slots.json) with nothing new.
const (
	slotLimitPoll      = 3 * time.Second
	slotStateHeartbeat = 30 * time.Second
)

// slotLimit keeps a runner's slot pool (runner start --all) on
// config.EffectiveConcurrency: its explicit flag, or machine.yaml's setting
// when that was chosen later, read live. It writes runner-slots.json for the
// App on this computer and answers the machine node.
type slotLimit struct {
	defaultPath string
	pool        *runner.SlotPool
	flag        int
	flagAt      time.Time
	log         *slog.Logger
	now         func() time.Time

	mu     sync.Mutex
	source string
}

// newSlotLimit starts the pool at the limit the rule gives now. flag is the
// explicit --max-concurrent (0: not given).
func newSlotLimit(defaultPath string, flag int, log *slog.Logger) *slotLimit {
	now := time.Now()
	prev, _ := config.LoadRunnerSlots(defaultPath)
	l := &slotLimit{defaultPath: defaultPath, flag: flag, flagAt: config.FlagSince(prev, flag, now), log: log, now: time.Now}
	n, src := l.rule()
	l.pool, l.source = runner.NewSlotPool(n), src
	return l
}

func (l *slotLimit) rule() (int, string) {
	set, err := config.LoadRunnerPolicy(l.defaultPath)
	if err != nil && l.log != nil {
		l.log.Debug("reading machine.yaml's runner section failed", "err", err)
	}
	return config.EffectiveConcurrency(l.flag, l.flagAt, set)
}

// apply puts the pool on the rule; it says whether the limit changed.
func (l *slotLimit) apply() bool {
	n, src := l.rule()
	l.mu.Lock()
	l.source = src
	l.mu.Unlock()
	if l.pool.Max() == n {
		return false
	}
	if l.log != nil {
		l.log.Info("agent CLIs running at once changed", "limit", n, "from", src)
	}
	l.pool.SetMax(n)
	return true
}

// View is the limit and the slots now.
func (l *slotLimit) View() *api.MachineConcurrency {
	s := l.pool.Snapshot("")
	l.mu.Lock()
	src := l.source
	l.mu.Unlock()
	return &api.MachineConcurrency{Limit: s.Limit, InUse: s.InUse, Waiting: s.Waiting, Source: src, Flag: l.flag, Ceiling: config.MaxConcurrentCeiling}
}

// Set writes n as machine.yaml's setting (it is chosen now, so it wins over
// the flag) and applies it at once.
func (l *slotLimit) Set(n int) (*api.MachineConcurrency, error) {
	if err := config.SetRunnerConcurrency(l.defaultPath, n, l.now()); err != nil {
		return nil, err
	}
	l.apply()
	l.save()
	return l.View(), nil
}

func (l *slotLimit) save() {
	v := l.View()
	st := &config.RunnerSlotsState{PID: os.Getpid(), Version: Version, Limit: v.Limit, Source: v.Source, Flag: l.flag, FlagAt: l.flagAt,
		InUse: v.InUse, Waiting: v.Waiting, At: l.now()}
	if err := config.SaveRunnerSlots(l.defaultPath, st); err != nil && l.log != nil {
		l.log.Debug("writing runner-slots.json failed", "err", err)
	}
}

// Run follows machine.yaml and keeps runner-slots.json current until ctx
// ends: every change is written, at most once a second while slots change
// (the last of a burst a moment later, never dropped), and every 30 seconds
// with nothing new.
func (l *slotLimit) Run(ctx context.Context) {
	changes, stop := l.pool.Subscribe()
	defer stop()
	poll := time.NewTicker(slotLimitPoll)
	defer poll.Stop()
	l.save()
	last := time.Now()
	var flush <-chan time.Time // the trailing write of a burst
	changed := func() {
		if wait := time.Second - time.Since(last); wait > 0 {
			if flush == nil {
				flush = time.After(wait)
			}
			return
		}
		l.save()
		last, flush = time.Now(), nil
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-changes:
			changed()
		case <-flush:
			flush = nil
			l.save()
			last = time.Now()
		case <-poll.C:
			if l.apply() || time.Since(last) >= slotStateHeartbeat {
				changed()
			}
		}
	}
}

// viewFunc and setFunc are the machine node's Deps (nil without --all: the
// node then says this runner does not limit them).
func (l *slotLimit) viewFunc() func() *api.MachineConcurrency {
	if l == nil {
		return nil
	}
	return l.View
}

func (l *slotLimit) setFunc() func(int) (*api.MachineConcurrency, error) {
	if l == nil {
		return nil
	}
	return l.Set
}
