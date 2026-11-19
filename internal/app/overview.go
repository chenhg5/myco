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
	StateWorking AgentState = "working" // has acked messages that show life
	StatePending AgentState = "pending" // has queued, delivered, or held messages
	StateIdle    AgentState = "idle"    // online, nothing to do
	StateOffline AgentState = "offline" // not seen within the online window
)

// AgentOverview is one agent with its live counters.
type AgentOverview struct {
	Agent  *domain.Agent
	State  AgentState
	Online bool
	Queued int
	// InProgress counts acked messages that show life: the agent is online
	// and one of them was worked on lately (see staleAfter).
	InProgress int
	// Stale counts acked messages that do not: the agent is away, or none
	// was touched for a while (left unclosed, or its runner died). They
	// stay open, but the agent does not count as working on them.
	Stale    int
	StaleIDs []string
	Held     int
	// Asking counts held tasks waiting on a question to their sender (they
	// are in Held too).
	Asking int
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
	presence     *Presence
}

// How long an acked task may go without a sign of life before it no
// longer counts as being worked on. A runner reads its task every few
// seconds while it runs; a session shows life when it reports progress,
// reads the task or uploads its trace, which can be far apart.
const (
	StaleRunner  = 5 * time.Minute
	StaleSession = 30 * time.Minute
)

// SetPresence lets the overview tell runners (quick to show life) from
// sessions.
func (s *OverviewService) SetPresence(p *Presence) { s.presence = p }

func (s *OverviewService) staleAfter(a *domain.Agent) time.Duration {
	if s.presence != nil {
		if r, _ := s.presence.State(a.Name, s.onlineWindow); r == ReceivingRunner {
			return StaleRunner
		}
	}
	return StaleSession
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
	acts, err := s.messages.AckedActivity(ctx)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	out := make(map[domain.AgentName]AgentOverview, len(agents))
	for _, a := range agents {
		out[a.Name] = s.stateOf(a, counts[a.Name], acts[a.Name], now)
	}
	return out, nil
}

func (s *OverviewService) stateOf(a *domain.Agent, c map[domain.Status]int, acts []domain.Activity, now time.Time) AgentOverview {
	o := AgentOverview{
		Agent: a, Online: a.IsOnline(now, s.onlineWindow),
		Queued: c[domain.StatusQueued] + c[domain.StatusDelivered], InProgress: c[domain.StatusAcked], Held: c[domain.StatusHeld] + c[domain.CountAsking],
		Asking: c[domain.CountAsking],
	}
	if len(acts) > 0 {
		limit := s.staleAfter(a)
		o.InProgress = 0
		for _, t := range acts {
			if o.Online && now.Sub(t.At) < limit {
				o.InProgress++
			} else {
				o.StaleIDs = append(o.StaleIDs, t.ID)
			}
		}
		o.Stale = len(o.StaleIDs)
	}
	switch {
	case o.InProgress > 0:
		o.State = StateWorking
	case o.Queued+o.Held > 0:
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
	acts, err := s.messages.AckedActivity(ctx)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	ov := &Overview{GeneratedAt: now}
	for _, a := range agents {
		o := s.stateOf(a, counts[a.Name], acts[a.Name], now)
		if (viewer == nil || viewer.Name == a.Name) && o.InProgress+o.Stale+o.Held+o.Queued > 0 {
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
