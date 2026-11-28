package oscap

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"time"
)

var credentialLookupIP = net.DefaultResolver.LookupNetIP

func credentialPrivate(ip netip.Addr) bool {
	ip = ip.Unmap()
	return !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || netip.MustParsePrefix("100.64.0.0/10").Contains(ip)
}
func credentialResolve(ctx context.Context, host string) ([]netip.Addr, bool, error) {
	ips, err := credentialLookupIP(ctx, "ip", host)
	if err != nil || len(ips) == 0 {
		return nil, false, errf("network_error", "could not resolve the destination")
	}
	private := false
	for _, ip := range ips {
		if credentialPrivate(ip) {
			private = true
		}
	}
	return ips, private, nil
}

// CredentialPreview resolves an origin without sending HTTP or reading secrets.
func CredentialPreview(ctx context.Context, origin string) (bool, error) {
	origin, err := CredentialOrigin(origin)
	if err != nil {
		return false, err
	}
	u, _ := url.Parse(origin)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, private, err := credentialResolve(ctx, u.Hostname())
	return private, err
}
func credentialPinnedTransport(ctx context.Context, origin string, allowPrivate bool) (http.RoundTripper, error) {
	api, _ := url.Parse(origin)
	ips, private, err := credentialResolve(ctx, api.Hostname())
	if err != nil {
		return nil, err
	}
	if private && !allowPrivate {
		return nil, errf(CodePermission, "this server now resolves to a private network; disconnect and reconnect to review access")
	}
	t := http.DefaultTransport.(*http.Transport).Clone()
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
