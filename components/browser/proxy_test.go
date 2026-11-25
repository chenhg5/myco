package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func tunnelFixture(a, b net.Conn) {
	defer a.Close()
	defer b.Close()
	done := make(chan struct{})
	go func() { io.Copy(a, b); a.Close(); close(done) }()
	io.Copy(b, a)
	b.Close()
	<-done
}
func proxyOrigin(t *testing.T) *httptest.Server {
	t.Helper()
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Proxy-Authorization") != "" {
			t.Error("proxy authentication leaked to origin")
		}
		fmt.Fprint(w, "origin reached")
	}))
	t.Cleanup(s.Close)
	return s
}
func proxyClient(p *forwardProxy) *http.Client {
	u, _ := url.Parse(p.Address())
	return &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(u), TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
}
func TestForwardHTTPConnectAuthenticationAndClose(t *testing.T) {
	origin := proxyOrigin(t)
	var auth atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "CONNECT" || r.Header.Get("Proxy-Authorization") != "Basic "+base64.StdEncoding.EncodeToString([]byte("fixture-user:fixture-password")) {
			t.Error("missing upstream CONNECT authentication")
			http.Error(w, "denied", 407)
			return
		}
		auth.Add(1)
		target, err := net.Dial("tcp", r.Host)
		if err != nil {
			http.Error(w, "target", 502)
			return
		}
		c, b, err := w.(http.Hijacker).Hijack()
		if err != nil {
			target.Close()
			return
		}
		b.WriteString("HTTP/1.1 200 OK\r\n\r\n")
		b.Flush()
		tunnelFixture(c, target)
	}))
	defer upstream.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p, err := newForwardProxy(ctx, proxySettings{Server: upstream.URL, Username: "fixture-user"}, "fixture-password")
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	client := proxyClient(p)
	defer client.CloseIdleConnections()
	response, err := client.Get(origin.URL)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if string(body) != "origin reached" || auth.Load() != 1 {
		t.Fatal("authenticated proxy did not reach origin")
	}
	address := p.listener.Addr().String()
	p.Close()
	if c, err := net.DialTimeout("tcp", address, 100*time.Millisecond); err == nil {
		c.Close()
		t.Fatal("proxy listener survived session close")
	}

}
func TestForwardHTTPAbsoluteRequest(t *testing.T) {
	var got atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !r.URL.IsAbs() || r.Header.Get("Proxy-Authorization") != "Basic "+base64.StdEncoding.EncodeToString([]byte("u:p")) {
			t.Error("absolute HTTP proxy request/auth missing")
		}
		got.Store(true)
		fmt.Fprint(w, "proxied")
	}))
	defer upstream.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p, err := newForwardProxy(ctx, proxySettings{Server: upstream.URL, Username: "u"}, "p")
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	client := proxyClient(p)
	defer client.CloseIdleConnections()
	res, err := client.Get("http://origin.invalid/path")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if !got.Load() {
		t.Fatal("request bypassed upstream")
	}
}
func TestForwardSOCKS5Authentication(t *testing.T) {
	origin := proxyOrigin(t)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	checked := make(chan error, 1)
	go func() {
		c, err := listener.Accept()
		if err != nil {
			checked <- err
			return
		}
		defer c.Close()
		c.SetDeadline(time.Now().Add(5 * time.Second))
		r := bufio.NewReader(c)
		read := func(n int) []byte { b := make([]byte, n); _, err = io.ReadFull(r, b); return b }
		hello := read(2)
		read(int(hello[1]))
		c.Write([]byte{5, 2})
		version := read(2)
		user := read(int(version[1]))
		n := read(1)
		password := read(int(n[0]))
		if version[0] != 1 || string(user) != "fixture-user" || string(password) != "fixture-password" {
			checked <- fmt.Errorf("SOCKS auth mismatch")
			return
		}
		c.Write([]byte{1, 0})
		header := read(4)
		var host string
		switch header[3] {
		case 1:
			host = net.IP(read(4)).String()
		case 3:
			n := read(1)
			host = string(read(int(n[0])))
		case 4:
			host = net.IP(read(16)).String()
		default:
			checked <- fmt.Errorf("SOCKS address type")
			return
		}
		port := read(2)
		target, e := net.Dial("tcp", net.JoinHostPort(host, fmt.Sprint(int(port[0])*256+int(port[1]))))
		if e != nil {
			checked <- e
			return
		}
		c.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0})
		checked <- nil
		tunnelFixture(c, target)
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p, err := newForwardProxy(ctx, proxySettings{Server: "socks5://" + listener.Addr().String(), Username: "fixture-user"}, "fixture-password")
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	client := proxyClient(p)
	defer client.CloseIdleConnections()
	res, err := client.Get(origin.URL)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if strings.TrimSpace(string(b)) != "origin reached" {
		t.Fatal("SOCKS target not reached")
	}
	if err = <-checked; err != nil {
		t.Fatal(err)
	}
}

