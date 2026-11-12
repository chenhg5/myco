package runner

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/harness"
)

type rejectedHub struct {
	*fakeHub
	pulls int
}

func (h *rejectedHub) Pull(context.Context, int, time.Duration) ([]api.Message, error) {
	h.pulls++
	return nil, &client.Error{Status: 401, Code: "unauthenticated"}
}

func TestRunnerStopsOnRevokedCredential(t *testing.T) {
	h := &rejectedHub{fakeHub: newFakeHub()}
	r := New(h, fakeHarness{fn: func(harness.Request) (harness.Result, error) { t.Fatal("harness ran"); return harness.Result{}, nil }}, NewMemorySessions(), nil,
		Options{Self: "worker", HubURL: "https://hub.example"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := r.Run(context.Background()); !IsCredentialRejected(err) || h.pulls != 1 {
		t.Fatalf("runner retried revoked credentials: pulls=%d err=%v", h.pulls, err)
	}
}
