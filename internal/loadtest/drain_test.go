package loadtest

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/client"
)

func TestDrainChecksCompletionAtDeadline(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		workerDone, taskDone bool
		want                 string
	}{
		{"finished", true, true, ""},
		{"queued housekeeping", false, true, "drain_deadline"},
		{"task validation pending", true, false, "drain_deadline"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			w := &workload{tasks: map[string]*task{"task": {completed: tc.taskDone}}}
			workersDone := make(chan struct{})
			if tc.workerDone {
				close(workersDone)
			}
			deadline := make(chan time.Time, 1)
			deadline <- time.Time{}
			// No ticks: completion must not depend on a future polling tick, even
			// when the deadline and completed work are simultaneously observable.
			if got := w.waitDrain(ctx, workersDone, deadline, nil); got != tc.want {
				t.Fatalf("reason=%q, want %q", got, tc.want)
			}
			if ctx.Err() != nil {
				t.Fatal("drain waited for the test safety timeout")
			}
		})
	}
}

func TestDrainDoesNotWaitForEmptyPollAndCancelsItOnExit(t *testing.T) {
	for _, backlog := range []bool{false, true} {
		t.Run(map[bool]string{false: "completed", true: "backlog deadline"}[backlog], func(t *testing.T) {
			entered, canceled := make(chan struct{}), make(chan struct{})
			release := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				_ = r.Body.Close()
				close(entered)
				select {
				case <-r.Context().Done():
					close(canceled)
				case <-release:
				}
			}))
			defer server.Close()
			defer close(release)
			c, err := client.New(server.URL, "fixture-token")
			if err != nil {
				t.Fatal(err)
			}
			ctx, stop := context.WithCancel(t.Context())
			defer stop()
			b := &bot{auth: identity{c: c}}
			w := &workload{tasks: map[string]*task{}, start: time.Now()}
			pollDone := make(chan struct{})
			go func() { defer close(pollDone); w.poll(ctx, b) }()
			await := func(ch <-chan struct{}) {
				t.Helper()
				select {
				case <-ch:
				case <-time.After(5 * time.Second):
					t.Fatal("poll lifecycle did not finish")
				}
			}
			await(entered)
			workersDone := make(chan struct{})
			deadline := make(chan time.Time, 1)
			want := ""
			if backlog {
				want = "drain_deadline"
				deadline <- time.Time{}
			} else {
				close(workersDone)
			}
			reason := w.waitDrain(ctx, workersDone, deadline, nil)
			if reason != want {
				t.Fatalf("reason=%q, want %q", reason, want)
			}
			if w.activePolls.Load() != 1 {
				t.Fatal("test did not hold an empty inbox poll")
			}
			// This is the same order as runOne: decide drain, cancel, join pollers.
			stop()
			await(pollDone)
			await(canceled)
			if w.activePolls.Load() != 0 {
				t.Fatal("poll leaked after cancellation")
			}
			for _, op := range w.stats.snapshot() {
				if len(op.Errors) != 0 || op.Canceled != 1 {
					t.Fatalf("shutdown was counted as a request failure: %+v", op)
				}
			}
		})
	}
}
