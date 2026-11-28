package coordeval

import (
	"context"
	"sync"
	"time"
)

// scenarioClock excludes host scheduling and HTTP/SQLite time. It moves to
// the next timer only when every registered participant has yielded. A
// runnable participant (including one awaiting HTTP) holds scenario time.
// The caller's context remains a separate wall-clock watchdog for deadlocks.
type scenarioClock struct {
	mu     sync.Mutex
	now    time.Time
	actors map[*scenarioActor]bool
}
type scenarioActor struct {
	clock   *scenarioClock
	waiting bool
	due     time.Time
	wake    chan struct{}
}
type scenarioClockKey struct{}
type scenarioActorKey struct{}

func newScenarioClock() *scenarioClock {
	return &scenarioClock{now: time.Now().UTC(), actors: map[*scenarioActor]bool{}}
}
func (c *scenarioClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.now }

func scenarioActorContext(ctx context.Context) (context.Context, func()) {
	c, _ := ctx.Value(scenarioClockKey{}).(*scenarioClock)
	if c == nil {
		return ctx, func() {}
	}
	a := &scenarioActor{clock: c}
	c.mu.Lock()
	c.actors[a] = true
	c.mu.Unlock()
	return context.WithValue(ctx, scenarioActorKey{}, a), func() { c.mu.Lock(); delete(c.actors, a); c.advance(); c.mu.Unlock() }
}
func goScenario(ctx context.Context, fn func(context.Context)) {
	child, done := scenarioActorContext(ctx)
	go func() { defer done(); fn(child) }()
}

// advance is called with mu held. Wakees become runnable before releasing
// the lock, so a slow host cannot advance past their pending work.
func (c *scenarioClock) advance() {
	var next time.Time
	for a := range c.actors {
		if !a.waiting {
			return
		}
		if !a.due.IsZero() && (next.IsZero() || a.due.Before(next)) {
			next = a.due
		}
	}
	if next.IsZero() {
		return
	}
	c.now = next
	for a := range c.actors {
		if a.waiting && !a.due.IsZero() && !a.due.After(next) {
			a.waiting = false
			a.due = time.Time{}
			close(a.wake)
		}
	}
}
func (a *scenarioActor) sleep(ctx context.Context, d time.Duration) bool {
	if ctx.Err() != nil {
		return false
	}
	c := a.clock
	c.mu.Lock()
	a.waiting = true
	a.due = c.now.Add(max(d, time.Nanosecond))
	a.wake = make(chan struct{})
	wake := a.wake
	c.advance()
	c.mu.Unlock()
	select {
	case <-ctx.Done():
	case <-wake:
	}
	c.mu.Lock()
	a.waiting = false
	a.due = time.Time{}
	c.mu.Unlock()
	return ctx.Err() == nil
}

// A virtual poll must not block on the Hub's real-time long-poll timer.
func pollDuration(ctx context.Context, d time.Duration) time.Duration {
	if ctx.Value(scenarioClockKey{}) != nil {
		return 0
	}
	return d
}

// scenarioJoin yields a parent while its children work. The last child
// makes the parent runnable before signalling it: an ordinary WaitGroup
// would leave a scheduling gap in which the clock could pass its deadline.
type scenarioJoin struct {
	mu     *sync.Mutex
	parent *scenarioActor
	left   int
	ready  chan struct{}
}

func newScenarioJoin(ctx context.Context, n int) *scenarioJoin {
	a, _ := ctx.Value(scenarioActorKey{}).(*scenarioActor)
	j := &scenarioJoin{mu: &sync.Mutex{}, parent: a, left: n, ready: make(chan struct{})}
	if a != nil {
		j.mu = &a.clock.mu
	}
	if n == 0 {
		close(j.ready)
	}
	return j
}
func (j *scenarioJoin) done() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.left--
	if j.left == 0 {
		if j.parent != nil {
			j.parent.waiting = false
		}
		close(j.ready)
	}
}
func (j *scenarioJoin) wait() {
	j.mu.Lock()
	if j.left > 0 && j.parent != nil {
		j.parent.waiting = true
		j.parent.due = time.Time{}
		j.parent.clock.advance()
	}
	j.mu.Unlock()
	<-j.ready
}
