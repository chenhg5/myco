// Package processwatch owns runner children and observes machine pressure.
// It never terminates a process in response to a metric alone.
package processwatch

import (
	"context"
	"crypto/rand"
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/myco-ai/myco/internal/domain"
)

const Grace = 5 * time.Second

var ErrIdentity = errors.New("process is gone, changed identity, or is no longer a verified suspect")

type Process struct {
	PID, Parent, Group int
	Birth, Name        string
	CPUTime            float64 // cumulative seconds, not a sampled percentage
	Memory             uint64
	User               bool // same OS user; never expose another user's process details
}
type Sample struct {
	At               time.Time
	Complete         bool
	Cores            int
	LoadRatio        float64
	LoadMetric       string
	Total, Available uint64
	Processes        []Process
}
type Owner struct{ Workspace, Member, Agent, Session string }
type key struct{}
type contextOwner struct {
	Done    <-chan struct{}
	Manager *Manager
	Owner   Owner
}

func WithOwner(ctx context.Context, m *Manager, owner Owner) context.Context {
	return context.WithValue(ctx, key{}, contextOwner{Manager: m, Owner: owner, Done: ctx.Done()})
}
func FromContext(ctx context.Context) (*Manager, Owner) {
	v, _ := ctx.Value(key{}).(contextOwner)
	return v.Manager, v.Owner
}

type observed struct {
	process Process
	at      time.Time
	since   time.Time
	cpu     float64
	ref     string
	scope   *Scope
}
type Manager struct {
	closed              bool
	now                 func() time.Time
	lookup              func(int) (Process, error)
	signal              func(context.Context, Process, time.Duration) error
	incident            string
	mu                  sync.Mutex
	instance            string
	stateDir            string
	scopes              []*Scope
	previous            map[int]*observed
	latest              Sample
	highLoad, lowMemory time.Time
	alert               bool
	read                func(context.Context) (Sample, error)
}

func New(instance string) *Manager {
	return &Manager{instance: instance, previous: map[int]*observed{}, read: sample, now: time.Now, lookup: readProcess, signal: terminate}
}

// Scope survives a successful foreground turn, until its children are gone or
// the logical session is explicitly stopped. The OS handle is not tied to Wait.
type Scope struct {
	cancelled <-chan struct{}
	manager   *Manager
	owner     Owner
	root      Process
	command   *exec.Cmd
	native    nativeScope
	stopped   bool
	members   map[int]Process
}

func (m *Manager) Prepare(cmd *exec.Cmd, owner Owner) (*Scope, error) {
	// Allocate OS resources only at Start, after callers have prepared pipes.
	return &Scope{manager: m, owner: owner, command: cmd}, nil
}
func (m *Manager) PrepareContext(ctx context.Context, cmd *exec.Cmd, owner Owner) (*Scope, error) {
	scope, err := m.Prepare(cmd, owner)
	if v, ok := ctx.Value(key{}).(contextOwner); ok {
		scope.cancelled = v.Done
	} else {
		scope.cancelled = ctx.Done()
	}
	return scope, err
}
func (s *Scope) Start() error {
	m := s.manager
	m.mu.Lock()
	defer m.mu.Unlock() // no scan can observe a half-registered launch
	if m.closed || s.stopped {
		return context.Canceled
	}
	select {
	case <-s.cancelled:
		return context.Canceled
	default:
	}
	if err := s.native.prepare(s.command); err != nil {
		return err
	}
	if err := s.command.Start(); err != nil {
		s.native.close()
		return err
	}
	if err := s.native.started(s.command); err != nil {
		_ = s.command.Process.Kill()
		_ = s.command.Wait()
		s.native.close()
		return err
	}
	p, err := readProcess(s.command.Process.Pid)
	if err != nil {
		_ = s.native.stop(s.command.Process.Pid, Grace)
		_ = s.command.Wait()
		s.native.close()
		return errors.New("cannot establish child process creation identity")
	}
	s.root = p
	s.members = map[int]Process{p.PID: p}
	m.scopes = append(m.scopes, s)
	m.saveLocked()
	return nil
}
func (s *Scope) Cancel() error {
	m := s.manager
	m.mu.Lock()
	defer m.mu.Unlock()
	return s.stopLocked()
}
func (s *Scope) stopLocked() error {
	if s.stopped {
		return nil
	}
	s.stopped = true
	if s.root.PID <= 1 || s.root.Birth == "" {
		s.native.close()
		return nil
	}
	// The OS adapter checks the original root/member identities before signalling.
	var members []Process
	for _, p := range s.members {
		members = append(members, p)
	}
	err := s.native.stopVerified(s.root, members, Grace)
	if err != nil {
		s.stopped = false
		return err
	}
	s.native.close()
	return err
}
func (m *Manager) stopMatching(match func(*Scope) bool) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var wg sync.WaitGroup
	for _, s := range m.scopes {
		if match(s) {
			wg.Add(1)
			go func() { defer wg.Done(); _ = s.stopLocked() }()
		}
	}
	wg.Wait()
	m.saveLocked()
}
func (m *Manager) StopSession(owner Owner) {
	m.stopMatching(func(s *Scope) bool { return s.owner == owner })
}
func (m *Manager) StopAgent(workspace, agent string) {
	m.stopMatching(func(s *Scope) bool { return s.owner.Workspace == workspace && s.owner.Agent == agent })
}
func (m *Manager) Close() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	m.stopMatching(func(*Scope) bool { return true })
}
func (m *Manager) Run(ctx context.Context) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		sampleCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		s, err := m.read(sampleCtx)
		cancel()
		if err != nil {
			s = Sample{At: time.Now()}
		}
		m.Observe(s)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
