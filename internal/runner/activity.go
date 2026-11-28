package runner

import (
	"context"
	"sync"
)

// Activity makes idle detection and stopping new inbox claims one operation.
// Poll responses reserve work before dropping their poll lease, so a response
// racing with quiescence cannot leave an unaccounted claimed message.
type Activity struct {
	mu      sync.Mutex
	work    int
	polls   map[*pollLease]context.CancelFunc
	pending map[string]int
	paused  bool
	resume  chan struct{}
}
type pollLease struct{ token byte }

func (a *Activity) Work(ctx context.Context) (func(), bool) {
	if a == nil {
		return func() {}, ctx.Err() == nil
	}
	for {
		if ctx.Err() != nil {
			return nil, false
		}
		a.mu.Lock()
		if !a.paused {
			a.work++
			a.mu.Unlock()
			return a.Done, true
		}
		ch := a.resume
		a.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, false
		case <-ch:
		}
	}
}
func (a *Activity) Poll(ctx context.Context) (context.Context, func(int), bool) {
	if a == nil {
		return ctx, func(int) {}, ctx.Err() == nil
	}
	for {
		a.mu.Lock()
		if !a.paused {
			if a.polls == nil {
				a.polls = map[*pollLease]context.CancelFunc{}
			}
			p := &pollLease{}
			c, cancel := context.WithCancel(ctx)
			a.polls[p] = cancel
			a.mu.Unlock()
			return c, func(n int) { a.mu.Lock(); a.work += n; delete(a.polls, p); a.mu.Unlock(); cancel() }, true
		}
		ch := a.resume
		a.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, nil, false
		case <-ch:
		}
	}
}
func (a *Activity) Done() {
	if a != nil {
		a.mu.Lock()
		a.work--
		a.mu.Unlock()
	}
}

// PauseIfIdle leaves existing work alone. Only empty long polls are canceled.
func (a *Activity) PauseIfIdle() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.work != 0 || a.hasPending() {
		return false
	}
	if !a.paused {
		a.paused = true
		a.resume = make(chan struct{})
		for _, cancel := range a.polls {
			cancel()
		}
	}
	return true
}
func (a *Activity) Quiescent() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.paused && a.work == 0 && !a.hasPending() && len(a.polls) == 0
}
func (a *Activity) Resume() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.paused {
		a.paused = false
		close(a.resume)
	}
}

// Pending includes results still owed to the Hub, even after the harness exits.
func (a *Activity) Pending(owner string, n int) {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.pending == nil {
		a.pending = map[string]int{}
	}
	if n == 0 {
		delete(a.pending, owner)
	} else {
		a.pending[owner] = n
	}
}
func (a *Activity) hasPending() bool {
	for _, n := range a.pending {
		if n > 0 {
			return true
		}
	}
	return false
}
