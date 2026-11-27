package client

import (
	"context"
	"encoding/json"
	"github.com/myco-ai/myco/internal/api"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type healthRoundTripper func(*http.Request) (*http.Response, error)

func (f healthRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestMachineHealthCompatibilityOnlyRetriesUnknownField(t *testing.T) {
	for _, status := range []int{400, 403} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			c, e := New("https://test.invalid", "test-credential")
			if e != nil {
				t.Fatal(e)
			}
			calls := 0
			c.http.Transport = healthRoundTripper(func(r *http.Request) (*http.Response, error) {
				calls++
				var rt api.Runtime
				if e = json.NewDecoder(r.Body).Decode(&rt); e != nil {
					t.Fatal(e)
				}
				responseStatus := status
				body := `{"error":"invalid_argument","message":"unknown field \"machine_health\""}`
				if calls > 1 {
					if rt.MachineHealth != nil || rt.CLI != "codex" {
						t.Fatal("retry changed unrelated fields")
					}
					responseStatus = 200
					body = `{}`
				}
				return &http.Response{StatusCode: responseStatus, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})
			e = c.ReportRuntime(context.Background(), api.Runtime{CLI: "codex", MachineHealth: &api.MachineHealth{At: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}})
			if status == 400 && (e != nil || calls != 2) {
				t.Fatalf("old-hub fallback: %d %v", calls, e)
			}
			if status == 403 && (e == nil || calls != 1) {
				t.Fatalf("authentication failure was retried: %d %v", calls, e)
			}
		})
	}
}

func TestRuntimeCompatibilityCombinesMainAndHealthFields(t *testing.T) {
	c, err := New("https://test.invalid", "test-credential")
	if err != nil {
		t.Fatal(err)
	}
	fields := []string{"account", "profile_ref", "runner_instance", "machine_health", "runner_slots", "models_from", "provider_host"}
	calls := 0
	c.http.Transport = healthRoundTripper(func(r *http.Request) (*http.Response, error) {
		calls++
		status, body := 200, `{}`
		if calls <= len(fields) {
			status = 400
			b, _ := json.Marshal(map[string]string{"error": "invalid_argument", "message": "unknown field \"" + fields[calls-1] + "\""})
			body = string(b)
		}
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})
	err = c.ReportRuntime(context.Background(), api.Runtime{CLI: "codex", Account: "test@example.invalid", Quota: &api.Quota{ProfileRef: "profile"}, RunnerInstance: "runner", MachineHealth: &api.MachineHealth{}, RunnerSlots: &api.RunnerSlots{}, ModelsFrom: "cli", ProviderHost: "test.invalid"})
	if err != nil || calls != 8 {
		t.Fatalf("combined legacy fallback calls=%d error=%v", calls, err)
	}
}
