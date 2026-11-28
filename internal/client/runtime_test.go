package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
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

func TestRuntimeProvidersEmptyListOlderHubHidesControls(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var old struct {
			CLI          string   `json:"cli"`
			ProviderHost string   `json:"provider_host,omitempty"`
			Features     []string `json:"features,omitempty"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		w.Header().Set("Content-Type", "application/json")
		if err := decoder.Decode(&old); err != nil {
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(api.Error{Code: "invalid_argument", Message: fmt.Sprintf("invalid JSON body: %v", err)})
			return
		}
		if old.ProviderHost != "127.0.0.1:3425" {
			t.Error("older Hub lost a provider host it already supports")
		}
		for _, f := range old.Features {
			if f == "remote_provider" {
				t.Error("old Hub advertised unsupported remote controls")
			}
		}
		w.Write([]byte("{}"))
	}))
	defer srv.Close()
	c, _ := New(srv.URL, "token")
	options := []api.ProviderOption{}
	if err := c.ReportRuntime(context.Background(), api.Runtime{CLI: "codex", ProviderHost: "127.0.0.1:3425", Features: []string{"remote_provider"}, Providers: &options}); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatal("empty inventory did not probe older Hub compatibility")
	}
}

func TestRuntimeSlotsOlderHubStrictDecoder(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var old struct {
			CLI   string `json:"cli"`
			Model string `json:"model"`
		}
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&old); err != nil {
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(api.Error{Code: "invalid_argument", Message: err.Error()})
			return
		}
		if old.CLI != "codex" || old.Model != "example" {
			t.Error("lost old runtime fields")
		}
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c, _ := New(srv.URL, "test")
	if err := c.ReportRuntime(context.Background(), api.Runtime{CLI: "codex", Model: "example", RunnerSlots: &api.RunnerSlots{MaxConcurrent: 3, InUse: 3, Waiting: 2}}); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("got %d requests", calls.Load())
	}
}
func TestRuntimeSlotsDoesNotDowngradeOtherErrors(t *testing.T) {
	for _, status := range []int{400, 401, 403, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(status)
				json.NewEncoder(w).Encode(api.Error{Code: "invalid_argument", Message: "unrelated validation"})
			}))
			defer srv.Close()
			c, _ := New(srv.URL, "test")
			if err := c.ReportRuntime(context.Background(), api.Runtime{RunnerSlots: &api.RunnerSlots{MaxConcurrent: 3}}); err == nil {
				t.Fatal("error swallowed")
			}
			if calls.Load() != 1 {
				t.Fatal("retried unrelated error")
			}
		})
	}
}

func TestRuntimeRestartFieldsFallBackOnlyForOlderSchema(t *testing.T) {
	for _, status := range []int{400, 403} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body map[string]any
				json.NewDecoder(r.Body).Decode(&body)
				if calls == 1 {
					w.WriteHeader(status)
					fmt.Fprint(w, `{"error":"invalid_argument","message":"invalid JSON body: json: unknown field \"runner_instance\""}`)
					return
				}
				if body["runner_instance"] != nil || body["restarting_until"] != nil || body["away"] != "updating" || body["model"] != "demo" {
					t.Errorf("fallback lost fields: %+v", body)
				}
				fmt.Fprint(w, `{}`)
			}))
			defer srv.Close()
			c, _ := New(srv.URL, "token")
			err := c.ReportRuntime(context.Background(), api.Runtime{RunnerInstance: "process", Away: "updating", RestartingUntil: time.Now().Add(time.Minute), Model: "demo"})
			if status == 400 && (err != nil || calls != 2) {
				t.Fatalf("old Hub fallback: %v calls%d", err, calls)
			}
			if status == 403 && (err == nil || calls != 1) {
				t.Fatalf("auth failure retried: %v calls%d", err, calls)
			}
		})
	}
}

func TestPrivateQuotaDoesNotDowngradeToOlderPublicRuntime(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var old struct {
			CLI   string `json:"cli"`
			Quota *struct {
				Source  string            `json:"source"`
				At      time.Time         `json:"at"`
				Windows []api.QuotaWindow `json:"windows"`
			} `json:"quota"`
		}
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		w.Header().Set("Content-Type", "application/json")
		if e := dec.Decode(&old); e != nil {
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(api.Error{Code: "invalid_argument", Message: e.Error()})
			return
		}
		if old.Quota != nil {
			t.Fatal("private snapshot downgraded to public legacy field")
		}
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c, _ := New(srv.URL, "test-token")
	if e := c.ReportRuntime(context.Background(), api.Runtime{CLI: "codex", Quota: &api.Quota{Source: "codex", Provider: "codex", ProfileRef: "opaque-ref", At: time.Now(), Windows: []api.QuotaWindow{{Name: "primary", Used: .82}}}}); e != nil {
		t.Fatal(e)
	}
	if calls.Load() != 2 {
		t.Fatalf("expected one schema fallback, got %d", calls.Load())
	}
}

// A hub from before provider model lists keeps the models, without where
// they came from (it refuses unknown fields).
func TestRuntimeModelSourceFallsBackForOlderHubs(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["models_from"] != nil || body["models_at"] != nil {
			w.WriteHeader(400)
			fmt.Fprint(w, `{"error":"invalid_argument","message":"invalid JSON body: json: unknown field \"models_from\""}`)
			return
		}
		if models, _ := body["models"].([]any); len(models) != 1 {
			t.Errorf("fallback lost the models: %+v", body)
		}
		fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()
	c, _ := New(srv.URL, "token")
	at := time.Now()
	if err := c.ReportRuntime(context.Background(), api.Runtime{CLI: "claude", Models: []api.RuntimeModel{{ID: "kimi-k3"}}, ModelsFrom: "provider", ModelsAt: &at}); err != nil || calls != 2 {
		t.Fatalf("old Hub fallback: %v calls %d", err, calls)
	}
}

func TestRuntimeAccountOlderHubAndOtherErrors(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		message string
		calls   int
	}{{"old-hub", 400, `unknown field "account"`, 2}, {"validation", 400, "invalid runtime", 1}, {"unauthorized", 401, `unknown field "account"`, 1}} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := calls.Add(1)
				var body map[string]any
				json.NewDecoder(r.Body).Decode(&body)
				if n == 1 {
					w.WriteHeader(tc.status)
					json.NewEncoder(w).Encode(api.Error{Code: "invalid_argument", Message: tc.message})
					return
				}
				if _, ok := body["account"]; ok {
					t.Error("legacy retry retained account")
				}
				if body["cli"] != "claude" || body["model"] != "test-model" {
					t.Error("legacy retry dropped ordinary fields")
				}
				w.Write([]byte("{}"))
			}))
			defer srv.Close()
			c, _ := New(srv.URL, "fixture")
			err := c.ReportRuntime(context.Background(), api.Runtime{CLI: "claude", Model: "test-model", Account: "alice@example.com"})
			if (err == nil) != (tc.name == "old-hub") || int(calls.Load()) != tc.calls {
				t.Fatal("compatibility retry mismatch")
			}
		})
	}
}

func TestRuntimeNewQuotaOlderHub(t *testing.T) {
	for _, message := range []string{`invalid quota provider`, `invalid JSON body: json: unknown field "balance"`, `unrelated validation failure`} {
		t.Run(message, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var rt api.Runtime
				if e := json.NewDecoder(r.Body).Decode(&rt); e != nil {
					t.Fatal(e)
				}
				if calls == 1 {
					w.WriteHeader(400)
					json.NewEncoder(w).Encode(api.Error{Code: "invalid_argument", Message: message})
					return
				}
				if rt.Quota != nil || rt.CLI != "codex" || rt.Model != "test-model" {
					t.Error("fallback lost runtime or exposed private quota")
				}
				w.Write([]byte(`{}`))
			}))
			defer srv.Close()
			c, _ := New(srv.URL, "token")
			e := c.ReportRuntime(context.Background(), api.Runtime{CLI: "codex", Model: "test-model", Quota: &api.Quota{Provider: "openrouter", ProfileRef: "qp_opaque", Windows: []api.QuotaWindow{{Name: "balance", Balance: &api.QuotaBalance{Currency: "USD", Unlimited: true}}}}})
			if message == "unrelated validation failure" {
				if e == nil || calls != 1 {
					t.Fatal("unrelated validation was retried")
				}
			} else if e != nil || calls != 2 {
				t.Fatalf("compatibility fallback failed: %v calls=%d", e, calls)
			}
		})
	}
}
