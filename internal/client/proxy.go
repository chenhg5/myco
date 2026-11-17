package client

import (
	"net/http"
	"net/url"
	"sync/atomic"
)

// How this process reaches out: by default through the proxy in the
// environment (HTTPS_PROXY and friends), as Go does. The App lets people
// set one (Settings › Network): SetProxy swaps it for every hub client at
// once, and for http.DefaultTransport when Install was called.

type proxyFunc = func(*http.Request) (*url.URL, error)

var proxyNow atomic.Pointer[proxyFunc]

func init() {
	f := proxyFunc(http.ProxyFromEnvironment)
	proxyNow.Store(&f)
}

// Proxy picks the proxy for a request by the current setting.
func Proxy(r *http.Request) (*url.URL, error) { return (*proxyNow.Load())(r) }

// SetProxy changes how requests reach out from now on (nil: from the
// environment again). Connections already open are closed when idle, so
// the next requests take the new way.
func SetProxy(f func(*http.Request) (*url.URL, error)) {
	if f == nil {
		f = http.ProxyFromEnvironment
	}
	pf := proxyFunc(f)
	proxyNow.Store(&pf)
	hubTransport.CloseIdleConnections()
	if dt, ok := http.DefaultTransport.(*http.Transport); ok {
		dt.CloseIdleConnections()
	}
}

// InstallProxyOnDefault makes http.DefaultTransport (and so every
// http.Client without a transport of its own) follow SetProxy too.
func InstallProxyOnDefault() {
	if dt, ok := http.DefaultTransport.(*http.Transport); ok {
		dt.Proxy = Proxy
	}
}
