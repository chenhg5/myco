package app

import (
	"sync"
	"time"

	"github.com/chenhg5/myco/internal/domain"
)

// Presence tracks, in memory, how each agent is currently receiving messages.
//
// An agent can receive in two ways: a session keeps `myco inbox wait` running
// (it is "live" and reads its own inbox), or a runner pulls messages and wakes
// the session for each one. While a session is live, runner pulls get nothing,
// so the two never compete; the runner takes over again a grace period after
// the session's last wait ends.
type Presence struct {
	clock Clock
	grace time.Duration // a live session stays live this long after its last wait
	busy  time.Duration // how long a runner's "session in use" report counts

	mu        sync.Mutex
	listeners map[domain.AgentName]*listener
	runner    map[domain.AgentName]time.Time // last runner pull
	inUse     map[domain.AgentName]time.Time // last "session in use" release
}

type listener struct {
	active int
	last   time.Time
}

// NewPresence returns a tracker; grace is how long a session counts as live
// after its last `inbox wait` returned.
func NewPresence(clock Clock, grace time.Duration) *Presence {
	return &Presence{clock: clock, grace: grace, busy: 5 * time.Minute,
		listeners: map[domain.AgentName]*listener{}, runner: map[domain.AgentName]time.Time{}, inUse: map[domain.AgentName]time.Time{}}
}

// Listen marks agent as live until the returned func is called.
func (p *Presence) Listen(agent domain.AgentName) (done func()) {
	p.mu.Lock()
	l := p.listeners[agent]
	if l == nil {
		l = &listener{}
		p.listeners[agent] = l
	}
	l.active++
	l.last = p.clock.Now()
	p.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			p.mu.Lock()
			l.active--
			l.last = p.clock.Now()
			p.mu.Unlock()
		})
	}
}

// Live reports whether a session of agent is receiving through `inbox wait`.
func (p *Presence) Live(agent domain.AgentName) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.liveLocked(agent, p.clock.Now())
}

func (p *Presence) liveLocked(agent domain.AgentName, now time.Time) bool {
	l := p.listeners[agent]
	return l != nil && (l.active > 0 || now.Sub(l.last) < p.grace)
}

// RunnerPulled records a pull by agent's runner.
func (p *Presence) RunnerPulled(agent domain.AgentName) {
	p.mu.Lock()
	p.runner[agent] = p.clock.Now()
	p.mu.Unlock()
}

// SessionInUse records that agent's runner found its session in use by
// someone else (e.g. open in a terminal) and handed a message back.
func (p *Presence) SessionInUse(agent domain.AgentName) {
	p.mu.Lock()
	p.inUse[agent] = p.clock.Now()
	p.mu.Unlock()
}

// ReceivingSession and ReceivingRunner name how an agent receives messages.
const (
	ReceivingSession = "session"
	ReceivingRunner  = "runner"
)

// State reports how agent receives right now ("session", "runner" or "" when
// nothing has pulled within window) and whether its runner recently found
// the session in use.
func (p *Presence) State(agent domain.AgentName, window time.Duration) (receiving string, inUse bool) {
	if p == nil {
		return "", false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.clock.Now()
	switch {
	case p.liveLocked(agent, now):
		return ReceivingSession, false
	case !p.runner[agent].IsZero() && now.Sub(p.runner[agent]) <= window:
		receiving = ReceivingRunner
	}
	t := p.inUse[agent]
	return receiving, !t.IsZero() && now.Sub(t) < p.busy
}
