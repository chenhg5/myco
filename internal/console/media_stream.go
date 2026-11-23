package console

import (
	"context"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"

	"github.com/chenhg5/myco/internal/client"
)

// A page cannot load media from the internet (its CSP allows only its own
// origin): __myco/stream?url= plays a public audio or video file through the
// App instead, as it is (seeking with Range, read as it plays, never held in
// memory). Only public addresses are reached, redirects included, so a page
// cannot use it to look into this computer's network, and only audio and
// video come back.

// streamPublicOnly: tests turn it off to stream from a local server.
var streamPublicOnly = true

// streamTypes are the answers passed on; podcast hosts often call audio
// octet-stream.
func streamType(ct string) bool {
	mt, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return false
	}
	return strings.HasPrefix(mt, "audio/") || strings.HasPrefix(mt, "video/") ||
		mt == "application/octet-stream" || mt == "binary/octet-stream" ||
		mt == "application/vnd.apple.mpegurl" || mt == "application/x-mpegurl" || mt == "application/ogg"
}

// publicIP reports whether ip is on the public internet.
func publicIP(ip net.IP) bool {
	return !(ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() || ip.IsInterfaceLocalMulticast() ||
		// 100.64.0.0/10 (carrier-grade NAT, Tailscale)
		(ip.To4() != nil && ip.To4()[0] == 100 && ip.To4()[1]&0xc0 == 64))
}

var errNotPublic = errors.New("not a public address")

// allowedProxies are the proxies the App sends requests through (its
// setting, or the environment's): dialling one of those is fine, though it
// may be on this computer; the media's own host is checked by name instead.
type proxiesKey struct{}

func proxiesFor(u *url.URL) map[string]bool {
	out := map[string]bool{}
	for _, scheme := range []string{"http", "https"} {
		v := *u
		v.Scheme = scheme
		if p, err := client.Proxy(&http.Request{URL: &v}); err == nil && p != nil {
			port := p.Port()
			if port == "" {
				port = map[string]string{"http": "80", "https": "443", "socks5": "1080"}[p.Scheme]
			}
			out[net.JoinHostPort(p.Hostname(), port)] = true
		}
	}
	return out
}

// checkHost resolves a media host and refuses it unless every address is
// public (a proxy resolves names on its side, so the dial check alone would
// not see them).
func checkHost(ctx context.Context, host string) error {
	if !streamPublicOnly {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil {
		if !publicIP(ip) {
			return errNotPublic
		}
		return nil
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return errNotPublic
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return err
	}
	for _, a := range addrs {
		if !publicIP(a.IP) {
			return errNotPublic
		}
	}
	return nil
}

var streamDialer = &net.Dialer{Timeout: 15 * time.Second}

var streamClient = &http.Client{
	Transport: &http.Transport{
		Proxy: client.Proxy,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			if streamPublicOnly {
				proxies, _ := ctx.Value(proxiesKey{}).(map[string]bool)
				if !proxies[address] {
					// Checked on the address actually dialled, after DNS.
					d := *streamDialer
					d.Control = func(_, addr string, _ syscall.RawConn) error {
						h, _, err := net.SplitHostPort(addr)
						if ip := net.ParseIP(h); err != nil || ip == nil || !publicIP(ip) {
							return errNotPublic
						}
						return nil
					}
					return d.DialContext(ctx, network, address)
				}
			}
			return streamDialer.DialContext(ctx, network, address)
		},
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		ForceAttemptHTTP2:     true,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 8 {
			return errors.New("too many redirects")
		}
		if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
			return errors.New("redirected away from http(s)")
		}
		return checkHost(req.Context(), req.URL.Hostname())
	},
}

// streamTarget checks the address a page asks for.
func streamTarget(raw string) (*url.URL, bool) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || len(raw) > 4096 {
		return nil, false
	}
	return u, true
}

func (b *Backend) serveStream(w http.ResponseWriter, r *http.Request) {
	u, ok := streamTarget(r.URL.Query().Get("url"))
	if !ok {
		http.Error(w, "only public http(s) addresses can be streamed", http.StatusBadRequest)
		return
	}
	if err := checkHost(r.Context(), u.Hostname()); err != nil {
		http.Error(w, "only public addresses can be streamed", http.StatusForbidden)
		return
	}
	ctx := context.WithValue(r.Context(), proxiesKey{}, proxiesFor(u))
	req, err := http.NewRequestWithContext(ctx, r.Method, u.String(), nil)
	if err != nil {
		http.Error(w, "bad address", http.StatusBadRequest)
		return
	}
	for _, k := range []string{"Range", "If-Range"} {
		if v := r.Header.Get(k); v != "" {
			req.Header.Set(k, v)
		}
	}
	req.Header.Set("User-Agent", "myco-app")
	res, err := streamClient.Do(req)
	if err != nil {
		if errors.Is(err, errNotPublic) {
			http.Error(w, "only public addresses can be streamed", http.StatusForbidden)
			return
		}
		if errors.Is(err, context.Canceled) {
			return
		}
		http.Error(w, "the media could not be reached", http.StatusBadGateway)
		return
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusRequestedRangeNotSatisfiable:
	case res.StatusCode != http.StatusOK && res.StatusCode != http.StatusPartialContent:
		http.Error(w, "the media answered "+res.Status, http.StatusBadGateway)
		return
	case !streamType(res.Header.Get("Content-Type")):
		http.Error(w, "not audio or video", http.StatusUnsupportedMediaType)
		return
	}
	h := w.Header()
	for _, k := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified"} {
		if v := res.Header.Get(k); v != "" {
			h.Set(k, v)
		}
	}
	h.Set("Cache-Control", "no-store")
	h.Set("Content-Security-Policy", "sandbox")
	w.WriteHeader(res.StatusCode)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = io.Copy(w, res.Body) // ends with the page's request (seek, pause, close)
}
