package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCoordinationOldHubUnsupportedAndErrors(t *testing.T) {
	for _, status := range []int{404, 405, 200} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				if status == 200 {
					_, _ = w.Write([]byte(`{"agents":[]}`))
				}
			}))
			defer srv.Close()
			c, _ := New(srv.URL, "example-token")
			_, err := c.AgentStats(context.Background(), AgentStatsFilter{})
			e, ok := AsError(err)
			if !ok || e.Code != "unsupported" || e.Retryable {
				t.Fatalf("old hub: %v", err)
			}
		})
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		_, _ = w.Write([]byte(`{"error":"permission_denied","message":"denied","suggestion":"ask the owner","retryable":false}`))
	}))
	defer srv.Close()
	c, _ := New(srv.URL, "example-token")
	_, err := c.PickAgents(context.Background(), AgentPickFilter{})
	e, ok := AsError(err)
	if !ok || e.Code != "permission_denied" {
		t.Fatal(err)
	}
}

func TestCoordinationOldHubJSONMissingRoute(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`{"error":"not_found","message":"no route for GET /v1/stats/agents"}`))
	}))
	defer srv.Close()
	c, _ := New(srv.URL, "example-token")
	_, err := c.AgentStats(context.Background(), AgentStatsFilter{})
	if e, ok := AsError(err); !ok || e.Code != "unsupported" || e.Retryable {
		t.Fatal(err)
	}
	realMissing := &Error{Status: 404, Code: "not_found", Message: "agent unknown not found"}
	if coordinationError(realMissing, 0) != realMissing {
		t.Fatal("legitimate not_found became unsupported")
	}
	unsupported := &Error{Status: 501, Code: "unsupported", Retryable: true}
	if coordinationError(unsupported, 0).(*Error).Retryable {
		t.Fatal("unsupported incorrectly retryable")
	}
}
