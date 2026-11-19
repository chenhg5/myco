package client

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// A hub on this very computer, joined at its LAN address
// (http://192.168.1.20:7777, as links made there say), stops answering
// there when the computer moves to another network: the address is no
// longer its own, and a proxy in between may answer 503 for it. The hub is
// still right here, on loopback. So when a request to a private address
// fails, the client asks 127.0.0.1 on the same port whether it knows this
// token; if it does, it is the same hub (a token belongs to one hub), and
// requests go there from then on, without a proxy. Requests without a
// token (enrolling) are never redirected: nothing proves which hub it is.

// loopbackTransport reaches this computer's own hub (never through a proxy).
var loopbackTransport = func() *http.Transport {
	t := newTransport(0, 0)
	t.Proxy = nil
	return t
}()

// lanHub is the transport for a hub URL with a private IP and a port.
type lanHub struct {
	base  http.RoundTripper
	host  string // "192.168.1.20:7777"
	local string // "127.0.0.1:7777"
	path  string // the hub's base path ("/w/team" or "")

	mu  sync.Mutex
	via bool // requests go to local

	// probe asks local whether it knows the token (tests replace it).
	probe func(ctx context.Context, scheme, local, path, auth string) bool
}

// forLANHub wraps base when u is a hub at a private IPv4 address and port.
func forLANHub(u *url.URL, base http.RoundTripper) http.RoundTripper {
	ip := net.ParseIP(u.Hostname())
	if ip == nil || ip.To4() == nil || !ip.IsPrivate() || u.Port() == "" {
		return base
	}
	return &lanHub{base: base, host: u.Host, local: net.JoinHostPort("127.0.0.1", u.Port()),
		path: strings.TrimRight(u.Path, "/"), probe: knowsToken}
}

func (t *lanHub) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host != t.host {
		return t.base.RoundTrip(req)
	}
	t.mu.Lock()
	via := t.via
	t.mu.Unlock()
	if via {
		res, err := loopbackTransport.RoundTrip(t.toLocal(req))
		if err == nil {
			return res, nil
		}
		// Nothing here any more: the address itself again.
		t.mu.Lock()
		t.via = false
		t.mu.Unlock()
		if req, err = again(req); err != nil {
			return nil, err
		}
	}
	res, err := t.base.RoundTrip(req)
	if err == nil && res.StatusCode != http.StatusBadGateway && res.StatusCode != http.StatusServiceUnavailable &&
		res.StatusCode != http.StatusGatewayTimeout {
		return res, nil
	}
	if req.Context().Err() != nil {
		return res, err
	}
	auth := req.Header.Get("Authorization")
	if auth == "" || !t.probe(req.Context(), req.URL.Scheme, t.local, t.path, auth) {
		return res, err
	}
	retry, rerr := again(req)
	if rerr != nil {
		return res, err
	}
	t.mu.Lock()
	t.via = true
	t.mu.Unlock()
	if res != nil {
		res.Body.Close()
	}
	return loopbackTransport.RoundTrip(t.toLocal(retry))
}

// toLocal is req sent to this computer's hub.
func (t *lanHub) toLocal(req *http.Request) *http.Request {
	r := req.Clone(req.Context())
	r.URL.Host, r.Host = t.local, ""
	return r
}

// again is req with its body anew, for sending it a second time.
func again(req *http.Request) (*http.Request, error) {
	r := req.Clone(req.Context())
	if req.Body != nil && req.Body != http.NoBody {
		if req.GetBody == nil {
			return nil, http.ErrBodyReadAfterClose
		}
		b, err := req.GetBody()
		if err != nil {
			return nil, err
		}
		r.Body = b
	}
	return r, nil
}

// knowsToken reports whether the hub at local accepts auth.
func knowsToken(ctx context.Context, scheme, local, path, auth string) bool {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, scheme+"://"+local+path+"/v1/me", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", auth)
	res, err := loopbackTransport.RoundTrip(req)
	if err != nil {
		return false
	}
	res.Body.Close()
	return res.StatusCode == http.StatusOK
}
