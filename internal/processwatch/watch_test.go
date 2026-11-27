package processwatch

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

func fixture() (*Manager, time.Time, Owner) {
	m := New("test-runner")
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	owner := Owner{Workspace: "test-workspace", Member: "owner", Agent: "agent", Session: "session"}
	scope := &Scope{manager: m, owner: owner, root: Process{PID: 40, Birth: "root"}}
	m.scopes = []*Scope{scope}
	m.previous[50] = &observed{process: Process{PID: 50, Birth: "child"}, scope: scope}
	return m, at, owner
}
func observation(at time.Time, minutes int) Sample {
	return Sample{At: at.Add(time.Duration(minutes) * time.Minute), Complete: true, Cores: 8, Total: 1000, Available: 500, LoadMetric: "load1_per_core", Processes: []Process{{PID: 50, Parent: 1, Group: 40, Birth: "child", Name: "worker", User: true, CPUTime: float64(minutes) * 60}}}
}
func TestOrphanNeedsThirtyMinutesAndProvenOwnership(t *testing.T) {
	m, at, _ := fixture()
	for i := 0; i < 30; i++ {
		m.Observe(observation(at, i))
		if m.previous[50].ref != "" {
			t.Fatalf("early suspect at minute %d", i)
		}
	}
	m.Observe(observation(at, 30))
	if m.previous[50].ref == "" || !m.alert {
		t.Fatal("sustained owned orphan not reported")
	}
	// This is an alert, not a call to an OS kill function: the scope stays live.
	if m.scopes[0].stopped {
		t.Fatal("watchdog stopped a scope")
	}
}
func TestUnknownOrphanNeverHasTerminationReference(t *testing.T) {
	m := New("test")
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i <= 40; i++ {
		s := observation(at, i)
		s.LoadRatio = .9
		m.Observe(s)
	}
	if !m.alert {
		t.Fatal("unknown processes must still contribute to machine pressure")
	}
	if m.previous[50].ref != "" || m.previous[50].scope != nil {
		t.Fatal("unknown orphan was attributed")
	}
}
func TestPIDReuseAndSamplingGapResetSuspicion(t *testing.T) {
	for _, kind := range []string{"gap", "invalid", "pid-reuse"} {
		t.Run(kind, func(t *testing.T) {
			m, at, _ := fixture()
			for i := 0; i < 30; i++ {
				m.Observe(observation(at, i))
			}
			s := observation(at, 30)
			switch kind {
			case "gap":
				s.At = s.At.Add(3 * time.Minute)
			case "invalid":
				s.Complete = false
			case "pid-reuse":
				s.Processes[0].Birth = "replacement"
				s.Processes[0].Group = 99
			}
			m.Observe(s)
			if o := m.previous[50]; o.ref != "" || !o.since.IsZero() {
				t.Fatal("kept continuity without evidence")
			}
			if kind == "pid-reuse" && m.previous[50].scope != nil {
				t.Fatal("PID reuse inherited ownership")
			}
		})
	}
}
func TestMachineThresholdsAndRecovery(t *testing.T) {
	for _, kind := range []string{"load", "memory"} {
		t.Run(kind, func(t *testing.T) {
			m := New("test")
			at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			for i := 0; i <= 10; i++ {
				s := observation(at, i)
				s.Processes = nil
				if kind == "load" {
					s.LoadRatio = .71
				} else {
					s.Available = 99
				}
				m.Observe(s)
				if m.alert != (i == 10) {
					t.Fatalf("threshold at %d: %v", i, m.alert)
				}
			}
			invalid := observation(at, 11)
			invalid.Complete = false
			m.Observe(invalid)
			if !m.alert {
				t.Fatal("incomplete sample falsely recovered")
			}
			healthy := observation(at, 12)
			healthy.Processes = nil
			m.Observe(healthy)
			if m.alert {
				t.Fatal("did not recover from fresh healthy sample")
			}
		})
	}
}
func TestLiveParentAndLowCPUAreNotSuspects(t *testing.T) {
	for _, kind := range []string{"parent", "low-cpu"} {
		t.Run(kind, func(t *testing.T) {
			m, at, _ := fixture()
			for i := 0; i <= 40; i++ {
				s := observation(at, i)
				if kind == "parent" {
					s.Processes[0].Parent = 40
					s.Processes = append(s.Processes, Process{PID: 40, Birth: "root", Name: "harness"})
				} else {
					s.Processes[0].CPUTime = float64(i) * 10
				}
				m.Observe(s)
			}
			if m.previous[50].ref != "" {
				t.Fatal("not a sustained orphan")
			}
		})
	}
}
func TestTerminationFailsClosedWithoutFreshEvidence(t *testing.T) {
	m, at, owner := fixture()
	m.Observe(observation(at, 0))
	if _, e := m.Terminate(context.Background(), owner.Workspace, owner.Member, ""); e != ErrIdentity {
		t.Fatal(e)
	}
	m.previous[50].ref = "reference"
	if _, e := m.AgentFor("other-workspace", owner.Member, "reference"); e != ErrIdentity {
		t.Fatal("cross-workspace reference accepted")
	}
	if _, e := m.AgentFor(owner.Workspace, "other-owner", "reference"); e != ErrIdentity {
		t.Fatal("cross-owner reference accepted")
	}
	if _, e := m.Terminate(context.Background(), owner.Workspace, owner.Member, "reference"); e != ErrIdentity {
		t.Fatal("stale sample accepted", e)
	}
}

