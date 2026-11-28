package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/config"
	"github.com/myco-ai/myco/internal/runner"
	"github.com/myco-ai/myco/internal/runnerupdate"
	"github.com/myco-ai/myco/internal/service"
)

func (e *env) requestIdleRestart(target string) (*runnerupdate.Request, error) {
	now := time.Now().UTC()
	r := &runnerupdate.Request{ID: fmt.Sprintf("%d", now.UnixNano()), Target: target, RequestedAt: now, Deadline: now.Add(runnerupdate.IdleLimit)}
	return r, runnerupdate.Write(config.Dir(e.defaultPath), "request", r)
}

// Only the process that owns the service can hand it off. A terminal runner
// must not restart a different service, whose agents may be working.
func (e *env) startIdleServiceRestart(req *runnerupdate.Request, path, version string) (<-chan error, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	m, err := service.Detect(home, idleServiceStatusRunner{})
	if err != nil {
		return nil, err
	}
	st, err := m.Status()
	if err != nil {
		return nil, err
	}
	if !st.Running || st.PID != os.Getpid() {
		return nil, fmt.Errorf("this runner is not the managed service; restart pending")
	}
	if !time.Now().Before(req.Deadline) {
		return nil, fmt.Errorf("idle restart wait expired")
	}
	attempt := runnerupdate.Runtime{RequestID: req.ID, Instance: runnerInstance, PID: os.Getpid(), Version: version, At: time.Now().UTC(), State: "restarting"}
	if err := runnerupdate.Write(config.Dir(e.defaultPath), "attempt", attempt); err != nil {
		return nil, err
	}
	if err := runnerupdate.Write(config.Dir(e.defaultPath), "runtime", attempt); err != nil {
		return nil, err
	}
	cmd := exec.Command(path, "runner", "service", "restart", "--idle-request", req.ID, "--idle-instance", runnerInstance, "--format", "json")
	detachProcess(cmd)
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	return done, nil
}

// The runner owns the admission latch; a Hub snapshot cannot authorize it.
type idleRestart struct {
	base       string
	activity   *runner.Activity
	runtime    runnerupdate.Runtime
	restarting <-chan error
	start      func(*runnerupdate.Request) (<-chan error, error)
}

func (s *idleRestart) step(now time.Time) {
	s.runtime.At = now
	req, err := runnerupdate.LoadRequest(s.base)
	if err != nil {
		return
	}
	// Never release the latch while a service restart may already be in flight,
	// including when a second upgrade request replaces the first receipt.
	if s.restarting != nil {
		select {
		case err := <-s.restarting:
			s.restarting = nil
			if err != nil {
				var attempt runnerupdate.Runtime
				if runnerupdate.Read(s.base, "attempt", &attempt) == nil && attempt.RequestID == s.runtime.RequestID && attempt.State == "restarting" {
					s.runtime.State = "failed"
					s.runtime.Error = "runner restart was not started; retry when idle"
					s.activity.Resume()
				} else {
					s.runtime.State = "restart_unknown"
					s.runtime.Error = "runner restart status is unknown; check the service before restarting it manually"
				}
			}
		default:
		}
		return
	}
	if s.runtime.State == "restarting" || s.runtime.State == "restart_unknown" {
		return
	}
	if s.runtime.RequestID != req.ID {
		s.activity.Resume()
		s.runtime.RequestID = req.ID
		s.runtime.State = "pending"
		s.runtime.Error = ""
	}
	switch {
	case strings.TrimPrefix(req.Target, "v") == strings.TrimPrefix(s.runtime.Version, "v"):
		s.runtime.State = "complete"
		s.activity.Resume()
	case s.runtime.State == "failed" || s.runtime.State == "expired":
	case !now.Before(req.Deadline):
		s.runtime.State = "expired"
		s.activity.Resume()
	case s.alreadyAttempted(req.ID):
		s.runtime.State = "failed"
		s.runtime.Error = "runner restart did not load the installed version; retry when idle"
	case s.activity.PauseIfIdle() && s.activity.Quiescent():
		s.restarting, err = s.start(req)
		if err != nil {
			s.runtime.State = "failed"
			s.runtime.Error = err.Error()
			s.activity.Resume()
		} else {
			s.runtime.State = "restarting"
		}
	}
}
func (s *idleRestart) alreadyAttempted(id string) bool {
	var attempt runnerupdate.Runtime
	return runnerupdate.Read(s.base, "attempt", &attempt) == nil && attempt.RequestID == id
}

func (e *env) watchIdleRestart(ctx context.Context, a *runner.Activity, log *slog.Logger, path, version string) {
	if a == nil {
		return
	}
	s := idleRestart{base: config.Dir(e.defaultPath), activity: a, runtime: runnerupdate.Runtime{PID: os.Getpid(), Instance: runnerInstance, Version: version}, start: func(req *runnerupdate.Request) (<-chan error, error) {
		return e.startIdleServiceRestart(req, path, version)
	}}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		prev := s.runtime.State
		s.step(time.Now().UTC())
		if prev != "restarting" && s.runtime.State == "restarting" {
			log.Info("restarting idle runner for installed CLI update")
		}
		if err := runnerupdate.Write(s.base, "runtime", s.runtime); err != nil {
			log.Warn("cannot save runner upgrade status", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// Status queries must not hold the paused admission latch indefinitely.
type idleServiceStatusRunner struct{}

func (idleServiceStatusRunner) Run(name string, args ...string) (string, error) {
	ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if ctx.Err() != nil {
		return "", fmt.Errorf("service status timed out")
	}
	return string(out), err
}

// Join the watcher before command return: tests, profile replacement and Windows
// cleanup must not leave a goroutine reading environment or rewriting old state.
func (e *env) startIdleWatcher(a *runner.Activity, log *slog.Logger) func() {
	path, err := upgradeTarget(e.in.Getenv)
	if err != nil {
		log.Warn("cannot locate installed CLI for idle restart")
		return func() {}
	}
	version := Version
	ctx, stop := context.WithCancel(e.in.Ctx)
	done := make(chan struct{})
	go func() { defer close(done); e.watchIdleRestart(ctx, a, log, path, version) }()
	return func() { stop(); <-done }
}
