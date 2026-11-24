package console

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/sysproxy"
)

// The App's network proxy (Settings › 代理): how it reaches workspaces,
// update checks, translation and voice services. By default the proxy in
// the environment (HTTPS_PROXY and friends), else the system's (Windows'
// and macOS' network settings); or none; or one
// set here, HTTP or SOCKS5, with a user and password if it needs them (the
// password in the keychain). Addresses on this computer and the local
// network always go direct: a workspace there keeps working.
//
//	GET  /app/proxy       → the setting (has_password, never the password)
//	PUT  /app/proxy       {mode, type, host, port, user, password?, clear_password?}
//	POST /app/proxy/test  {…the same, url?} → {ok, ms} or {ok: false, error}

// ProxyConfig is the App's proxy setting.
type ProxyConfig struct {
	// Mode: "system" (the environment's, else the system settings'), "none", or "manual".
	Mode string `json:"mode"`
	// Type of a manual proxy: "http", "https" or "socks5".
	Type string `json:"type,omitempty"`
	Host string `json:"host,omitempty"`
	Port int    `json:"port,omitempty"`
	User string `json:"user,omitempty"`
}

const proxySecret = "proxy:password"

var proxyTypes = map[string]bool{"http": true, "https": true, "socks5": true}

type proxyState struct {
	mu   sync.Mutex
	path string
}

func (b *Backend) proxyState() *proxyState {
	b.proxyOnce.Do(func() {
		path := ""
		if b.Store != nil {
			path = filepath.Join(filepath.Dir(b.Store.path), "console-proxy.json")
		}
		b.proxy = &proxyState{path: path}
	})
	return b.proxy
}

func (p *proxyState) load() ProxyConfig {
	c := ProxyConfig{Mode: "system"}
	if p.path == "" {
		return c
	}
	if raw, err := os.ReadFile(p.path); err == nil {
		_ = json.Unmarshal(raw, &c)
	}
	if c.Mode != "none" && c.Mode != "manual" {
		c.Mode = "system"
	}
	return c
}

func (p *proxyState) save(c ProxyConfig) error {
	if p.path == "" {
		return errors.New("no place to keep the settings")
	}
	if err := os.MkdirAll(filepath.Dir(p.path), 0o700); err != nil {
		return err
	}
	raw, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(p.path, raw, 0o600)
}

// check says what is wrong with a setting, in words for the person.
func (c ProxyConfig) check() error {
	switch c.Mode {
	case "system", "none":
		return nil
	case "manual":
	default:
		return errors.New("unknown proxy mode")
	}
	if !proxyTypes[c.Type] {
		return errors.New("the proxy type is HTTP, HTTPS or SOCKS5")
	}
	h := strings.TrimSpace(c.Host)
	if h == "" || strings.ContainsAny(h, "/ \t@") || strings.Contains(h, "://") {
		return errors.New("the proxy address is a host name or IP, without http:// or a port")
	}
	if c.Port < 1 || c.Port > 65535 {
		return errors.New("the proxy port is 1 to 65535")
	}
	return nil
}

// proxyURL is the manual proxy as a URL (with its user and password).
func (c ProxyConfig) proxyURL(password string) *url.URL {
	u := &url.URL{Scheme: c.Type, Host: net.JoinHostPort(strings.TrimSpace(c.Host), strconv.Itoa(c.Port))}
	if c.User != "" {
		if password != "" {
			u.User = url.UserPassword(c.User, password)
		} else {
			u.User = url.User(c.User)
		}
	}
	return u
}

// envProxy and systemProxyFor are the environment's proxy for a request
// and the system settings' proxy for a host (tests replace them: Go reads
// the environment once per process).
var (
	envProxy       = http.ProxyFromEnvironment
	systemProxyFor = sysproxy.For
)

// direct says whether a host is on this computer or the local network.
func direct(host string) bool {
	h := strings.ToLower(strings.TrimSuffix(host, "."))
	if h == "localhost" || strings.HasSuffix(h, ".localhost") || strings.HasSuffix(h, ".local") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified())
}

// proxyFor turns a setting into the choice made for each request.
func proxyFor(c ProxyConfig, password string) func(*http.Request) (*url.URL, error) {
	switch c.Mode {
	case "none":
		return func(*http.Request) (*url.URL, error) { return nil, nil }
	case "manual":
		u := c.proxyURL(password)
		return func(r *http.Request) (*url.URL, error) {
			if direct(r.URL.Hostname()) {
				return nil, nil
			}
			return u, nil
		}
	}
	// The environment's proxy, else the one in the system's network
	// settings (an App started from the desktop has no HTTPS_PROXY, and in
	// some places GitHub is reachable only through a proxy app's "system
	// proxy"), but not for this computer or the local network: a proxy
	// app's HTTPS_PROXY without NO_PROXY would otherwise send a workspace at
	// 192.168.x.x through it, and that usually fails. A system proxy that is
	// not listening (its app quit) is skipped.
	return func(r *http.Request) (*url.URL, error) {
		if direct(r.URL.Hostname()) {
			return nil, nil
		}
		if u, err := envProxy(r); u != nil || err != nil {
			return u, err
		}
		if u := systemProxyFor(r.URL.Hostname()); u != nil {
			return u, nil
		}
		return nil, nil
	}
}