func TestForwardHTTPUpgrade(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Upgrade") != "fixture" {
			http.Error(w, "upgrade missing", 400)
			return
		}
		c, b, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer c.Close()
		b.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: fixture\r\n\r\n")
		b.Flush()
		io.Copy(c, b)
	}))
	defer upstream.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p, err := newForwardProxy(ctx, proxySettings{Server: upstream.URL}, "")
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	c, err := net.Dial("tcp", p.listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(5 * time.Second))
	fmt.Fprint(c, "GET http://origin.invalid/socket HTTP/1.1\r\nHost: origin.invalid\r\nConnection: Upgrade\r\nUpgrade: fixture\r\n\r\n")
	reader := bufio.NewReader(c)
	response, err := http.ReadResponse(reader, nil)
	if err != nil || response.StatusCode != 101 {
		t.Fatalf("upgrade response: %v %v", response, err)
	}
	fmt.Fprint(c, "fixture-echo")
	b := make([]byte, len("fixture-echo"))
	if _, err = io.ReadFull(reader, b); err != nil || string(b) != "fixture-echo" {
		t.Fatal("upgrade did not forward both directions", err)
	}
	p.Close()
	if _, err = reader.ReadByte(); err == nil {
		t.Fatal("upgraded connection survived proxy close")
	}
}

func TestForwardHTTPSProxyVerifiedTLS(t *testing.T) {
	origin := proxyOrigin(t)
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "CONNECT" || r.Header.Get("Proxy-Authorization") != "Basic "+base64.StdEncoding.EncodeToString([]byte("u:p")) {
			http.Error(w, "auth", 407)
			return
		}
		target, err := net.Dial("tcp", r.Host)
		if err != nil {
			http.Error(w, "target", 502)
			return
		}
		c, b, err := w.(http.Hijacker).Hijack()
		if err != nil {
			target.Close()
			return
		}
		b.WriteString("HTTP/1.1 200 OK\r\n\r\n")
		b.Flush()
		tunnelFixture(c, target)
	}))
	defer upstream.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p, err := newForwardProxy(ctx, proxySettings{Server: upstream.URL, Username: "u"}, "p")
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	client := proxyClient(p)
	defer client.CloseIdleConnections()
	if res, err := client.Get(origin.URL); err == nil {
		res.Body.Close()
		t.Fatal("untrusted HTTPS proxy certificate accepted")
	}
	// Trust only this local fixture CA; production always uses system roots.
	roots := x509.NewCertPool()
	roots.AddCert(upstream.Certificate())
	u, _ := url.Parse(upstream.URL)
	conn, err := dialHTTPProxy(ctx, strings.TrimPrefix(origin.URL, "https://"), u, "Basic "+base64.StdEncoding.EncodeToString([]byte("u:p")), &tls.Config{RootCAs: roots, ServerName: u.Hostname(), MinVersion: tls.VersionTLS12})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	target := tls.Client(conn, &tls.Config{InsecureSkipVerify: true})
	req, _ := http.NewRequest("GET", origin.URL, nil)
	if err = req.Write(target); err != nil {
		t.Fatal(err)
	}
	res, err := http.ReadResponse(bufio.NewReader(target), req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()

	if string(b) != "origin reached" {
		t.Fatal("verified TLS proxy did not tunnel origin")
	}
}

func TestProxyConnectHeaderIsBounded(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Oversized", strings.Repeat("a", 2<<20))
		w.WriteHeader(200)
	}))
	defer upstream.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p, err := newForwardProxy(ctx, proxySettings{Server: upstream.URL}, "")
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	client := proxyClient(p)
	defer client.CloseIdleConnections()
	if res, err := client.Get("https://origin.invalid"); err == nil {
		res.Body.Close()
		t.Fatal("oversized CONNECT headers accepted")
	}
}
