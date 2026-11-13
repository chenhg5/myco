package integration_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
)

// A command another agent sent to a shell node, picked up and then lost
// (the machine slept), can be stopped from the app by whoever manages the
// node; the sender is told instead of waiting forever.
func TestConsoleStopsAnotherAgentsTask(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	caller := h.client(h.register(api.RegisterRequest{Name: "caller"}))
	node := h.client(h.register(api.RegisterRequest{Name: "box", Binding: api.Binding{Harness: "exec"}}))

	task, err := caller.Send(c, api.SendRequest{To: "box", Type: "task", Body: "echo awake"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := node.Pull(c, 1, 0); err != nil {
		t.Fatal(err)
	}
	if err := node.Ack(c, task.Message.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.client("").ConsoleStop(c, task.Message.ID); err == nil {
		t.Fatal("stop needs a console credential")
	}
	m, err := h.admin().ConsoleStop(c, task.Message.ID)
	if err != nil || m.Status != "failed" || !strings.HasPrefix(m.Error, "stopped in the app by") {
		t.Fatalf("console stop: %v %+v", err, m)
	}
	got, err := caller.WaitReply(c, task.Message.ID, 0)
	if err != nil || got == nil || got.Type != "event" {
		t.Fatalf("the sender must hear about it: %v %+v", err, got)
	}
	if _, err := h.admin().ConsoleStop(c, task.Message.ID); err == nil {
		t.Fatal("stopping a closed task must fail")
	}
}
