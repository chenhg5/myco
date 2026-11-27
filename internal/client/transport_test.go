package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// silentLink relays TCP between a client and a server until cut; then it
// keeps both sides open and drops everything, like a network that went away
// without closing anything (sleep, a changed network, a lost NAT entry).
type silentLink struct {
	ln  net.Listener
	cut atomic.Bool
	wg  sync.WaitGroup
}

func newSilentLink(t *testing.T, target string) *silentLink {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	l := &silentLink{ln: ln}
	var conns []net.Conn
	var mu sync.Mutex
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			s, err := net.Dial("tcp", target)
			if err != nil {
				c.Close()
				continue
			}
			mu.Lock()
			conns = append(conns, c, s)
			mu.Unlock()
			l.wg.Add(2)
			go l.relay(s, c)
			go l.relay(c, s)
		}
	}()
	t.Cleanup(func() {
		ln.Close()
		mu.Lock()
		for _, c := range conns {
			c.Close()
		}
		mu.Unlock()
		l.wg.Wait()
	})
	return l
}

func (l *silentLink) relay(dst, src net.Conn) {
	defer l.wg.Done()
	buf := make([]byte, 32<<10)
	for {
		n, err := src.Read(buf)
		if n > 0 && !l.cut.Load() {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// A long-poll on an HTTP/2 connection that went silent must fail soon and
// retryably, not wait out its own bound (its wait plus a minute).
func TestDeadHTTP2ConnectionIsNoticedByPing(t *testing.T) {
	polling := make(chan struct{}, 4)
	var proto atomic.Int32
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proto.Store(int32(r.ProtoMajor))
		if r.URL.Path == "/v1/inbox/wait" {
			polling <- struct{}{}
			select {
			case <-r.Context().Done():
			case <-time.After(30 * time.Second):
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true}`)
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())

	longPoll := func(t *testing.T, pingAfter, pingTimeout time.Duration) (chan error, context.CancelFunc) {
		t.Helper()
		link := newSilentLink(t, srv.Listener.Addr().String())
		c, err := New("https://"+link.ln.Addr().String(), "tok")
		if err != nil {
			t.Fatal(err)
		}
		tr := newTransport(pingAfter, pingTimeout)
		tr.Proxy = nil
		tr.TLSClientConfig = &tls.Config{RootCAs: pool}
		t.Cleanup(tr.CloseIdleConnections)
		c.http = &http.Client{Transport: tr}
		if _, err := c.Health(context.Background()); err != nil {
			t.Fatalf("health through the link: %v", err)
		}
		if proto.Load() != 2 {
			t.Fatalf("the test needs HTTP/2, got HTTP/%d", proto.Load())
		}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			var out map[string]any
			done <- c.do(ctx, http.MethodGet, "/v1/inbox/wait", url.Values{"wait_seconds": {"60"}}, nil, &out)
		}()
		select {
		case <-polling:
		case <-time.After(5 * time.Second):
			t.Fatal("the long-poll did not reach the server")
		}
		link.cut.Store(true)
		return done, cancel
	}

	t.Run("with pings", func(t *testing.T) {
		done, cancel := longPoll(t, 200*time.Millisecond, 200*time.Millisecond)
		defer cancel()
		select {
		case err := <-done:
			e, ok := AsError(err)
			if !ok || !e.Retryable || e.Code != CodeUnavailable {
				t.Fatalf("want a retryable unavailable error, got %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("the long-poll still waits on a dead connection")
		}
	})
	// Without pings nothing notices: this is what the test above guards against.
	t.Run("without pings", func(t *testing.T) {
		done, cancel := longPoll(t, 0, 0)
		select {
		case err := <-done:
			t.Fatalf("returned without a health check: %v", err)
		case <-time.After(1500 * time.Millisecond):
		}
		cancel()
		<-done
	})
}

// Every client shares one transport (and so one pool of connections to a hub).
func TestClientsShareTheHubTransport(t *testing.T) {
	a, err := New("https://hub.example.com", "x")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := New("https://hub.example.com", "y")
	base := func(c *Client) http.RoundTripper { return c.http.Transport.(withRequestID).base }
	if base(a) != hubTransport || base(b) != hubTransport {
		t.Fatal("clients do not share hubTransport")
	}
	if h := hubTransport.current.HTTP2; h == nil || h.SendPingTimeout != pingAfter || h.PingTimeout != pingTimeout {
		t.Fatalf("hubTransport has no HTTP/2 health check: %+v", hubTransport.current.HTTP2)
	}
}
