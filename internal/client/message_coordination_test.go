package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/myco-ai/myco/internal/api"
)

func TestSendParentOldHubFallbackIsPrecise(t *testing.T) {
	for _, unknown := range []string{"parent_id", "unrelated"} {
		t.Run(unknown, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var in map[string]any
				if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
					t.Error(err)
				}
				if calls == 1 {
					w.WriteHeader(http.StatusBadRequest)
					_ = json.NewEncoder(w).Encode(api.Error{Code: "invalid_argument", Message: `json: unknown field "` + unknown + `"`})
					return
				}
				if _, ok := in["parent_id"]; ok {
					t.Error("fallback still sends parent_id")
				}
				if in["body"] != "payload" || in["idempotency_key"] != "stable-key" {
					t.Error("fallback changed request payload")
				}
				_ = json.NewEncoder(w).Encode(api.SendResponse{Message: api.Message{ID: "task"}})
			}))
			defer srv.Close()
			c, err := New(srv.URL, "test-token")
			if err != nil {
				t.Fatal(err)
			}
			_, err = c.Send(context.Background(), api.SendRequest{To: "worker", Body: "payload", ParentID: "parent", IdempotencyKey: "stable-key"}, false)
			if unknown == "parent_id" {
				if err != nil || calls != 2 {
					t.Fatalf("parent fallback: calls=%d err=%v", calls, err)
				}
			} else if err == nil || calls != 1 {
				t.Fatalf("unrelated error retried: calls=%d err=%v", calls, err)
			}
		})
	}
}

func TestMessageCoordinationUnsupportedAndMissingTask(t *testing.T) {
	for _, structured := range []bool{false, true} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			if structured {
				_ = json.NewEncoder(w).Encode(api.Error{Code: "not_found", Message: "task not found"})
			} else {
				_, _ = w.Write([]byte("404 page not found\n"))
			}
		}))
		c, err := New(srv.URL, "test-token")
		if err != nil {
			t.Fatal(err)
		}
		_, treeErr := c.MessageTree(context.Background(), "missing", 200)
		_, moveErr := c.ReassignMessage(context.Background(), "missing", api.ReassignRequest{To: "worker"}, false)
		for _, e := range []error{treeErr, moveErr} {
			var ce *Error
			if !errors.As(e, &ce) {
				t.Fatalf("unstructured error: %v", e)
			}
			want := "unsupported"
			if structured {
				want = "not_found"
			}
			if ce.Code != want || ce.Retryable {
				t.Fatalf("compatibility error: %+v", ce)
			}
		}
		srv.Close()
	}
}

func TestReassignE2EUnsupportedIsNotRetryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotImplemented)
		_ = json.NewEncoder(w).Encode(api.Error{Code: "unsupported", Message: "E2E tasks cannot be reassigned", Suggestion: "stop + send to re-encrypt"})
	}))
	defer srv.Close()
	c, err := New(srv.URL, "test-token")
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.ReassignMessage(context.Background(), "sealed", api.ReassignRequest{To: "worker"}, false)
	var got *Error
	if !errors.As(err, &got) || got.Code != "unsupported" || got.Retryable || got.Suggestion != "stop + send to re-encrypt" {
		t.Fatalf("wrong E2E error: %+v", err)
	}
}