func elapsed(now time.Time, since *time.Time, condition bool, valid bool, threshold time.Duration) bool {
	if !valid || !condition {
		*since = time.Time{}
		return false
	}
	if since.IsZero() {
		*since = now
	}
	return now.Sub(*since) >= threshold
}

// Observe is deterministic: tests provide timestamps and cumulative counters.
func (m *Manager) Observe(s Sample) {
	m.mu.Lock()
	defer m.mu.Unlock()
	valid := s.Complete && !s.At.IsZero() && s.Cores > 0 && s.Total > 0 && s.Available <= s.Total
	dt := s.At.Sub(m.latest.At)
	continuous := valid && m.latest.Complete && dt > 0 && dt <= 2*time.Minute
	if !continuous {
		m.highLoad = time.Time{}
		m.lowMemory = time.Time{}
	}
	load := elapsed(s.At, &m.highLoad, s.LoadRatio > .70, valid, 10*time.Minute)
	memory := elapsed(s.At, &m.lowMemory, float64(s.Available)/float64(max(s.Total, 1)) < .10, valid, 10*time.Minute)
	byPID := map[int]Process{}
	for _, p := range s.Processes {
		byPID[p.PID] = p
	}
	next := map[int]*observed{}
	for _, p := range s.Processes {
		if !p.User || p.PID <= 1 || p.Birth == "" {
			continue
		}
		o := &observed{process: p, at: s.At}
		prior := m.previous[p.PID]
		if prior != nil && prior.process.Birth == p.Birth {
			o.scope = prior.scope
		}
		if o.scope == nil {
			for _, scope := range m.scopes {
				if !scope.stopped && (scope.members[p.PID].Birth == p.Birth || scope.native.contains(scope.root, p, byPID)) {
					o.scope = scope
					break
				}
			}
		}
		// An escaped child is attributable only through an already verified parent.
		if o.scope == nil {
			if parent := m.previous[p.Parent]; parent != nil && byPID[p.Parent].Birth == parent.process.Birth && bornWithin(parent.process, p) {
				o.scope = parent.scope
			}
		}
		if prior != nil && prior.process.Birth == p.Birth && continuous && p.CPUTime >= prior.process.CPUTime {
			o.cpu = 100 * (p.CPUTime - prior.process.CPUTime) / dt.Seconds()
			if o.scope != nil && !o.scope.stopped && orphan(p, byPID, o.scope.root) && o.cpu >= 80 {
				o.since = prior.since
				if o.since.IsZero() {
					o.since = prior.at
				}
				if s.At.Sub(o.since) >= 30*time.Minute {
					o.ref = prior.ref
					if o.ref == "" {
						o.ref = rand.Text()
					}
				}
			}
		}
		if o.scope != nil {
			if o.scope.members == nil {
				o.scope.members = map[int]Process{}
			}
			o.scope.members[p.PID] = p
		}
		next[p.PID] = o
	}
	wasAlert := m.alert
	if valid {
		stillPressured := s.LoadRatio > .70 || float64(s.Available)/float64(max(s.Total, 1)) < .10
		for _, o := range next {
			if o.scope != nil && !o.scope.stopped && o.cpu >= 80 && orphan(o.process, byPID, o.scope.root) {
				stillPressured = true
			}
		}
		m.alert = load || memory || (m.alert && stillPressured)
		for _, o := range next {
			m.alert = m.alert || o.ref != ""
		}
	}
	if !wasAlert && m.alert {
		m.incident = s.At.UTC().Format(time.RFC3339Nano)
	}
	m.previous = next
	m.latest = s
	m.latest.Complete = valid
	// Release empty Jobs, never a Job merely because its foreground exited.
	keep := m.scopes[:0]
	for _, scope := range m.scopes {
		alive := false
		for _, o := range next {
			if o.scope == scope {
				alive = true
				break
			}
		}
		if scope.stopped || (!alive && valid && scope.native.empty(scope.root, scope.members)) {
			scope.native.close()
		} else {
			keep = append(keep, scope)
		}
	}
	m.scopes = keep
	m.saveLocked()
}
func (m *Manager) Snapshot(workspace, member string) *domain.MachineHealth {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.latest
	if s.At.IsZero() {
		return nil
	}
	h := &domain.MachineHealth{OwnerID: member, Incident: m.incident, At: s.At, RunnerInstance: m.instance, Complete: s.Complete, Alert: m.alert, LoadRatio: s.LoadRatio, LoadMetric: s.LoadMetric, Cores: s.Cores, MemoryTotal: s.Total, MemoryAvailable: s.Available, Processes: []domain.MachineProcess{}}
	for _, o := range m.previous {
		p := o.process
		agent, ref := "", ""
		if o.scope != nil {
			if o.scope.owner.Workspace != workspace || o.scope.owner.Member != member {
				continue
			}
			agent = o.scope.owner.Agent
			if !o.scope.stopped && s.Complete && m.now().Sub(s.At) >= 0 && m.now().Sub(s.At) <= 2*time.Minute {
				ref = o.ref
			}
		}
		h.Processes = append(h.Processes, domain.MachineProcess{PID: p.PID, Name: cleanName(p.Name), Created: p.Birth, CPU: o.cpu, Memory: p.Memory, Agent: agent, Suspicious: o.ref != "", Ref: ref})
	}
	sort.Slice(h.Processes, func(i, j int) bool {
		a, b := h.Processes[i], h.Processes[j]
		if a.Suspicious != b.Suspicious {
			return a.Suspicious
		}
		return a.CPU > b.CPU
	})
	if len(h.Processes) > 20 {
		h.Processes = h.Processes[:20]
	}
	return h
}
func cleanName(s string) string {
	s = filepath.Base(s)
	s = strings.Map(func(r rune) rune {
		if r < ' ' || r == 127 {
			return -1
		}
		return r
	}, s)
	if len(s) > 100 {
		s = s[:100]
	}
	return s
}