func (b *Backend) proxyPassword() string {
	if b.Secrets == nil {
		return ""
	}
	pw, _ := b.Secrets.Get(proxySecret)
	return pw
}

// applyProxy puts the saved setting in force (at start, and when saved).
func (b *Backend) applyProxy() {
	client.InstallProxyOnDefault()
	c := b.proxyState().load()
	if c.check() != nil {
		c = ProxyConfig{Mode: "system"}
	}
	client.SetProxy(proxyFor(c, b.proxyPassword()))
}

func (b *Backend) proxyRoutes(m *http.ServeMux) {
	b.applyProxy()
	view := func(c ProxyConfig) map[string]any {
		return map[string]any{"mode": c.Mode, "type": c.Type, "host": c.Host, "port": c.Port, "user": c.User, "has_password": b.proxyPassword() != ""}
	}
	type input struct {
		ProxyConfig
		Password      string `json:"password"`
		ClearPassword bool   `json:"clear_password"`
		URL           string `json:"url"`
	}
	bad := func(w http.ResponseWriter, err error) {
		writeErr(w, &client.Error{Code: "invalid_argument", Message: err.Error(), Status: http.StatusBadRequest})
	}
	m.HandleFunc("GET /app/proxy", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, view(b.proxyState().load()))
	})
	m.HandleFunc("PUT /app/proxy", func(w http.ResponseWriter, r *http.Request) {
		var in input
		if !decode(w, r, &in) {
			return
		}
		in.Host = strings.TrimSpace(in.Host)
		if err := in.ProxyConfig.check(); err != nil {
			bad(w, err)
			return
		}
		ps := b.proxyState()
		ps.mu.Lock()
		defer ps.mu.Unlock()
		if err := ps.save(in.ProxyConfig); err != nil {
			writeErr(w, err)
			return
		}
		if b.Secrets != nil {
			switch {
			case in.ClearPassword || in.User == "":
				_ = b.Secrets.Delete(proxySecret)
			case in.Password != "":
				if err := b.Secrets.Set(proxySecret, in.Password); err != nil {
					writeErr(w, err)
					return
				}
			}
		}
		b.applyProxy()
		writeJSON(w, http.StatusOK, view(in.ProxyConfig))
	})
	// Tried before it is saved: a request through the setting given (the
	// saved password when none is given) to a workspace or the address
	// given, else to GitHub.
	m.HandleFunc("POST /app/proxy/test", func(w http.ResponseWriter, r *http.Request) {
		var in input
		if !decode(w, r, &in) {
			return
		}
		in.Host = strings.TrimSpace(in.Host)
		if err := in.ProxyConfig.check(); err != nil {
			bad(w, err)
			return
		}
		target := in.URL
		if u, err := url.Parse(target); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			target = "https://github.com/"
		}
		pw := in.Password
		if pw == "" && in.User != "" {
			pw = b.proxyPassword()
		}
		t := http.DefaultTransport.(*http.Transport).Clone()
		t.Proxy = proxyFor(in.ProxyConfig, pw)
		defer t.CloseIdleConnections()
		ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, http.MethodHead, target, nil)
		start := time.Now()
		res, err := (&http.Client{Transport: t}).Do(req)
		if err != nil {
			code, msg := proxyError(err)
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "reason": code, "error": msg})
			return
		}
		res.Body.Close()
		if res.StatusCode == http.StatusProxyAuthRequired {
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "reason": "auth", "error": "the proxy asks for a user name and password"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ms": time.Since(start).Milliseconds(), "status": res.StatusCode})
	})
}

// proxyError says why a request through a proxy failed, without the URL
// (it may carry the proxy's password).
// (reason: timeout, refused, not_found, auth or other, for the App's words)
func proxyError(err error) (string, string) {
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	msg := err.Error()
	switch {
	case errors.Is(err, context.DeadlineExceeded) || strings.Contains(msg, "timeout"):
		return "timeout", "no answer within 12 seconds"
	case strings.Contains(msg, "connection refused"):
		return "refused", "the proxy refused the connection (is it running, on that port?)"
	case strings.Contains(msg, "no such host"):
		return "not_found", "the address was not found"
	case strings.Contains(msg, "407") || strings.Contains(strings.ToLower(msg), "authentication"):
		return "auth", "the proxy did not accept the user name and password"
	}
	return "other", fmt.Sprintf("could not connect: %v", err)
}
