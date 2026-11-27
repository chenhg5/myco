package runner

import (
	"context"
	"testing"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/harness"
)

func TestManagedForkAuthorizationRefusalDoesNotRunModel(t *testing.T) {
	for _, transient := range []bool{false, true} {
		hub := newFakeHub(api.Message{ID: "task", Type: "task", From: "lead", To: "copy", Body: "work"})
		called := false
		r := New(hub, fakeHarness{fn: func(harness.Request) (harness.Result, error) { called = true; return harness.Result{}, nil }}, NewMemorySessions(), DefaultPrompt{}, Options{Self: "copy", Once: true, AuthorizeRun: func(context.Context, string) error {
			if transient {
				return &client.Error{Code: "unavailable", Retryable: true}
			}
			return &client.Error{Code: "permission_denied", Status: 403}
		}}, quiet())
		if err := r.Run(context.Background()); err != nil {
			t.Fatal(err)
		}
		if called {
			t.Fatal("authorization failure ran the model")
		}
		if transient && len(hub.fails) > 0 {
			t.Fatal("temporary query failure was treated as revoked permission")
		}
		if !transient && hub.status["task"] != "failed" {
			t.Fatal("permanent denial was not explained to sender")
		}
	}
}
