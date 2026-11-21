package oscap

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// The controller is deliberately separate from myco's own outbound proxy.
// This first adapter uses the Clash API shared by Mihomo and sing-box.
type proxyProfile struct {
	Endpoint  string `json:"endpoint"`
	Engine    string `json:"engine"`
	Secret    string `json:"secret,omitempty"`
	HasSecret bool   `json:"has_secret"`
}

type proxyNode struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Now     string   `json:"now,omitempty"`
	All     []string `json:"all,omitempty"`
	History []struct {
		Delay int `json:"delay"`
	} `json:"history,omitempty"`
}
type proxyConnection struct {
	ID       string `json:"id"`
	Metadata struct {
		Network         string `json:"network"`
		Host            string `json:"host"`
		DestinationIP   string `json:"destinationIP"`
		DestinationPort string `json:"destinationPort"`
		Process         string `json:"process"`
	} `json:"metadata"`
	Upload   uint64   `json:"upload"`
	Download uint64   `json:"download"`
	Start    string   `json:"start"`
	Chains   []string `json:"chains"`
	Rule     string   `json:"rule"`
}
type proxyConnections struct {
	DownloadTotal uint64            `json:"downloadTotal"`
	UploadTotal   uint64            `json:"uploadTotal"`
	Connections   []proxyConnection `json:"connections"`
}
type proxyConfig struct {
	Mode      string   `json:"mode"`
	ModeList  []string `json:"mode-list,omitempty"`
	MixedPort int      `json:"mixed-port"`
	Port      int      `json:"port"`
	SocksPort int      `json:"socks-port"`
	AllowLAN  *bool    `json:"allow-lan,omitempty"`
	LogLevel  string   `json:"log-level"`
	TUN       *struct {
		Enable bool `json:"enable"`
	} `json:"tun,omitempty"`
}

func init() {
	missing := func(e *Env) string {
		if e == nil || e.Secrets == nil {
			return "the system keychain is unavailable; update the myco desktop App"
		}
		return ""
	}
	register(
		&Cap{Name: "proxy.profile", About: "Get, save or forget this page's local controller (credentials stay in the keychain)", Missing: missing, Fn: proxyProfileCall},
		&Cap{Name: "proxy.control", About: "Read a local Mihomo/sing-box controller, select a proxy, change mode or close a connection", Missing: missing, Fn: proxyControl},
	)
}
func proxyKey(e *Env) string {
	h := sha256.Sum256([]byte(e.Scope))
	return "app-proxy-" + hex.EncodeToString(h[:])
}
func proxyEndpoint(s string) (string, error) {
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "http" && u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" {
		return "", errf(CodeInvalid, "use a loopback HTTP(S) controller URL, without credentials, path or query")
	}
	// Literal loopback addresses prevent DNS rebinding and accidental LAN exposure.
	ip := net.ParseIP(u.Hostname())
	if ip == nil || !ip.IsLoopback() {
		return "", errf(CodeInvalid, "use a loopback IP such as http://127.0.0.1:9090")
	}
	if u.Port() == "" {
		return "", errf(CodeInvalid, "include the controller port")
	}
	return strings.TrimRight(u.String(), "/"), nil
}
func proxyProfileCall(_ context.Context, e *Env, args json.RawMessage, _ Emit) (any, error) {
	var a struct {
		Action   string  `json:"action"`
		Endpoint string  `json:"endpoint"`
		Engine   string  `json:"engine"`
		Secret   *string `json:"secret"`
	}
	if err := decode(args, &a); err != nil {
		return nil, err
	}
	key := proxyKey(e)
	if a.Action == "forget" {
		if err := e.Secrets.Delete(key); err != nil {
			return nil, errf(CodeFailed, "could not remove the controller from the keychain")
		}
		return true, nil
	}
	p, err := loadProxy(e)
	if a.Action == "save" {
		endpoint, x := proxyEndpoint(a.Endpoint)
		if x != nil {
			return nil, x
		}
		if a.Engine != "mihomo" && a.Engine != "sing-box" {
			return nil, errf(CodeInvalid, "engine must be mihomo or sing-box")
		}
		if err != nil {
			p = proxyProfile{}
		}
		// An endpoint change never silently forwards the old endpoint's credential.
		if p.Endpoint != endpoint && a.Secret == nil {
			p.Secret = ""
		}
		p.Endpoint, p.Engine = endpoint, a.Engine
		if a.Secret != nil {
			p.Secret = *a.Secret
		}
		if strings.ContainsAny(p.Secret, "\r\n") || len(p.Secret) > 4096 {
			return nil, errf(CodeInvalid, "invalid controller secret")
		}
		raw, _ := json.Marshal(p)
		if e.Secrets.Set(key, string(raw)) != nil {
			return nil, errf(CodeFailed, "could not save the controller in the keychain")
		}
	} else if a.Action != "" && a.Action != "get" {
		return nil, errf(CodeInvalid, "action must be get, save or forget")
	} else if err != nil {
		return nil, err
	}
	p.HasSecret = p.Secret != ""
	p.Secret = ""
	return p, nil
}
func loadProxy(e *Env) (proxyProfile, error) {
	var p proxyProfile
	s, err := e.Secrets.Get(proxyKey(e))
	if err != nil || s == "" {
		return p, errf(CodeNotFound, "connect a local controller first")
	}
	if json.Unmarshal([]byte(s), &p) != nil {
		return p, errf(CodeFailed, "the saved controller is invalid; reconnect it")
	}
	if _, err := proxyEndpoint(p.Endpoint); err != nil {
		return p, err
	}
	return p, nil
}

