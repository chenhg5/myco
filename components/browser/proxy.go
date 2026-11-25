package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	socks "golang.org/x/net/proxy"
)

// forwardProxy owns a random IPv4-loopback listener and all CONNECT sockets.
// Credentials remain in memory; Chrome receives only this local endpoint.
type forwardProxy struct {
	server    *http.Server
	listener  net.Listener
	transport *http.Transport
	dial      func(context.Context, string, string) (net.Conn, error)
	once      sync.Once
	mu        sync.Mutex
	closed    bool
	conns     map[io.Closer]bool
	done      chan struct{}
}

func newForwardProxy(ctx context.Context, settings proxySettings, password string) (*forwardProxy, error) {
	upstream, err := proxyURL(settings.Server)
	if err != nil {
		return nil, err
	}
	p := &forwardProxy{conns: map[io.Closer]bool{}, done: make(chan struct{})}
	direct := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	p.transport = &http.Transport{ForceAttemptHTTP2: false, MaxIdleConns: 16, IdleConnTimeout: 30 * time.Second, ResponseHeaderTimeout: 30 * time.Second, TLSHandshakeTimeout: 15 * time.Second}
	if upstream.Scheme == "socks5" {
		var auth *socks.Auth
		if settings.Username != "" || password != "" {
			auth = &socks.Auth{User: settings.Username, Password: password}
		}
		d, e := socks.SOCKS5("tcp", upstream.Host, auth, direct)
		if e != nil {
			return nil, errors.New("invalid_argument")
		}
		cd, ok := d.(socks.ContextDialer)
		if !ok {
			return nil, errors.New("proxy_unavailable")
		}
		p.dial = cd.DialContext
		p.transport.DialContext = p.dial
	} else {
		proxyAddress := *upstream
		if settings.Username != "" || password != "" {
			proxyAddress.User = url.UserPassword(settings.Username, password)
		}
		p.transport.Proxy = http.ProxyURL(&proxyAddress)
		authorization := ""
		if settings.Username != "" || password != "" {
			authorization = "Basic " + base64.StdEncoding.EncodeToString([]byte(settings.Username+":"+password))
		}
		p.dial = func(ctx context.Context, network, address string) (net.Conn, error) {
			return dialHTTPProxy(ctx, address, upstream, authorization, nil)
		}
	}
	p.listener, err = net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		p.transport.CloseIdleConnections()
		return nil, err
	}
	p.server = &http.Server{Handler: http.HandlerFunc(p.serve), ReadHeaderTimeout: 10 * time.Second, MaxHeaderBytes: 64 << 10}
	go func() { _ = p.server.Serve(p.listener) }()
	go func() {
		select {
		case <-ctx.Done():
			p.Close()
		case <-p.done:
		}
	}()
	return p, nil
}

type bufferedProxyConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedProxyConn) Read(b []byte) (int, error) { return c.reader.Read(b) }
func (p *forwardProxy) Address() string                 { return "http://" + p.listener.Addr().String() }
func (p *forwardProxy) Close() {
	p.once.Do(func() {
		close(p.done)
		p.server.Close()
		p.transport.CloseIdleConnections()
		p.mu.Lock()
		p.closed = true
		for c := range p.conns {
			c.Close()
		}
		p.mu.Unlock()
	})
}
func (p *forwardProxy) track(c io.Closer) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		c.Close()
		return false
	}
	p.conns[c] = true
	return true
}
func (p *forwardProxy) forget(c io.Closer) { c.Close(); p.mu.Lock(); delete(p.conns, c); p.mu.Unlock() }
func proxyHopHeaders(h http.Header) {
	for _, line := range h.Values("Connection") {
		for _, key := range strings.Split(line, ",") {
			h.Del(strings.TrimSpace(key))
		}
	}
	for _, key := range []string{"Connection", "Proxy-Connection", "Keep-Alive", "Proxy-Authorization", "Proxy-Authenticate", "Te", "Trailer", "Transfer-Encoding", "Upgrade"} {
		h.Del(key)
	}
}
func (p *forwardProxy) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		if _, _, err := net.SplitHostPort(r.Host); err != nil {
			http.Error(w, "invalid CONNECT", 400)
			return
		}
		upstream, err := p.dial(r.Context(), "tcp", r.Host)
		if err != nil {
			http.Error(w, "proxy unavailable", 502)
			return
		}
		if !p.track(upstream) {
			return
		}
		defer p.forget(upstream)
		h, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "proxy unavailable", 502)
			return
		}
		client, buf, err := h.Hijack()
		if err != nil {
			return
		}
		if !p.track(client) {
			return
		}
		defer p.forget(client)
		if _, err = buf.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
			return
		}
		if buf.Flush() != nil {
			return
		}
		done := make(chan struct{})
		go func() { _, _ = io.Copy(upstream, buf); upstream.Close(); close(done) }()
		_, _ = io.Copy(client, upstream)
		client.Close()
		upstream.Close()
		<-done
		return
	}
	if (r.URL.Scheme != "http" && r.URL.Scheme != "ws") || r.URL.Host == "" {
		http.Error(w, "absolute HTTP URL required", 400)
		return
	}
	req := r.Clone(r.Context())
	req.RequestURI = ""
	req.URL.Scheme = "http"
	upgrade := req.Header.Get("Upgrade")
	proxyHopHeaders(req.Header)
	if upgrade != "" {
		req.Header.Set("Upgrade", upgrade)
		req.Header.Set("Connection", "Upgrade")
	}
	response, err := p.transport.RoundTrip(req)
	if err != nil {
		http.Error(w, "proxy unavailable", 502)
		return
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusSwitchingProtocols {
		stream, ok := response.Body.(io.ReadWriteCloser)
		if !ok {
			http.Error(w, "proxy unavailable", 502)
			return
		}
		if !p.track(stream) {
			return
		}
		defer p.forget(stream)
		h, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "proxy unavailable", 502)
			return
		}
		client, buf, err := h.Hijack()
		if err != nil {
			return
		}
		if !p.track(client) {
			return
		}
		defer p.forget(client)
		response.Header.Del("Proxy-Authenticate")
		response.Header.Del("Proxy-Authorization")
		if _, err = buf.WriteString("HTTP/1.1 101 Switching Protocols\r\n"); err != nil {
			return
		}
		if response.Header.Write(buf) != nil {
			return
		}
		buf.WriteString("\r\n")
		if buf.Flush() != nil {
			return
		}
		done := make(chan struct{})
		go func() { io.Copy(stream, buf); stream.Close(); close(done) }()
		io.Copy(client, stream)
		client.Close()
		stream.Close()
		<-done
		return
	}
	proxyHopHeaders(response.Header)
	for key, values := range response.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, response.Body)
}

func dialHTTPProxy(ctx context.Context, address string, upstream *url.URL, authorization string, tlsConfig *tls.Config) (net.Conn, error) {
	c, e := (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext(ctx, "tcp", upstream.Host)
	if e != nil {
		return nil, e
	}
	ok := false
	defer func() {
		if !ok {
			c.Close()
		}
	}()
	deadline := time.Now().Add(15 * time.Second)
	if d, yes := ctx.Deadline(); yes && d.Before(deadline) {
		deadline = d
	}
	c.SetDeadline(deadline)
	baseConn := c
	stop := context.AfterFunc(ctx, func() { baseConn.Close() })
	defer stop()
	if upstream.Scheme == "https" {
		if tlsConfig == nil {
			tlsConfig = &tls.Config{ServerName: upstream.Hostname(), MinVersion: tls.VersionTLS12}
		}
		tc := tls.Client(c, tlsConfig)
		c = tc
		if e = tc.HandshakeContext(ctx); e != nil {
			return nil, e
		}
	}
	req := &http.Request{Method: http.MethodConnect, URL: &url.URL{Opaque: address}, Host: address, Header: make(http.Header)}
	if authorization != "" {
		req.Header.Set("Proxy-Authorization", authorization)
	}
	if e = req.Write(c); e != nil {
		return nil, e
	}
	// Bound the untrusted proxy handshake without limiting tunnel traffic.
	header := &io.LimitedReader{R: c, N: 1 << 20}
	reader := bufio.NewReader(header)
	response, e := http.ReadResponse(reader, req)
	if e != nil {
		return nil, e
	}
	if response.StatusCode != http.StatusOK {
		return nil, errors.New("proxy_unavailable")
	}
	header.N = 1<<63 - 1
	c.SetDeadline(time.Time{})
	ok = true
	return &bufferedProxyConn{Conn: c, reader: reader}, nil
}
