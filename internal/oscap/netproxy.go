package oscap

import (
	"net/http"
	"net/url"

	"github.com/chenhg5/myco/internal/sysproxy"
)

// Downloads (speech.install) go through the proxy the App or the
// environment gives, and otherwise through the one set in the system's
// network settings (macOS, Windows; sysproxy): a GUI app's environment has
// none, and in some places GitHub is reachable only through it. If that
// proxy is not there, they go direct.

// downloadClients: the ways to try, in order.
func downloadClients() []*http.Client {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return []*http.Client{http.DefaultClient}
	}
	if sysproxy.Current().Proxy == nil {
		return []*http.Client{http.DefaultClient}
	}
	viaSys := base.Clone()
	viaSys.Proxy = func(r *http.Request) (*url.URL, error) {
		if base.Proxy != nil {
			if u, err := base.Proxy(r); u != nil || err != nil {
				return u, err
			}
		}
		return sysproxy.For(r.URL.Hostname()), nil
	}
	return []*http.Client{{Transport: viaSys}, http.DefaultClient}
}