var proxyHTTP = &http.Client{
	Timeout:       8 * time.Second,
	Transport:     &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 2 * time.Second}).DialContext, ResponseHeaderTimeout: 3 * time.Second, MaxIdleConnsPerHost: 4},
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

func proxyRequest(ctx context.Context, p proxyProfile, method, path string, body any, out any) error {
	var b []byte
	if body != nil {
		b, _ = json.Marshal(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.Endpoint+path, bytes.NewReader(b))
	if err != nil {
		return errf(CodeInvalid, "invalid controller request")
	}
	if p.Secret != "" {
		req.Header.Set("Authorization", "Bearer "+p.Secret)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := proxyHTTP.Do(req)
	if err != nil {
		return errf(CodeFailed, "cannot reach the local controller; check its address and whether the core is running")
	}
	defer res.Body.Close()
	if res.StatusCode == 401 || res.StatusCode == 403 {
		return errf(CodePermission, "controller rejected the secret; check connection settings")
	}
	if res.StatusCode == 404 || res.StatusCode == 405 || res.StatusCode == 501 {
		return errf(CodeUnsupported, "this controller does not support that operation")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return errf(CodeFailed, "controller returned HTTP %d", res.StatusCode)
	}
	if out == nil {
		return nil
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, (8<<20)+1))
	if err != nil {
		return errf(CodeFailed, "cannot read the controller response")
	}
	if len(raw) > 8<<20 {
		return errf(CodeTooLarge, "controller response exceeds 8 MiB")
	}
	if json.Unmarshal(raw, out) != nil {
		return errf(CodeFailed, "invalid controller response")
	}
	return nil
}
func proxyControl(ctx context.Context, e *Env, args json.RawMessage, _ Emit) (any, error) {
	var a struct {
		Action string `json:"action"`
		Group  string `json:"group"`
		Name   string `json:"name"`
		Mode   string `json:"mode"`
		ID     string `json:"id"`
	}
	if err := decode(args, &a); err != nil {
		return nil, err
	}
	p, err := loadProxy(e)
	if err != nil {
		return nil, err
	}
	// Bound every call even when the embedding client has no deadline.
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	switch a.Action {
	case "snapshot":
		var v struct {
			Version string `json:"version"`
		}
		var c proxyConfig
		var nodes struct {
			Proxies map[string]proxyNode `json:"proxies"`
		}
		var conn proxyConnections
		if err = proxyRequest(ctx, p, "GET", "/version", nil, &v); err != nil {
			return nil, err
		}
		warnings := []string{}
		for _, r := range []struct {
			path string
			out  any
		}{{"/configs", &c}, {"/proxies", &nodes}, {"/connections", &conn}} {
			if x := proxyRequest(ctx, p, "GET", r.path, nil, r.out); x != nil {
				warnings = append(warnings, fmt.Sprintf("%s: %s", r.path, AsError(x).Message))
			}
		}
		return map[string]any{"version": v.Version, "config": c, "proxies": nodes.Proxies, "traffic": conn, "warnings": warnings, "at": time.Now().UTC().Format(time.RFC3339Nano)}, nil
	case "mode":
		if a.Mode != "rule" && a.Mode != "global" && a.Mode != "direct" {
			return nil, errf(CodeInvalid, "mode must be rule, global or direct")
		}
		err = proxyRequest(ctx, p, "PATCH", "/configs", map[string]string{"mode": a.Mode}, nil)
		if err == nil {
			var actual proxyConfig
			if err = proxyRequest(ctx, p, "GET", "/configs", nil, &actual); err == nil && !strings.EqualFold(actual.Mode, a.Mode) {
				return nil, errf(CodeUnsupported, "the controller did not apply this mode; check its routing configuration")
			}
		}
	case "select":
		if a.Group == "" || a.Name == "" {
			return nil, errf(CodeInvalid, "group and name are required")
		}
		err = proxyRequest(ctx, p, "PUT", "/proxies/"+url.PathEscape(a.Group), map[string]string{"name": a.Name}, nil)
	case "delay":
		if a.Name == "" {
			return nil, errf(CodeInvalid, "name is required")
		}
		var d struct {
			Delay int `json:"delay"`
		}
		err = proxyRequest(ctx, p, "GET", "/proxies/"+url.PathEscape(a.Name)+"/delay?timeout=5000&url=https%3A%2F%2Fwww.gstatic.com%2Fgenerate_204", nil, &d)
		return d, err
	case "close":
		if a.ID == "" {
			return nil, errf(CodeInvalid, "connection id is required")
		}
		err = proxyRequest(ctx, p, "DELETE", "/connections/"+url.PathEscape(a.ID), nil, nil)
	default:
		return nil, errf(CodeInvalid, "unknown controller action")
	}
	if err != nil {
		return nil, err
	}
	return true, nil
}
