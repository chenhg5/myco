package app

import (
	"context"
	"sort"
	"time"

	"github.com/chenhg5/myco/internal/domain"
)

// AgentState summarizes what an agent is doing right now.
type AgentState string

const (
	StateWorking AgentState = "working" // has acked/held/delivered messages
	StatePending AgentState = "pending" // has queued messages, none in progress
	StateIdle    AgentState = "idle"    // online, nothing to do
	StateOffline AgentState = "offline" // not seen within the online window
)

// AgentOverview is one agent with its live counters.
type AgentOverview struct {
	Agent      *domain.Agent
	State      AgentState
	Online     bool
	Queued     int
	InProgress int // delivered + acked
	Held       int
	// Current lists open messages; only filled when the viewer may see them
	// (admin, or the agent itself).
	Current []*domain.Message
}

// Overview is the network snapshot for dashboards.
type Overview struct {
	GeneratedAt time.Time
	Agents      []AgentOverview
	// Recent messages: all (admin) or those involving the viewer.
	Recent []*domain.Message
}

// OverviewService builds dashboard snapshots.
type OverviewService struct {
	agents       domain.AgentRepository
	messages     domain.MessageRepository
	clock        Clock
	onlineWindow time.Duration
}

// NewOverviewService wires the overview use case.
func NewOverviewService(agents domain.AgentRepository, messages domain.MessageRepository, clock Clock, onlineWindow time.Duration) *OverviewService {
	return &OverviewService{agents: agents, messages: messages, clock: clock, onlineWindow: onlineWindow}
}

// States returns every agent's live state and counters, without message
// contents (for agent listings).
func (s *OverviewService) States(ctx context.Context) (map[domain.AgentName]AgentOverview, error) {
	agents, err := s.agents.List(ctx)
	if err != nil {
		return nil, err
	}
	counts, err := s.messages.OpenCounts(ctx)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	out := make(map[domain.AgentName]AgentOverview, len(agents))
	for _, a := range agents {
		out[a.Name] = s.stateOf(a, counts[a.Name], now)
	}
	return out, nil
}

func (s *OverviewService) stateOf(a *domain.Agent, c map[domain.Status]int, now time.Time) AgentOverview {
	o := AgentOverview{
		Agent: a, Online: a.IsOnline(now, s.onlineWindow),
		Queued: c[domain.StatusQueued], InProgress: c[domain.StatusDelivered] + c[domain.StatusAcked], Held: c[domain.StatusHeld],
	}
	switch {
	case o.InProgress+o.Held > 0:
		o.State = StateWorking
	case o.Queued > 0:
		o.State = StatePending
	case o.Online:
		o.State = StateIdle
	default:
		o.State = StateOffline
	}
	return o
}

// Snapshot returns the overview as seen by viewer (nil viewer = admin).
// Non-admin viewers see every agent's profile and counters, but message
// contents only for messages they are part of.
func (s *OverviewService) Snapshot(ctx context.Context, viewer *domain.Agent, recentLimit int) (*Overview, error) {
	agents, err := s.agents.List(ctx)
	if err != nil {
		return nil, err
	}
	counts, err := s.messages.OpenCounts(ctx)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	ov := &Overview{GeneratedAt: now}
	for _, a := range agents {
		o := s.stateOf(a, counts[a.Name], now)
		if (viewer == nil || viewer.Name == a.Name) && o.InProgress+o.Held > 0 {
			if o.Current, err = s.messages.ListOpen(ctx, a.Name, 10); err != nil {
				return nil, err
			}
		}
		ov.Agents = append(ov.Agents, o)
	}
	order := map[AgentState]int{StateWorking: 0, StatePending: 1, StateIdle: 2, StateOffline: 3}
	sort.SliceStable(ov.Agents, func(i, j int) bool {
		if oi, oj := order[ov.Agents[i].State], order[ov.Agents[j].State]; oi != oj {
			return oi < oj
		}
		return ov.Agents[i].Agent.Name < ov.Agents[j].Agent.Name
	})
	var who domain.AgentName
	if viewer != nil {
		who = viewer.Name
	}
	if ov.Recent, err = s.messages.ListRecent(ctx, who, recentLimit); err != nil {
		return nil, err
	}
	return ov, nil
}
