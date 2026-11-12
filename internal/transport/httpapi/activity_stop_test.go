package httpapi_test

import (
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/app"
)

func TestTaskActivityAndSenderStop(t *testing.T) {
	h := newHub(t, app.DefaultMessagePolicy())
	alice := h.register("alice", nil)
	bob := h.register("bob", nil)
	carol := h.register("carol", nil)
	sent := h.do("POST", "/v1/messages", alice, map[string]any{"to": "bob", "body": "work"}).want(t, 201)
	id := sent.body["message"].(map[string]any)["id"].(string)
	h.do("POST", "/v1/inbox/pull", bob, nil).want(t, 200)
	h.do("POST", "/v1/messages/"+id+"/ack", bob, nil).want(t, 200)
	first := h.do("GET", "/v1/messages/"+id, alice, nil).want(t, 200)
	a1 := first.body["active_at"].(string)
	time.Sleep(2 * time.Millisecond)
	h.do("GET", "/v1/messages/"+id, bob, nil).want(t, 200)
	second := h.do("GET", "/v1/messages/"+id, alice, nil).want(t, 200)
	a2 := second.body["active_at"].(string)
	if a1 == a2 {
		t.Fatalf("recipient polling did not advance active_at: %s", a1)
	}
	time.Sleep(2 * time.Millisecond)
	h.do("POST", "/v1/messages/"+id+"/progress", bob, map[string]any{"body": "halfway"}).want(t, 201)
	third := h.do("GET", "/v1/messages/"+id, alice, nil).want(t, 200)
	if third.body["active_at"] == a2 {
		t.Fatal("progress did not advance active_at")
	}
	h.do("POST", "/v1/messages/"+id+"/stop", carol, nil).wantErr(t, 403, "permission_denied")
	h.do("POST", "/v1/messages/"+id+"/stop?dry_run=true", alice, nil).want(t, 200)
	if m := h.do("GET", "/v1/messages/"+id, alice, nil).want(t, 200); m.body["status"] != "acked" {
		t.Fatalf("dry run changed task: %s", m.raw)
	}
	stopped := h.do("POST", "/v1/messages/"+id+"/stop", alice, nil).want(t, 200)
	if stopped.body["message"].(map[string]any)["error"] != "stopped by the sender" {
		t.Fatalf("wrong stop semantics: %s", stopped.raw)
	}
}