// Terminate only accepts a current suspect reference belonging to this machine
// node's workspace AND owner. The OS adapter retains/revalidates process identity.
func (m *Manager) Terminate(ctx context.Context, workspace, member, ref string) (domain.MachineProcess, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.latest.Complete || (m.now().Sub(m.latest.At) < 0 || m.now().Sub(m.latest.At) > 2*time.Minute) || ref == "" {
		return domain.MachineProcess{}, ErrIdentity
	}
	for _, o := range m.previous {
		if o.ref != ref || o.scope == nil || o.scope.stopped || o.scope.owner.Workspace != workspace || o.scope.owner.Member != member {
			continue
		}
		p, err := m.lookup(o.process.PID)
		if err != nil || p.Birth != o.process.Birth || p.PID != o.process.PID {
			return domain.MachineProcess{}, ErrIdentity
		}
		if err = m.signal(ctx, p, Grace); err != nil {
			return domain.MachineProcess{}, err
		}
		o.ref = ""
		return domain.MachineProcess{PID: p.PID, Name: cleanName(p.Name), Created: p.Birth}, nil
	}
	return domain.MachineProcess{}, ErrIdentity
}

type usageSnapshot struct {
	CPU              float64
	Total, Available uint64
	Load             []float64
}

func sample(ctx context.Context) (Sample, error) {
	u, err := readUsage(ctx)
	if err != nil {
		return Sample{}, err
	}
	ps, err := readProcesses(ctx)
	if err != nil {
		return Sample{}, err
	}
	s := Sample{At: time.Now(), Complete: true, Cores: runtime.NumCPU(), LoadRatio: u.CPU / 100, LoadMetric: "cpu_busy", Total: u.Total, Available: u.Available, Processes: ps}
	if runtime.GOOS != "windows" {
		if len(u.Load) == 0 {
			return Sample{}, errors.New("load unavailable")
		}
		s.LoadRatio = u.Load[0] / float64(s.Cores)
		s.LoadMetric = "load1_per_core"
	}
	return s, nil
}

func (m *Manager) AgentFor(workspace, member, ref string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, o := range m.previous {
		if o.ref == ref && ref != "" && o.scope != nil && o.scope.owner.Workspace == workspace && o.scope.owner.Member == member {
			return o.scope.owner.Agent, nil
		}
	}
	return "", ErrIdentity
}
