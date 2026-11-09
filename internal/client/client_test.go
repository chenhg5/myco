package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// A proxy reset or 502 in the middle of a long poll must not end the wait.
func TestLongPollsRideOutNetworkBlips(t *testing.T) {
	retryPause = 10 * time.Millisecond
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch n.Add(1) {
		case 1: // connection reset
			conn, _, _ := w.(http.Hijacker).Hijack()
			conn.Close()
		case 2:
			http.Error(w, `{"error":"bad_gateway","message":"upstream"}`, http.StatusBadGateway)
		default:
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Path == "/v1/inbox/wait" {
				w.Write([]byte(`{"pending":[{"id":"msg_1"}]}`))
			} else {
				w.Write([]byte(`{"reply":{"id":"msg_r","body":"ok"}}`))
			}
		}
	}))
	defer srv.Close()
	c, err := New(srv.URL, "myco_test")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	reply, err := c.WaitReply(ctx, "msg_x", 5*time.Second)
	if err != nil || reply == nil || reply.Body != "ok" {
		t.Fatalf("WaitReply: %+v %v", reply, err)
	}
	n.Store(0)
	msgs, err := c.WaitPending(ctx, 5*time.Second)
	if err != nil || len(msgs) != 1 {
		t.Fatalf("WaitPending: %+v %v", msgs, err)
	}
	// Non-retryable errors still end the wait at once.
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"not_found","message":"no such message"}`, http.StatusNotFound)
	}))
	defer bad.Close()
	c2, _ := New(bad.URL, "myco_test")
	if _, err := c2.WaitReply(ctx, "msg_x", 5*time.Second); err == nil {
		t.Fatal("404 must not be retried")
	}
}

// A hub that never answers (a connection that died while the computer
// slept) must not hang a call forever: it fails as retryable instead.
func TestCallsAreBounded(t *testing.T) {
	boundMargin, plainBound = 200*time.Millisecond, 200*time.Millisecond
	t.Cleanup(func() { boundMargin, plainBound = 60*time.Second, 2*time.Minute })
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-release }))
	t.Cleanup(func() { close(release); srv.Close() })
	c, err := New(srv.URL, "tok")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err = c.GetAgent(context.Background(), "x")
	var ce *Error
	if !errors.As(err, &ce) || ce.Code != CodeUnavailable || !ce.Retryable {
		t.Fatalf("want a retryable unavailable error, got %v", err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("took %s", d)
	}
	// The caller's own cancellation still reads as such.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := c.GetAgent(ctx, "x"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("caller deadline: %v", err)
	}
}
