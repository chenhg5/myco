package runner

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/harness"
)

type flakyFinishHub struct {
	*fakeHub
	getFailures, replyFailures int
	getCalls, replyCalls       int
}

func (h *flakyFinishHub) GetMessage(ctx context.Context, id string) (*api.Message, error) {
	h.getCalls++
	if h.getCalls <= h.getFailures {
		return nil, &client.Error{Code: client.CodeUnavailable, Retryable: true}
	}
	return h.fakeHub.GetMessage(ctx, id)
}

func (h *flakyFinishHub) Reply(ctx context.Context, id, body string, dry bool) (*api.Message, error) {
	h.replyCalls++
	if h.replyCalls <= h.replyFailures {
		return nil, io.EOF
	}
	return h.fakeHub.Reply(ctx, id, body, dry)
}

func TestFinishedTaskRetriesGetMessageAndReply(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		getFailures, replyFailures int
	}{
		{"get message after two failures", 2, 0},
		{"reply after one failure", 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := api.Message{ID: "t1", Type: "task", From: "alice", ThreadID: "th"}
			hub := &flakyFinishHub{fakeHub: newFakeHub(m), getFailures: tc.getFailures, replyFailures: tc.replyFailures}
			ctx, cancel := context.WithCancel(context.Background())
			h := fakeHarness{fn: func(harness.Request) (harness.Result, error) {
				cancel() // delivery must use an independent context
				return harness.Result{Output: "finished"}, nil
			}}
			r := New(hub, h, NewMemorySessions(), nil, Options{}, quiet())
			r.finishRetryMin, r.finishTimeout = time.Millisecond, time.Second
			r.Handle(ctx, m)
			if hub.getCalls != tc.getFailures+1 || hub.replyCalls != tc.replyFailures+1 || hub.replies[m.ID] != "finished" {
				t.Fatalf("get=%d reply=%d delivered=%q", hub.getCalls, hub.replyCalls, hub.replies[m.ID])
			}
		})
	}
}

func TestFinishedTaskLogsUndeliveredResultAfterRetryBudget(t *testing.T) {
	m := api.Message{ID: "t1", Type: "task", From: "alice", ThreadID: "th"}
	hub := &flakyFinishHub{fakeHub: newFakeHub(m), getFailures: 1000}
	var logs bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logs, nil))
	h := fakeHarness{fn: func(harness.Request) (harness.Result, error) {
		return harness.Result{Output: "partial result myco_enroll_secret123"}, nil
	}}
	r := New(hub, h, NewMemorySessions(), nil, Options{}, log)
	r.finishRetryMin, r.finishTimeout = time.Millisecond, 20*time.Millisecond
	r.Handle(context.Background(), m)
	text := logs.String()
	if !strings.Contains(text, "result not delivered to hub after retries") || !strings.Contains(text, "partial result") || strings.Contains(text, "myco_enroll_secret123") {
		t.Fatalf("missing recovery log or leaked token: %s", text)
	}
}

func TestRetryableFinishErrorDoesNotRetryClientErrors(t *testing.T) {
	if retryableFinishError(&client.Error{Code: client.CodeUnavailable, Status: 404, Retryable: true}) {
		t.Fatal("4xx errors must stop delivery retries")
	}
	if !retryableFinishError(&client.Error{Status: 503}) {
		t.Fatal("5xx errors should be retried even without a retryable flag")
	}
}
