package integration_test

import (
	"path/filepath"
	"testing"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
)

// A task's recipient offers its sender a next message; the sender sees it
// on the task (the app puts it in the input). Only the recipient sets it.
func TestSuggestedNextInput(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	lead := h.client(h.register(api.RegisterRequest{Name: "lead", Binding: api.Binding{Harness: "claude", SessionID: "s1"}}))
	docs := h.client(h.register(api.RegisterRequest{Name: "docs", Binding: api.Binding{Harness: "claude", SessionID: "s2"}}))

	sent, err := lead.Send(c, api.SendRequest{To: "docs", Type: "task", Body: "fix the login page"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lead.Suggest(c, sent.Message.ID, "not mine to say", false); err == nil {
		t.Fatal("only the recipient suggests")
	}
	docs.Pull(c, 1, 0)
	if m, err := docs.Suggest(c, sent.Message.ID, "Also check the signup page", false); err != nil || m.Suggest != "Also check the signup page" {
		t.Fatalf("suggest: %v %+v", err, m)
	}
	if _, err := docs.Reply(c, sent.Message.ID, "fixed", false); err != nil {
		t.Fatal(err)
	}
	th, err := h.admin().Thread(c, sent.Message.ThreadID)
	if err != nil || len(th) < 1 || th[0].ID != sent.Message.ID || th[0].Suggest != "Also check the signup page" {
		t.Fatalf("the sender sees it on the task: %v %+v", err, th)
	}
	// An empty one clears it, after the answer too.
	if m, err := docs.Suggest(c, sent.Message.ID, "", false); err != nil || m.Suggest != "" {
		t.Fatalf("clear: %v %+v", err, m)
	}
}
