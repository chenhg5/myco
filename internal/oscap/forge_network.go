package oscap

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"time"
)

var forgeLookup = net.DefaultResolver.LookupNetIP

func forgePrivate(ip netip.Addr) bool {
	ip = ip.Unmap()
	return !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || netip.MustParsePrefix("100.64.0.0/10").Contains(ip)
}
func forgeResolve(ctx context.Context, host string) ([]netip.Addr, bool, error) {
	ips, err := forgeLookup(ctx, "ip", host)
	if err != nil || len(ips) == 0 {
		return nil, false, errf("network_error", "could not resolve the code host")
	}
	private := false
	for _, ip := range ips {
		if forgePrivate(ip) {
			private = true
		}
	}
	return ips, private, nil
}

// ForgePreview never sends HTTP or reads credentials. The trusted host uses it
// to describe the exact server and private-network risk before confirmation.
func ForgePreview(ctx context.Context, provider, host string) (string, bool, error) {
	origin, err := forgeHost(provider, host)
	if err != nil {
		return "", false, err
	}
	api, _ := url.Parse(forgeBase(forgeAccount{Provider: provider, Host: origin}))
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, private, err := forgeResolve(ctx, api.Hostname())
	return origin, private, err
}
func forgePinnedTransport(ctx context.Context, p forgeAccount) (http.RoundTripper, error) {
	api, _ := url.Parse(forgeBase(p))
	ips, private, err := forgeResolve(ctx, api.Hostname())
	if err != nil {
		return nil, err
	}
	if private && !p.AllowPrivate {
		return nil, errf(CodePermission, "this server now resolves to a private network; disconnect and reconnect to review access")
	}
	t := forgeNetworkTransport().(*http.Transport)
	// For direct connections use exactly the addresses checked above, avoiding a
	// second DNS lookup between policy and socket creation. A configured proxy
	// retains verified end-to-end TLS; its own address is the user's proxy setting.
	t.DialContext = func(callCtx context.Context, network, address string) (net.Conn, error) {
		host, port, e := net.SplitHostPort(address)
		if e != nil {
			return nil, e
		}
		dial := net.Dialer{Timeout: 10 * time.Second}
		if host != api.Hostname() {
			return dial.DialContext(callCtx, network, address)
		}
		var last error
		for _, ip := range ips {
			conn, e := dial.DialContext(callCtx, network, net.JoinHostPort(ip.String(), port))
			if e == nil {
				return conn, nil
			}
			last = e
		}
		return nil, last
	}
	return t, nil
}
