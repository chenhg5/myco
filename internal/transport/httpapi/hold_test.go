package httpapi_test

import (
	"testing"

	"github.com/myco-ai/myco/internal/app"
)

func TestRunnerWakesHeldTaskOnDelegatedReply(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	alice := h.register("alice", nil)
	bob := h.register("bob", nil)
	carol := h.register("carol", nil)
	parent := h.send(alice, "bob", "original task")
	h.do("POST", "/v1/inbox/pull", bob, nil).want(t, 200)
	h.do("POST", "/v1/messages/"+parent+"/ack", bob, nil).want(t, 200)
	child := h.send(bob, "carol", "delegated task")
	route := "/v1/messages/" + parent + "/hold-until"
	h.do("POST", route, bob, map[string]string{"until": parent}).wantErr(t, 400, "invalid_argument")
	h.do("POST", route+"?dry_run=1", bob, map[string]string{"until": child}).want(t, 200)
	if got := str(h.do("GET", "/v1/messages/"+parent, bob, nil).want(t, 200).body, "status"); got != "acked" {
		t.Fatalf("dry-run changed status to %s", got)
	}
	held := h.do("POST", route, bob, map[string]string{"until": child}).want(t, 200)
	if str(held.body, "message", "held_for") != "reply" {
		t.Fatalf("reply dependency missing: %s", held.raw)
	}
	runnerPull := func() []map[string]any {
		return messages(t, h.do("POST", "/v1/inbox/pull?wake_held=1", bob, map[string]any{"via": "runner"}).want(t, 200))
	}
	if got := runnerPull(); len(got) != 0 {
		t.Fatalf("woke before reply: %v", got)
	}
	h.do("POST", "/v1/inbox/pull", carol, nil).want(t, 200)
	reply := h.do("POST", "/v1/messages/"+child+"/reply", carol, map[string]string{"body": "delegation result"}).want(t, 201)
	wake := str(reply.body, "message", "id")
	if got := runnerPull(); len(got) != 1 || str(got[0], "id") != parent || str(got[0], "hold_wake") != wake {
		t.Fatalf("expected original task with its reply: %v", got)
	}
	if got := runnerPull(); len(got) != 0 {
		t.Fatalf("duplicate mail: %v", got)
	}
	h.do("POST", "/v1/messages/"+parent+"/ack", bob, nil).want(t, 200)
	h.do("POST", "/v1/messages/"+parent+"/reply", bob, map[string]string{"body": "original done"}).want(t, 201)
}

func TestRunnerWakesOriginalTaskOnAnswer(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	alice := h.register("alice", nil)
	bob := h.register("bob", nil)
	parent := h.send(alice, "bob", "original task")
	h.do("POST", "/v1/inbox/pull", bob, nil).want(t, 200)
	question := h.do("POST", "/v1/messages/"+parent+"/progress", bob, map[string]any{"body": "pick one", "ask": true}).want(t, 201)
	if str(question.body, "message", "id") == "" {
		t.Fatalf("missing question: %s", question.raw)
	}
	answer := h.do("POST", "/v1/messages/"+parent+"/steer", alice, map[string]string{"body": "the answer"}).want(t, 201)
	wake := str(answer.body, "message", "id")
	// The steer-only watcher may poll first. It must not consume the answer
	// as an independent turn or settle a still-held task in the old run.
	pull := "/v1/inbox/pull?wake_held=1"
	got := messages(t, h.do("POST", pull, bob, map[string]any{"via": "runner", "types": []string{"steer"}}).want(t, 200))
	if len(got) != 0 {
		t.Fatalf("answer leaked into steer watcher: %v", got)
	}
	got = messages(t, h.do("POST", pull, bob, map[string]any{"via": "runner"}).want(t, 200))
	if len(got) != 1 || str(got[0], "id") != parent || str(got[0], "hold_wake") != wake {
		t.Fatalf("original task was not resumed: %v", got)
	}
}
