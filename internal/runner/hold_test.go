package runner

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/harness"
)

type wakeHub struct {
	*fakeHub
	wake api.Message
}

func (h wakeHub) GetMessage(ctx context.Context, id string) (*api.Message, error) {
	if id == h.wake.ID {
		return &h.wake, nil
	}
	return h.fakeHub.GetMessage(ctx, id)
}

func TestRunnerResumesOriginalTaskWithWakeMessage(t *testing.T) {
	original := api.Message{ID: "original", Type: "task", From: "alice", ThreadID: "thread", Body: "original request", HoldWake: "reply"}
	hub := wakeHub{newFakeHub(original), api.Message{ID: "reply", Type: "reply", From: "carol", Body: "delegated result"}}
	sessions := NewMemorySessions()
	if err := sessions.Put("thread", "original-session"); err != nil {
		t.Fatal(err)
	}
	runner := New(hub, fakeHarness{fn: func(req harness.Request) (harness.Result, error) {
		if req.SessionID != "original-session" || !strings.Contains(req.Prompt, "delegated result") ||
			!slices.Contains(req.Env, "MYCO_MESSAGE_ID=original") || !slices.Contains(req.Env, "MYCO_RUNNER_HOLD=1") {
			t.Errorf("wrong session or wake context: %s %s", req.SessionID, req.Prompt)
		}
		return harness.Result{Output: "finished original"}, nil
	}}, sessions, nil, Options{Once: true, Scope: ScopeThread}, quiet())
	if err := runner.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if hub.replies[original.ID] != "finished original" {
		t.Fatalf("did not answer original: %+v", hub.replies)
	}
}
