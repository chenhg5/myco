package integration_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/client"
)

// An answer that arrives after its caller stopped waiting must not sit in
// the caller's inbox forever: exec results are closed when stored, anything
// else can be dismissed without pulling it, by the node or from the console.
func TestStaleAnswersDoNotLinger(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	caller := h.client(h.register(api.RegisterRequest{Name: "caller"}))
	node := h.client(h.register(api.RegisterRequest{Name: "box", Binding: api.Binding{Harness: "exec"}}))
	helper := h.client(h.register(api.RegisterRequest{Name: "helper"}))

	// The caller gives up waiting; the exec node answers later.
	task, err := caller.Send(c, api.SendRequest{To: "box", Type: "task", Body: "uptime"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := node.Pull(c, 1, 0); err != nil {
		t.Fatal(err)
	}
	r, err := node.Reply(c, task.Message.ID, `{"exit_code":0,"stdout":"up"}`, false)
	if err != nil || r.Status != "done" {
		t.Fatalf("exec result should be stored closed: %v %+v", err, r)
	}
	if got, err := caller.Pull(c, 10, 0); err != nil || len(got) != 0 {
		t.Fatalf("a late exec result must not reach the inbox: %v %+v", err, got)
	}
	// Whoever still waits gets it.
	if got, err := caller.WaitReply(c, task.Message.ID, 0); err != nil || got == nil || got.ID != r.ID {
		t.Fatalf("wait after the fact: %v %+v", err, got)
	}
	// A failed command too.
	task2, _ := caller.Send(c, api.SendRequest{To: "box", Type: "task", Body: "false"}, false)
	if _, err := node.Pull(c, 1, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := node.Fail(c, task2.Message.ID, "boom", false); err != nil {
		t.Fatal(err)
	}
	if got, _ := caller.Pull(c, 10, 0); len(got) != 0 {
		t.Fatalf("an exec failure must not reach the inbox: %+v", got)
	}

	// Other answers still arrive; the recipient can drop one unpulled.
	ask, _ := caller.Send(c, api.SendRequest{To: "helper", Type: "task", Body: "help"}, false)
	if _, err := helper.Pull(c, 1, 0); err != nil {
		t.Fatal(err)
	}
	late, err := helper.Reply(c, ask.Message.ID, "here you go", false)
	if err != nil || late.Status != "queued" {
		t.Fatalf("a normal reply is queued: %v %+v", err, late)
	}
	if err := caller.Complete(c, late.ID); err != nil {
		t.Fatalf("done on a queued message: %v", err)
	}
	if got, _ := caller.Pull(c, 10, 0); len(got) != 0 {
		t.Fatalf("dismissed message delivered: %+v", got)
	}

	// From the console: a hub admin dismisses a queued message for its node.
	note, _ := helper.Send(c, api.SendRequest{To: "caller", Type: "note", Body: "fyi"}, false)
	m, err := h.admin().ConsoleDismiss(c, note.Message.ID)
	if err != nil || m.Status != "done" {
		t.Fatalf("console dismiss: %v %+v", err, m)
	}
	if _, err := h.admin().ConsoleDismiss(c, note.Message.ID); err != nil {
		t.Fatalf("dismissing twice is harmless: %v", err)
	}
	if _, err := h.client("").ConsoleDismiss(c, note.Message.ID); err == nil {
		t.Fatal("dismiss needs a console credential")
	}
	var ce *client.Error
	if _, err := h.admin().ConsoleDismiss(c, "msg_nope"); !errors.As(err, &ce) || ce.Code != "not_found" {
		t.Fatalf("unknown message: %v", err)
	}
}
