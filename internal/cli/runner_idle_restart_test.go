package cli

import (
	"errors"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/runner"
	"github.com/myco-ai/myco/internal/runnerupdate"
	"github.com/myco-ai/myco/internal/service"
)

func TestIdleRestartWaitsForRunningTaskAndPendingResult(t *testing.T) {
	now := time.Now().UTC()
	base := t.TempDir()
	a := &runner.Activity{}
	req := runnerupdate.Request{ID: "one", Target: "v9.0.0", RequestedAt: now, Deadline: now.Add(runnerupdate.IdleLimit)}
	if err := runnerupdate.Write(base, "request", req); err != nil {
		t.Fatal(err)
	}
	starts := 0
	finished := make(chan error, 1)
	s := idleRestart{base: base, activity: a, runtime: runnerupdate.Runtime{Version: "v1.0.0"}, start: func(*runnerupdate.Request) (<-chan error, error) { starts++; return finished, nil }}
	done, _ := a.Work(t.Context())
	s.step(now)
	if starts != 0 {
		t.Fatal("restarted while task was running")
	}
	a.Pending("agent", 1)
	done()
	s.step(now)
	if starts != 0 {
		t.Fatal("restarted before delivering its result")
	}
	a.Pending("agent", 0)
	s.step(now)
	if starts != 1 || s.runtime.State != "restarting" {
		t.Fatalf("did not restart after task: %+v starts=%d", s.runtime, starts)
	}
	// A newer request must not reopen claims during an uncertain handoff.
	req.ID = "two"
	runnerupdate.Write(base, "request", req)
	s.step(now)
	if !a.Quiescent() || starts != 1 {
		t.Fatal("a second upgrade reopened claims")
	}
	finished <- nil
	s.step(now)
	s.step(now)
	if !a.Quiescent() || starts != 1 {
		t.Fatal("successful handoff was repeated")
	}
}

func TestIdleRestartExpiresWithoutInterruptingWork(t *testing.T) {
	now := time.Now().UTC()
	base := t.TempDir()
	a := &runner.Activity{}
	req := runnerupdate.Request{ID: "one", Target: "v2.0.0", RequestedAt: now, Deadline: now.Add(time.Minute)}
	runnerupdate.Write(base, "request", req)
	started := false
	s := idleRestart{base: base, activity: a, runtime: runnerupdate.Runtime{Version: "v1.0.0"}, start: func(*runnerupdate.Request) (<-chan error, error) {
		started = true
		return nil, errors.New("must not start")
	}}
	done, _ := a.Work(t.Context())
	s.step(now.Add(time.Minute))
	done()
	s.step(now.Add(2 * time.Minute))
	if started || s.runtime.State != "expired" {
		t.Fatalf("expired request restarted: %+v", s.runtime)
	}
	work, ok := a.Work(t.Context())
	if !ok {
		t.Fatal("expiry left admissions paused")
	}
	work()
}

func TestIdleRestartRejectsStaleServiceHandoffs(t *testing.T) {
	now := time.Now()
	rt := runnerupdate.Runtime{At: now, PID: 123, Instance: "instance", RequestID: "request", State: "restarting"}
	st := service.Status{Running: true, PID: 123}
	if !validIdleHandoff(rt, st, "request", "instance", now) {
		t.Fatal("valid handoff refused")
	}
	for _, tc := range []struct {
		name, request, instance string
		pid                     int
		at                      time.Time
		state                   string
	}{
		{"another request", "other", "instance", 123, now, "restarting"}, {"another runner", "request", "other", 123, now, "restarting"}, {"replacement service", "request", "instance", 124, now, "restarting"}, {"stale", "request", "instance", 123, now.Add(-time.Minute), "restarting"}, {"not paused", "request", "instance", 123, now, "pending"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			copy := rt
			copy.At = tc.at
			copy.State = tc.state
			status := st
			status.PID = tc.pid
			if validIdleHandoff(copy, status, tc.request, tc.instance, now) {
				t.Fatal("unsafe handoff accepted")
			}
		})
	}
}
func TestIdleRestartDoesNotRepeatAnUnchangedServiceVersion(t *testing.T) {
	now := time.Now()
	base := t.TempDir()
	runnerupdate.Write(base, "request", runnerupdate.Request{ID: "request", Target: "v2.0.0", RequestedAt: now, Deadline: now.Add(time.Hour)})
	runnerupdate.Write(base, "attempt", runnerupdate.Runtime{RequestID: "request", Instance: "old"})
	s := idleRestart{base: base, activity: &runner.Activity{}, runtime: runnerupdate.Runtime{Version: "v1.0.0", Instance: "new"}, start: func(*runnerupdate.Request) (<-chan error, error) { t.Fatal("repeated restart"); return nil, nil }}
	s.step(now)
	if s.runtime.State != "failed" {
		t.Fatalf("unchanged version: %+v", s.runtime)
	}
}

func TestIdleRestartUnknownServiceOutcomeKeepsClaimsPaused(t *testing.T) {
	now := time.Now()
	base := t.TempDir()
	a := &runner.Activity{}
	req := runnerupdate.Request{ID: "request", Target: "v2.0.0", RequestedAt: now, Deadline: now.Add(time.Hour)}
	runnerupdate.Write(base, "request", req)
	done := make(chan error, 1)
	starts := 0
	s := idleRestart{base: base, activity: a, runtime: runnerupdate.Runtime{Version: "v1.0.0"}, start: func(*runnerupdate.Request) (<-chan error, error) {
		starts++
		runnerupdate.Write(base, "attempt", runnerupdate.Runtime{RequestID: req.ID, State: "service_call"})
		return done, nil
	}}
	s.step(now)
	done <- errors.New("service command lost its answer")
	s.step(now)
	if s.runtime.State != "restart_unknown" || !a.Quiescent() {
		t.Fatalf("unknown handoff reopened claims: %+v", s.runtime)
	}
	req.ID = "new"
	runnerupdate.Write(base, "request", req)
	s.step(now)
	if starts != 1 || !a.Quiescent() {
		t.Fatal("unknown handoff retried or reopened claims")
	}
}
