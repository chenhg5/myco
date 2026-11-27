package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTaskBoardOldHubUnsupported(t *testing.T) {
	old := httptest.NewServer(http.NotFoundHandler())
	defer old.Close()
	c, _ := New(old.URL, "fake-token")
	_, err := c.TaskBoard(context.Background(), "", false, 15*time.Minute)
	ce, ok := AsError(err)
	if !ok || ce.Code != "unsupported" || ce.Retryable || ce.Suggestion == "" {
		t.Fatalf("old hub: %+v", err)
	}
}
func TestTaskPolicyNotFoundNotMaskedAsUnsupported(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		w.Write([]byte(`{"error":"not_found","message":"project missing","retryable":false}`))
	}))
	defer srv.Close()
	c, _ := New(srv.URL, "fake-token")
	_, err := c.TaskAlertPolicy(context.Background(), "missing", nil, false)
	ce, ok := AsError(err)
	if !ok || ce.Code != "not_found" {
		t.Fatal(err)
	}
}
