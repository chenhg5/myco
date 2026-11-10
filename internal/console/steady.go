package console

import (
	"errors"
	"sync"
	"time"

	"github.com/chenhg5/myco/internal/client"
)

// steadyFor is how long a hub's last good answer stands in for a failed
// check. The App polls every 5 seconds; a slow hub (far away, behind a
// tunnel) misses one now and then, and showing it offline (its nodes and
// conversations gone) until the next check makes the window flicker.
const steadyFor = 20 * time.Second

type stamped[T any] struct {
	at time.Time
	v  T
}

// steady remembers each hub's last good status and overview.
type steady struct {
	mu     sync.Mutex
	status map[string]stamped[HubStatus]
	ov     map[string]stamped[HubOverview]
}

// transient reports whether err is the kind a later check may not see:
// the hub unreachable, slow or failing on its side. A refused token or a
// missing hub is an answer, not a hiccup, and shows right away.
func transient(err error) bool {
	if err == nil {
		return false
	}
	var ce *client.Error
	if errors.As(err, &ce) {
		return ce.Status == 0 || ce.Status >= 500 || ce.Retryable
	}
	return true
}

// keep returns cur when ok (and remembers it); after a transient failure,
// the last good value when it is recent enough; else cur.
func keep[T any](mu *sync.Mutex, m *map[string]stamped[T], id string, ok, flaky bool, cur T, now time.Time) T {
	mu.Lock()
	defer mu.Unlock()
	if *m == nil {
		*m = map[string]stamped[T]{}
	}
	if ok {
		(*m)[id] = stamped[T]{at: now, v: cur}
		return cur
	}
	if last, found := (*m)[id]; found && flaky && now.Sub(last.at) < steadyFor {
		return last.v
	}
	return cur
}

func (s *steady) hubStatus(cur HubStatus, err error, now time.Time) HubStatus {
	return keep(&s.mu, &s.status, cur.ID, cur.OK, transient(err), cur, now)
}

func (s *steady) hubOverview(cur HubOverview, err error, now time.Time) HubOverview {
	return keep(&s.mu, &s.ov, cur.ID, cur.OK, transient(err), cur, now)
}
