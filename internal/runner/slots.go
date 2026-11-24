package runner

import (
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/myco-ai/myco/internal/api"
)

// SlotPool is one process's work-conserving, workspace-fair CLI scheduler.
// Running turns are never preempted. A workspace with no running turn is
// served first; otherwise admissions rotate between waiting workspaces.
// A workspace's own waiting turns remain FIFO.
type SlotPool struct {
	mu        sync.Mutex
	max, used int
	active    map[string]int
	queue     []*SlotTicket
	last      string
	watchers  map[chan struct{}]struct{}
}

type SlotTicket struct {
	pool                      *SlotPool
	workspace, owner, message string
	Ready                     chan struct{}
	granted, ended            bool
}

// SlotOwner namespaces agent names: independent workspaces may reuse them.
func SlotOwner(workspace, name string) string { return WorkspaceKey(workspace) + "\x00" + name }

func NewSlotPool(n int) *SlotPool {
	if n < 1 {
		n = 1
	}
	return &SlotPool{max: n, active: map[string]int{}, watchers: map[chan struct{}]struct{}{}}
}

// WorkspaceKey preserves the workspace URL path, while normalizing harmless
// trailing slashes and host casing. Never expose these keys in runtime reports.
func WorkspaceKey(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return strings.TrimRight(raw, "/")
	}
	u.Host = strings.ToLower(u.Host)
	u.Scheme = strings.ToLower(u.Scheme)
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawQuery, u.Fragment, u.User = "", "", nil
	return u.String()
}

func (p *SlotPool) Request(workspace, owner, message string) *SlotTicket {
	p.mu.Lock()
	defer p.mu.Unlock()
	t := &SlotTicket{pool: p, workspace: WorkspaceKey(workspace), owner: owner, message: message, Ready: make(chan struct{})}
	p.queue = append(p.queue, t)
	p.dispatch()
	p.changed()
	return t
}

// Done cancels a waiter or returns an acquired slot exactly once, including
// the race where cancellation and admission happen together.
func (t *SlotTicket) Done() {
	p := t.pool
	p.mu.Lock()
	defer p.mu.Unlock()
	if t.ended {
		return
	}
	t.ended = true
	if t.granted {
		p.used--
		p.active[t.workspace]--
	} else {
		for i, q := range p.queue {
			if q == t {
				p.queue = append(p.queue[:i], p.queue[i+1:]...)
				break
			}
		}
	}
	p.dispatch()
	p.changed()
}

// next chooses a workspace in round-robin order among those waiting. The
// zero-active priority gives a newly waiting workspace the next free slot.
func nextSlot(q []*SlotTicket, active map[string]int, last string) int {
	order := []string{}
	seen := map[string]bool{}
	for _, t := range q {
		if !seen[t.workspace] {
			seen[t.workspace] = true
			order = append(order, t.workspace)
		}
	}
	start := 0
	for i, w := range order {
		if w == last {
			start = (i + 1) % len(order)
			break
		}
	}
	chosen := ""
	for i := 0; i < len(order); i++ {
		w := order[(start+i)%len(order)]
		if active[w] == 0 {
			chosen = w
			break
		}
	}
	if chosen == "" {
		chosen = order[start]
	}
	for i, t := range q {
		if t.workspace == chosen {
			return i
		}
	}
	panic("empty slot queue")
}

func (p *SlotPool) dispatch() {
	for p.used < p.max && len(p.queue) > 0 {
		i := nextSlot(p.queue, p.active, p.last)
		t := p.queue[i]
		p.queue = append(p.queue[:i], p.queue[i+1:]...)
		p.last = t.workspace
		p.used++
		p.active[t.workspace]++
		t.granted = true
		close(t.Ready)
	}
}

// Snapshot exposes machine totals and only this agent's waiting message IDs.
// Queue positions simulate this same scheduler, not raw arrival order.
func (p *SlotPool) Snapshot(owner string) *api.RunnerSlots {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := &api.RunnerSlots{MaxConcurrent: p.max, InUse: p.used, Waiting: len(p.queue), At: time.Now()}
	q := append([]*SlotTicket(nil), p.queue...)
	active := map[string]int{}
	for w, n := range p.active {
		active[w] = n
	}
	last := p.last
	for pos := 1; len(q) > 0; pos++ {
		i := nextSlot(q, active, last)
		t := q[i]
		q = append(q[:i], q[i+1:]...)
		if t.owner == owner && len(s.Queue) < 256 {
			s.Queue = append(s.Queue, api.RunnerQueuedTask{MessageID: t.message, Position: pos})
		}
		active[t.workspace]++
		last = t.workspace
	}
	return s
}

func (p *SlotPool) Subscribe() (<-chan struct{}, func()) {
	p.mu.Lock()
	defer p.mu.Unlock()
	c := make(chan struct{}, 1)
	p.watchers[c] = struct{}{}
	return c, func() { p.mu.Lock(); delete(p.watchers, c); p.mu.Unlock() }
}
func (p *SlotPool) changed() {
	for c := range p.watchers {
		select {
		case c <- struct{}{}:
		default:
		}
	}
}
