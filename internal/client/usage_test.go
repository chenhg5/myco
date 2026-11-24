package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/myco-ai/myco/internal/api"
)

func TestUsageOldHub404IsSkippedWithoutRetry(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); http.NotFound(w, r) }))
	defer srv.Close()
	c, _ := New(srv.URL, "fixture")
	out, err := c.PostUsage(context.Background(), api.UsageRequest{Reports: []api.UsageReport{{Day: "2026-11-24", Calls: 1}}})
	if err != nil || out != nil || calls.Load() != 1 {
		t.Fatalf("old Hub %v %v requests=%d", out, err, calls.Load())
	}
}
