// Package sysproxy reads the proxy set in the operating system's network
// settings (Windows Internet Settings, macOS System Settings).
//
// Go reads only HTTPS_PROXY and friends, and an app started from the
// desktop has none of them: where GitHub is reachable only through a proxy
// app's "system proxy" (Clash, v2rayN, ...), the browser gets through and a
// Go program does not. For tells such a program which proxy the system
// would use for a host: none when the setting is off, the host is in its
// exceptions, or the proxy is not listening (a proxy app that quit without
// turning the setting off), so a stale setting never cuts a program off.
package sysproxy

import (
	"net"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Setting is the system's proxy setting.
type Setting struct {
	// Proxy is the proxy for HTTPS (else HTTP, else SOCKS); nil when off.
	Proxy *url.URL
	// Bypass lists the hosts that go direct: patterns with *, "<local>"
	// (names without a dot) and, on macOS, networks such as 169.254/16.
	Bypass []string
}

// How long a reading of the settings, and a check that the proxy listens,
// are trusted: the setting changes when a proxy app is switched on or off.
var (
	settingTTL = 30 * time.Second
	probeTTL   = 15 * time.Second
	probeWait  = 800 * time.Millisecond
)

// read reads the setting (per platform; tests replace it).
var read = readSystem

// dial checks that the proxy accepts connections (tests replace it).
var dial = func(addr string, timeout time.Duration) error {
	c, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return err
	}
	return c.Close()
}

var (
	mu      sync.Mutex
	cached  Setting
	readAt  time.Time
	probes  = map[string]probe{}
	nowFunc = time.Now
)

type probe struct {
	ok bool
	at time.Time
}

// Current is the system's proxy setting, read at most every 30 seconds.
func Current() Setting {
	mu.Lock()
	defer mu.Unlock()
	if readAt.IsZero() || nowFunc().Sub(readAt) > settingTTL {
		cached, readAt = read(), nowFunc()
	}
	return cached
}

// For is the proxy the system would use for host, or nil: none set, host
// in the exceptions, or the proxy not listening.
func For(host string) *url.URL {
	s := Current()
	if s.Proxy == nil || s.Bypassed(host) {
		return nil
	}
	if !listening(s.Proxy.Host) {
		return nil
	}
	return s.Proxy
}

// listening checks addr at most every 15 seconds.
func listening(addr string) bool {
	mu.Lock()
	p, ok := probes[addr]
	mu.Unlock()
	if ok && nowFunc().Sub(p.at) < probeTTL {
		return p.ok
	}
	up := dial(addr, probeWait) == nil
	mu.Lock()
	probes[addr] = probe{ok: up, at: nowFunc()}
	mu.Unlock()
	return up
}

// Bypassed says whether host goes direct by the setting's exceptions.
// Loopback addresses always do.
func (s Setting) Bypassed(host string) bool {
	h := strings.ToLower(strings.TrimSuffix(strings.Trim(host, "[]"), "."))
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	if ip != nil && ip.IsLoopback() {
		return true
	}
	for _, b := range s.Bypass {
		b = strings.ToLower(strings.TrimSpace(b))
		switch {
		case b == "":
		case b == "<local>":
			if ip == nil && !strings.Contains(h, ".") {
				return true
			}
		case strings.Contains(b, "/"):
			if ip != nil {
				if _, n, err := net.ParseCIDR(padCIDR(b)); err == nil && n.Contains(ip) {
					return true
				}
			}
		default:
			if ok, _ := path.Match(b, h); ok {
				return true
			}
			// "*.example.com" covers example.com itself, as browsers do;
			// ".example.com" means the same.
			if rest, ok := strings.CutPrefix(b, "*."); ok && h == rest {
				return true
			}
			if strings.HasPrefix(b, ".") && (h == b[1:] || strings.HasSuffix(h, b)) {
				return true
			}
		}
	}
	return false
}

// padCIDR completes macOS's short networks: 169.254/16 → 169.254.0.0/16.
func padCIDR(s string) string {
	ip, bits, _ := strings.Cut(s, "/")
	if strings.Contains(ip, ":") {
		return s
	}
	for strings.Count(ip, ".") < 3 {
		ip += ".0"
	}
	return ip + "/" + bits
}

// parseWindows reads Internet Settings' ProxyEnable, ProxyServer and
// ProxyOverride. ProxyServer is "host:port" for every scheme, or per
// scheme: "http=h:p;https=h:p;socks=h:p".
func parseWindows(enable uint64, server, override string) Setting {
	server = strings.TrimSpace(server)
	if enable == 0 || server == "" {
		return Setting{}
	}
	u := (*url.URL)(nil)
	if !strings.Contains(server, "=") {
		u = proxyURL("http", server)
	} else {
		per := map[string]string{}
		for _, part := range strings.Split(server, ";") {
			k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
			if ok && v != "" {
				per[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
			}
		}
		switch {
		case per["https"] != "":
			u = proxyURL("http", per["https"])
		case per["http"] != "":
			u = proxyURL("http", per["http"])
		case per["socks"] != "":
			u = proxyURL("socks5", per["socks"])
		}
	}
	if u == nil {
		return Setting{}
	}
	var bypass []string
	for _, b := range strings.Split(override, ";") {
		if b = strings.TrimSpace(b); b != "" {
			bypass = append(bypass, b)
		}
	}
	return Setting{Proxy: u, Bypass: bypass}
}

// parseScutil reads `scutil --proxy`: HTTPS, else HTTP, else SOCKS, and
// ExceptionsList.
func parseScutil(s string) Setting {
	v := map[string]string{}
	var bypass []string
	inList := false
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		if inList {
			if t == "}" {
				inList = false
				continue
			}
			if _, e, ok := strings.Cut(t, ":"); ok {
				bypass = append(bypass, strings.TrimSpace(e))
			}
			continue
		}
		k, val, ok := strings.Cut(t, ":")
		if !ok {
			continue
		}
		k, val = strings.TrimSpace(k), strings.TrimSpace(val)
		if k == "ExceptionsList" && strings.HasPrefix(val, "<array>") {
			inList = true
			continue
		}
		v[k] = val
	}
	for _, p := range []struct{ key, scheme string }{{"HTTPS", "http"}, {"HTTP", "http"}, {"SOCKS", "socks5"}} {
		if v[p.key+"Enable"] == "1" && v[p.key+"Proxy"] != "" && v[p.key+"Port"] != "" {
			if u := proxyURL(p.scheme, net.JoinHostPort(v[p.key+"Proxy"], v[p.key+"Port"])); u != nil {
				return Setting{Proxy: u, Bypass: bypass}
			}
		}
	}
	return Setting{}
}

// proxyURL makes a proxy URL of "host:port" (or "scheme://host:port").
func proxyURL(scheme, s string) *url.URL {
	if !strings.Contains(s, "://") {
		s = scheme + "://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Hostname() == "" {
		return nil
	}
	if p := u.Port(); p != "" {
		if n, err := strconv.Atoi(p); err != nil || n <= 0 || n > 65535 {
			return nil
		}
	}
	return u
}
