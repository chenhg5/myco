package client

import (
	"context"
	"github.com/myco-ai/myco/internal/api"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestManagedForkCompatibilityDoesNotMaskMissingSource(t *testing.T) {
	for _, tc := range []struct {
		name       string
		code       int
		body, want string
	}{{"old route", 404, "404 page not found", "unsupported"}, {"missing source", 404, `{"error":"not_found","message":"source not found"}`, "not_found"}, {"old runner", 501, `{"error":"unsupported","message":"upgrade runner"}`, "unsupported"}, {"forbidden", 403, `{"error":"permission_denied","message":"not yours"}`, "permission_denied"}} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.code); w.Write([]byte(tc.body)) }))
			defer srv.Close()
			c, _ := New(srv.URL, "fixture")
			_, err := c.ForkAgents(context.Background(), api.AgentForkRequest{From: "source", RequestID: "batch"}, false)
			e, ok := AsError(err)
			if !ok || e.Code != tc.want {
				t.Fatalf("%v", err)
			}
			if e.Code == "unsupported" && e.Retryable {
				t.Fatal("unsupported must not retry")
			}
		})
	}
}
