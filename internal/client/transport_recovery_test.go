package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// The first connection still answers HTTP/2 pings, but its application stops
// responding. Existing clients must recover without cancelling another stream.
func TestNoResponseTimeoutsReplaceStuckHTTP2Connection(t *testing.T) {
	oldBound := plainBound
	plainBound = 3 * time.Second
	t.Cleanup(func() { plainBound = oldBound })
	type connectionKey struct{}
	var connections atomic.Int32
	var protocol atomic.Int32
	var stuck atomic.Bool
	started := make(chan struct{}, 1)
	cancelled := make(chan struct{}, 1)
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseOld := func() { releaseOnce.Do(func() { close(release) }) }
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		protocol.Store(int32(r.ProtoMajor))
		if r.Context().Value(connectionKey{}).(int32) == 1 {
			if r.URL.Path == "/inflight" {
				started <- struct{}{}
				select {
				case <-release:
				case <-r.Context().Done():
					cancelled <- struct{}{}
					return
				}
			} else if stuck.Load() {
				if r.URL.Path == "/headers" {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					w.(http.Flusher).Flush()
				}
				<-r.Context().Done()
				return
			}
		}
		io.WriteString(w, `{}`)
	}))
	srv.Config.ConnContext = func(ctx context.Context, _ net.Conn) context.Context {
		return context.WithValue(ctx, connectionKey{}, connections.Add(1))
	}
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()
	defer releaseOld()
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	tr := newRecoveringTransport(func() *http.Transport {
		base := newTransport(20*time.Millisecond, 20*time.Millisecond)
		base.Proxy = nil
		base.TLSClientConfig = &tls.Config{RootCAs: pool}
		return base
	})
	old := tr.current
	defer old.CloseIdleConnections()
	defer tr.CloseIdleConnections()
	c, err := New(srv.URL, "test-token-do-not-log")
	if err != nil {
		t.Fatal(err)
	}
	c.http.Transport = forLANHub(c.base, tr)
	if _, err := c.Health(context.Background()); err != nil {
		t.Fatal(err)
	}
	if protocol.Load() != 2 {
		t.Skipf("HTTP/2 unavailable (HTTP/%d)", protocol.Load())
	}
	inflight := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/inflight", nil)
		if err != nil {
			inflight <- err
			return
		}
		resp, err := c.http.Do(req)
		if err == nil {
			_, err = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
		inflight <- err
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("old stream did not start")
	}
	stuck.Store(true)
	plainBound = 150 * time.Millisecond
	// Caller deadlines and a stalled response body are not no-response votes.
	caller, stopCaller := context.WithTimeout(context.Background(), 20*time.Millisecond)
	_, err = c.Health(caller)
	stopCaller()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("caller deadline: %v", err)
	}
	if err := c.do(context.Background(), http.MethodGet, "/headers", nil, nil, nil); err == nil {
		t.Fatal("expected stalled response body")
	}
	if tr.current != old || len(tr.timeouts) != 0 {
		t.Fatal("caller/body timeout counted")
	}
	for attempt := 0; attempt < 2; attempt++ {
		_, err := c.Health(context.Background())
		e, ok := AsError(err)
		if !ok || e.Code != CodeUnavailable || !e.Retryable {
			t.Fatalf("bounded call: %v", err)
		}
		if attempt == 0 && tr.current != old {
			t.Fatal("one timeout rotated the pool")
		}
	}
	if tr.current == old {
		t.Fatal("two no-response timeouts did not replace the pool")
	}
	// Both the existing client and a second client use the replacement.
	// TLS setup is not part of the deliberately short no-response bound.
	plainBound = 3 * time.Second
	b, _ := New(srv.URL, "other")
	b.http.Transport = tr
	for _, client := range []*Client{c, b} {
		if _, err := client.Health(context.Background()); err != nil {
			t.Fatalf("fresh pool: %v", err)
		}
	}
	if connections.Load() != 2 {
		t.Fatalf("want exactly two connections, got %d", connections.Load())
	}
	select {
	case <-cancelled:
		t.Fatal("swap cancelled an in-flight stream")
	case err := <-inflight:
		t.Fatalf("old stream ended before release: %v", err)
	default:
	}
	releaseOld()
	select {
	case err := <-inflight:
		if err != nil {
			t.Fatalf("old stream failed: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("old stream did not finish after release")
	}
}

func TestTransportRecoveryWindowCooldownAndOldGeneration(t *testing.T) {
	var logs bytes.Buffer
	prior := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	defer slog.SetDefault(prior)
	tr := newRecoveringTransport(func() *http.Transport { return newTransport(0, 0) })
	defer tr.CloseIdleConnections()
	now := time.Now()
	tr.now = func() time.Time { return now }
	old := tr.current
	tr.noResponse(old, "hub-a")
	tr.noResponse(old, "hub-b")
	if tr.current != old {
		t.Fatal("unrelated hubs combined their timeouts")
	}
	now = now.Add(tr.window + time.Nanosecond)
	tr.noResponse(old, "hub-a")
	if tr.current != old {
		t.Fatal("expired timeout counted")
	}
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() { defer wg.Done(); tr.noResponse(old, "hub-a") }()
	}
	wg.Wait()
	fresh := tr.current
	if fresh == old {
		t.Fatal("pool not replaced")
	}
	if strings.Count(logs.String(), "level=WARN") != 1 || strings.Contains(logs.String(), "hub-a") {
		t.Fatalf("want one redacted warning: %s", logs.String())
	}
	tr.noResponse(fresh, "hub-a")
	tr.noResponse(fresh, "hub-a")
	if tr.current != fresh {
		t.Fatal("rotation ignored cooldown")
	}
	now = now.Add(tr.cooldown)
	tr.noResponse(old, "hub-a")
	if tr.current != fresh {
		t.Fatal("retired pool rotated current pool")
	}
	tr.noResponse(fresh, "hub-a")
	if tr.current == fresh {
		t.Fatal("rotation did not recover after cooldown")
	}
}
