package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/chenhg5/myco/internal/api"
)

func TestRuntimeProviderHostOlderHub(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		// Reproduce the older Hub's real strict JSON decoder and error shape.
		var old struct {
			CLI      string `json:"cli"`
			Provider string `json:"provider,omitempty"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		w.Header().Set("Content-Type", "application/json")
		if err := decoder.Decode(&old); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(api.Error{Code: "invalid_argument", Message: fmt.Sprintf("invalid JSON body: %v", err)})
			return
		}
		if old.CLI != "codex" || old.Provider != "magpie 网关" {
			t.Error("fallback discarded old runtime fields")
		}
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()
	c, _ := New(srv.URL, "token")
	if err := c.ReportRuntime(context.Background(), api.Runtime{CLI: "codex", Provider: "magpie 网关", ProviderHost: "127.0.0.1:3425"}); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatal("expected exactly one compatibility retry")
	}
}

func TestRuntimeProviderHostDoesNotRetryOtherErrors(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"error":"invalid_argument","message":"other validation error"}`))
			}))
			defer srv.Close()
			c, _ := New(srv.URL, "token")
			if err := c.ReportRuntime(context.Background(), api.Runtime{CLI: "codex", ProviderHost: "127.0.0.1:3425"}); err == nil {
				t.Fatal("expected error")
			}
			if calls.Load() != 1 {
				t.Fatal("unrelated error retried")
			}
		})
	}
}
