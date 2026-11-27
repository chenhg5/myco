package client

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// How a dead connection to the hub is noticed. Runners and waiting sessions
// keep long-polls open for up to a minute, and against an HTTPS hub they all
// share one HTTP/2 connection. When that connection dies without a word (the
// computer slept, the network changed, a NAT entry expired), nothing arrives
// to say so: without a health check every request on it waits out its own
// bound (the long-poll's wait plus a minute) before the client reconnects.
// With these, a connection that has been silent for pingAfter is pinged and
// closed if the ping is not answered within pingTimeout, so its requests
// fail (retryably) and are sent again on a new connection within half a
// minute.
const (
	pingAfter   = 15 * time.Second
	pingTimeout = 15 * time.Second
)

// hubTransport is shared by every Client, so they pool connections to a hub
// as they did on http.DefaultTransport.
var hubTransport = newRecoveringTransport(func() *http.Transport {
	return newTransport(pingAfter, pingTimeout)
})

// Five minutes also covers consecutive two-minute calls from a single waiter.
const (
	noResponseWindow  = 5 * time.Minute
	transportCooldown = 30 * time.Second
)

// A caller cancelling a poll is normal. Only expiry of our own request bound
// counts as evidence of a transport that answers pings but not requests.
type boundParentKey struct{}

func boundedContext(parent context.Context, bound time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(parent, bound)
	return context.WithValue(ctx, boundParentKey{}, parent), cancel
}

type timeoutWindow struct {
	first time.Time
	count int
}

// All clients keep this stable RoundTripper, including inside LAN wrappers.
// Replacing its pool affects only subsequent RoundTrips; an in-flight call
// retains its original transport and is never cancelled by the replacement.
type recoveringTransport struct {
	mu               sync.Mutex
	current          *http.Transport
	fresh            func() *http.Transport
	now              func() time.Time
	window, cooldown time.Duration
	lastSwap         time.Time
	timeouts         map[string]timeoutWindow
}

func newRecoveringTransport(fresh func() *http.Transport) *recoveringTransport {
	return &recoveringTransport{current: fresh(), fresh: fresh, now: time.Now,
		window: noResponseWindow, cooldown: transportCooldown,
		timeouts: make(map[string]timeoutWindow)}
}

func (t *recoveringTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	used := t.current
	t.mu.Unlock()
	resp, err := used.RoundTrip(req)
	parent, bounded := req.Context().Value(boundParentKey{}).(context.Context)
	if resp == nil && err != nil && bounded && parent.Err() == nil &&
		errors.Is(req.Context().Err(), context.DeadlineExceeded) {
		t.noResponse(used, req.URL.Scheme+"://"+req.URL.Host)
	}
	return resp, err
}

func (t *recoveringTransport) noResponse(used *http.Transport, hub string) {
	t.mu.Lock()
	now := t.now()
	// Old requests may time out together after a swap. They must not vote
	// against the new pool. Nor may unrelated hubs combine their failures.
	if used != t.current {
		t.mu.Unlock()
		return
	}
	for host, w := range t.timeouts {
		if now.Sub(w.first) > t.window {
			delete(t.timeouts, host)
		}
	}
	w := t.timeouts[hub]
	if w.count == 0 {
		w.first = now
	}
	w.count++
	t.timeouts[hub] = w
	if w.count < 2 || (!t.lastSwap.IsZero() && now.Sub(t.lastSwap) < t.cooldown) {
		t.mu.Unlock()
		return
	}
	t.current = t.fresh()
	t.lastSwap = now
	clear(t.timeouts)
	t.mu.Unlock()
	used.CloseIdleConnections()
	// Do not include a URL, token, request body or transport error here.
	slog.Warn("replaced shared hub HTTP transport after repeated no-response timeouts")
}

func (t *recoveringTransport) CloseIdleConnections() {
	t.mu.Lock()
	current := t.current
	t.mu.Unlock()
	current.CloseIdleConnections()
}

// newTransport is the default transport (proxy from the environment,
// HTTP/2 when the hub's front offers it) with HTTP/2 health checks; zero
// durations leave them off.
func newTransport(pingAfter, pingTimeout time.Duration) *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = Proxy // (the environment's, unless SetProxy changed it)
	t.HTTP2 = &http.HTTP2Config{SendPingTimeout: pingAfter, PingTimeout: pingTimeout}
	return t
}
