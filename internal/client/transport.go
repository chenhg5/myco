package client

import (
	"net/http"
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
var hubTransport = newTransport(pingAfter, pingTimeout)

// newTransport is the default transport (proxy from the environment,
// HTTP/2 when the hub's front offers it) with HTTP/2 health checks; zero
// durations leave them off.
func newTransport(pingAfter, pingTimeout time.Duration) *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = Proxy // (the environment's, unless SetProxy changed it)
	t.HTTP2 = &http.HTTP2Config{SendPingTimeout: pingAfter, PingTimeout: pingTimeout}
	return t
}
