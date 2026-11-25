package runner

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/harness"
)

func TestSuccessfulTaskEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, kind, handled string
		runErr              error
		want                bool
	}{
		{name: "task", kind: "task", want: true},
		{name: "self reply", kind: "task", handled: "done", want: true},
		{name: "failed", kind: "task", runErr: errors.New("failed")},
		{name: "self failed", kind: "task", handled: "failed"},
		{name: "held", kind: "task", handled: "held"},
		{name: "note", kind: "note"},
		{name: "busy", kind: "task", runErr: harness.ErrSessionBusy},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "runner.json")
			m := api.Message{ID: "demo", Type: tc.kind, From: "sender", ThreadID: "demo"}
			hub := newFakeHub(m)
			h := fakeHarness{fn: func(harness.Request) (harness.Result, error) {
				if tc.handled != "" {
					_ = hub.set(m.ID, tc.handled)
				}
				return harness.Result{Output: "fixture output"}, tc.runErr
			}}
			now := time.Now()
			r := New(hub, h, NewMemorySessions(), nil, Options{Self: "demo", StatePath: path, Now: func() time.Time { return now }}, quiet())
			r.Handle(context.Background(), m)
			got := ReadSuccess(path)
			if (got != nil) != tc.want {
				t.Fatalf("success metadata present=%v want=%v", got != nil, tc.want)
			}
			if !tc.want {
				return
			}
			if got.CLI != "fake" || !got.At.Equal(now) {
				t.Fatal("wrong success metadata")
			}
			restarted := New(hub, h, NewMemorySessions(), nil, r.opts, quiet())
			restarted.mu.Lock()
			restarted.saveState()
			restarted.mu.Unlock()
			if ReadSuccess(path) == nil {
				t.Fatal("restart discarded success evidence")
			}
			if err := MarkOwnRun(path, "session", now); err != nil {
				t.Fatal(err)
			}
			if ReadSuccess(path) == nil {
				t.Fatal("session activity discarded success evidence")
			}
		})
	}
}
