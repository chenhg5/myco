package console

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPublicAccessWaitsForDelayedURLAndHealth(t *testing.T) {
	var probes atomic.Int32
	var reads int
	public := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := probes.Add(1)
		if r.URL.Path != "/v1/health" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if n == 1 {
			http.Error(w, "starting", 503)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true,"version":"v1"}`))
	}))
	defer public.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	got, err := waitPublicAccess(ctx, time.Millisecond, func(context.Context) string {
		reads++
		if reads < 3 {
			return ""
		}
		return public.URL
	})
	if err != nil || got != public.URL || reads < 4 || probes.Load() != 2 {
		t.Fatalf("url=%s err=%v reads=%d probes=%d", got, err, reads, probes.Load())
	}
}
func TestPublicAccessDoesNotMistakeLocalReadyForPublicReady(t *testing.T) {
	for _, kind := range []string{"empty", "unhealthy", "malformed"} {
		t.Run(kind, func(t *testing.T) {
			public := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"ok":false}`)) }))
			defer public.Close()
			value := public.URL
			if kind == "empty" {
				value = ""
			}
			if kind == "malformed" {
				value = "not-a-url"
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			got, err := waitPublicAccess(ctx, time.Millisecond, func(context.Context) string { return value })
			if err == nil || got != "" {
				t.Fatalf("false success: %s %v", got, err)
			}
		})
	}
	for _, bad := range []string{"", " ", "javascript:alert(1)", "https://user:secret@example.com", "https://"} {
		if validPublicURL(bad) {
			t.Errorf("accepted %q", bad)
		}
	}
	if !validPublicURL("https://hub.example.com/w/team") {
		t.Fatal("workspace path rejected")
	}
}
func TestPublicAccessHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := waitPublicAccess(ctx, time.Second, func(context.Context) string { return "" })
	if err == nil || !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("%v", err)
	}
}