func TestTerminationRevalidatesIdentityAndNeverSignalsUnknown(t *testing.T) {
	m, at, owner := fixture()
	for i := 0; i <= 30; i++ {
		m.Observe(observation(at, i))
	}
	now := at.Add(30 * time.Minute)
	m.now = func() time.Time { return now }
	suspect := m.previous[50]
	reference := suspect.ref
	calls := 0
	m.signal = func(context.Context, Process, time.Duration) error { calls++; return nil }
	m.lookup = func(pid int) (Process, error) { p := suspect.process; p.Birth = "reused"; return p, nil }
	if _, e := m.Terminate(context.Background(), owner.Workspace, owner.Member, reference); e != ErrIdentity || calls != 0 {
		t.Fatal("reused PID was signalled", e, calls)
	}
	// Windows identity reads intentionally contain no executable name.
	m.lookup = func(int) (Process, error) {
		return Process{PID: suspect.process.PID, Birth: suspect.process.Birth}, nil
	}
	if _, e := m.Terminate(context.Background(), owner.Workspace, "another-owner", reference); e != ErrIdentity || calls != 0 {
		t.Fatal("cross-owner signal", e)
	}
	if stopped, e := m.Terminate(context.Background(), owner.Workspace, owner.Member, reference); e != nil || calls != 1 || stopped.Name != suspect.process.Name || stopped.Agent != owner.Agent {
		t.Fatal("verified signal metadata missing", e, calls, stopped.Name, stopped.Agent)
	}
	if _, e := m.Terminate(context.Background(), owner.Workspace, owner.Member, reference); e != ErrIdentity || calls != 1 {
		t.Fatal("replayed reference signalled again", e, calls)
	}
}
func TestIncompleteSamplePreservesOwnershipEvidence(t *testing.T) {
	m, at, _ := fixture()
	m.Observe(observation(at, 0))
	m.Observe(Sample{At: at.Add(time.Minute)})
	m.Observe(observation(at, 2))
	if m.previous[50].scope == nil || m.previous[50].ref != "" {
		t.Fatal("lost ownership or retained CPU continuity across gap")
	}
}

func TestNoHarnessStartsAfterRunnerOrOriginalTurnStopped(t *testing.T) {
	for _, kind := range []string{"runner", "original-turn", "scope"} {
		t.Run(kind, func(t *testing.T) {
			m := New("test")
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			owner := Owner{Agent: "test"}
			owned := WithOwner(parent, m, owner)
			// ACP detaches transport cancellation, but must retain the original turn's
			// launch guard so delayed startup cannot outlive explicit cleanup.
			detached := context.WithoutCancel(owned)
			scope, e := m.PrepareContext(detached, exec.Command("/must-not-execute"), owner)
			if e != nil {
				t.Fatal(e)
			}
			if kind == "runner" {
				m.Close()
			} else {
				cancel()
			}
			if e = scope.Start(); e != context.Canceled || len(m.scopes) != 0 {
				t.Fatal("launched after stop", e)
			}
		})
	}
}

func TestReusedParentCannotClaimHistoricalProcess(t *testing.T) {
	m := New("test")
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	birth := func(n int) string {
		if runtime.GOOS == "windows" {
			return fmt.Sprintf("%020d", n)
		}
		return fmt.Sprintf("%d:0", n)
	}
	parent := Process{PID: 40, Birth: birth(200), Group: 40, User: true}
	scope := &Scope{manager: m, owner: Owner{Agent: "new-agent"}, root: parent}
	m.scopes = []*Scope{scope}
	m.previous[40] = &observed{process: parent, scope: scope}
	s := observation(at, 0)
	s.Processes = []Process{parent, {PID: 50, Parent: 40, Group: 999, Birth: birth(100), User: true, Name: "historical-worker"}}
	m.Observe(s)
	if m.previous[50].scope != nil {
		t.Fatal("reused parent claimed an older, unknown process")
	}
}
