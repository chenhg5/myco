package console

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/remotedesktop"
)

func TestRemoteConnectClosingPageDuringRequest(t *testing.T) {
	started := make(chan struct{})
	resume := make(chan struct{})
	deleted := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST":
			close(started)
			<-resume
			_, _ = w.Write([]byte(`{"id":"connection","machine":"mac","state":"pending"}`))
		case r.Method == "DELETE":
			deleted <- struct{}{}
			w.WriteHeader(204)
		}
	}))
	defer server.Close()
	c, e := client.New(server.URL, "test-token")
	if e != nil {
		t.Fatal(e)
	}
	b := &Backend{}
	comp := &remotedesktop.Component{Home: t.TempDir()}
	done := make(chan error, 1)
	go func() {
		_, e := b.remoteDesktopCall(context.Background(), c, Hub{}, "workspace/page", "connect", json.RawMessage(`{"machine":"mac","offer":{"description":{"type":"offer","sdp":"v=0"}}}`), comp)
		done <- e
	}()
	<-started
	if _, e = b.remoteDesktopCall(context.Background(), c, Hub{}, "workspace/page", "close", nil, comp); e != nil {
		t.Fatal(e)
	}
	close(resume)
	if e = <-done; e != context.Canceled {
		t.Fatalf("request survived its page: %v", e)
	}
	<-deleted
	b.remoteDesktop.mu.Lock()
	defer b.remoteDesktop.mu.Unlock()
	if len(b.remoteDesktop.sessions) != 0 {
		t.Fatal("orphan session", b.remoteDesktop.sessions)
	}
}
func TestRemoteSessionsAreScopedToTheirPage(t *testing.T) {
	b := &Backend{}
	b.remoteDesktop.sessions = map[string]string{"connection": "workspace/first"}
	_, e := b.remoteDesktopCall(context.Background(), nil, Hub{}, "workspace/second", "disconnect", json.RawMessage(`{"id":"connection"}`), &remotedesktop.Component{Home: t.TempDir()})
	ce, ok := e.(*client.Error)
	if !ok || ce.Status != 403 {
		t.Fatal(e)
	}
}

func TestRemoteRejectsRequestsArrivingAfterPageClose(t *testing.T) {
	b := &Backend{}
	comp := &remotedesktop.Component{Home: t.TempDir()}
	if _, e := b.remoteDesktopCall(context.Background(), nil, Hub{}, "workspace/page/old", "close", nil, comp); e != nil {
		t.Fatal(e)
	}
	if _, e := b.remoteDesktopCall(context.Background(), nil, Hub{}, "workspace/page/old", "connect", nil, comp); e != context.Canceled {
		t.Fatalf("late request was allowed: %v", e)
	}
	if _, e := b.remoteDesktopCall(context.Background(), nil, Hub{}, "workspace/page/new", "status", nil, comp); e != nil {
		t.Fatalf("new page instance inherited old revocation: %v", e)
	}
}
