package integration_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/e2e"
)

// With end-to-end encryption on, only a message's two sides and the people
// who own them read it: not the hub, not its admin token, not other members.
func TestEndToEndEncryption(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	admin := h.admin()
	known := &e2e.Known{Path: filepath.Join(t.TempDir(), "known.json")}

	// People and their app devices (each with keys).
	device := func(name string) *client.Client {
		inv, err := admin.InviteMember(c, api.InviteRequest{Name: name})
		if err != nil {
			t.Fatal(err)
		}
		p, err := h.client("").Pair(c, inv.Link[strings.Index(inv.Link, "/c/")+3:], name+"-mac")
		if err != nil {
			t.Fatal(err)
		}
		dc := h.client(p.Token)
		id, _ := e2e.NewIdentity()
		dc.Sealer = &client.Sealer{Me: "@console:" + name, ID: id, Device: true}
		if err := dc.RegisterKeys(c); err != nil {
			t.Fatal(err)
		}
		return dc
	}
	bobApp, carolApp := device("bob"), device("carol")

	// Agents, each owned by someone, with keys.
	agent := func(name, owner string, sealed bool) *client.Client {
		tok := h.register(api.RegisterRequest{Name: name, Binding: api.Binding{Harness: "claude", SessionID: "s-" + name}})
		if _, err := admin.SetAgentOwner(c, name, owner); err != nil {
			t.Fatal(err)
		}
		ac := h.client(tok)
		if sealed {
			id, _ := e2e.NewIdentity()
			ac.Sealer = &client.Sealer{Me: name, ID: id, Known: known}
			if err := ac.RegisterKeys(c); err != nil {
				t.Fatal(err)
			}
		}
		return ac
	}
	if _, err := admin.SetHubSettings(c, api.HubSettings{E2E: true}); err != nil {
		t.Fatal(err)
	}
	aliceBot := agent("alice-bot", "owner", true)
	bobBot := agent("bob-bot", "bob", true)
	carolBot := agent("carol-bot", "carol", true)
	oldBot := agent("old-bot", "bob", false)

	// alice's agent asks bob's agent.
	sent, err := aliceBot.Send(c, api.SendRequest{To: "bob-bot", Body: "the launch date is secret"}, false)
	if err != nil || sent.Message.Body != "the launch date is secret" || !sent.Message.E2E {
		t.Fatalf("send: %v %+v", err, sent)
	}
	// What the hub holds (read with its admin token, no keys).
	rawOf := func(id string) api.Message {
		t.Helper()
		ex, err := admin.Exchanges(c, "alice-bot", "bob-bot", 0, 200)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range ex.Messages {
			if m.ID == id {
				return m
			}
		}
		t.Fatalf("message %s not found", id)
		return api.Message{}
	}
	if raw := rawOf(sent.Message.ID); !e2e.IsSealed(raw.Body) || strings.Contains(raw.Body, "launch") || !raw.E2E {
		t.Fatalf("the hub must only hold ciphertext: %q", raw.Body)
	}
	got, err := bobBot.Pull(c, 1, 0)
	if err != nil || len(got) != 1 || got[0].Body != "the launch date is secret" {
		t.Fatalf("recipient: %v %+v", err, got)
	}
	// The recipient's suggested next message is sealed for the sender too.
	if _, err := bobBot.Suggest(c, sent.Message.ID, "ask marketing for the date", false); err != nil {
		t.Fatal(err)
	}
	if raw := rawOf(sent.Message.ID); !e2e.IsSealed(raw.Suggest) || strings.Contains(raw.Suggest, "marketing") {
		t.Fatalf("the hub must only hold a sealed suggestion: %q", raw.Suggest)
	}
	if m, err := aliceBot.GetMessage(c, sent.Message.ID); err != nil || m.Suggest != "ask marketing for the date" {
		t.Fatalf("the sender reads the suggestion: %v %+v", err, m)
	}
	// bob (the recipient's owner) reads it in the app; carol cannot.
	th, err := bobApp.Thread(c, sent.Message.ThreadID)
	if err != nil || len(th) != 1 || th[0].Body != "the launch date is secret" {
		t.Fatalf("owner's app: %v %+v", err, th)
	}
	// Another member's app does not even list it: the thread is not theirs.
	if th, err = carolApp.Thread(c, sent.Message.ThreadID); err == nil {
		t.Fatalf("another member must not get the thread: %+v", th)
	}
	// The answer comes back encrypted too.
	if _, err := bobBot.Reply(c, got[0].ID, "next friday", false); err != nil {
		t.Fatal(err)
	}
	reply, err := aliceBot.WaitReply(c, sent.Message.ID, time.Second)
	if err != nil || reply == nil || reply.Body != "next friday" {
		t.Fatalf("reply: %v %+v", err, reply)
	}

	// A failure reason is sealed for the sender as well.
	task, _ := aliceBot.Send(c, api.SendRequest{To: "bob-bot", Body: "do the impossible"}, false)
	bobBot.Pull(c, 1, 0)
	if _, err := bobBot.Fail(c, task.Message.ID, "cannot: no budget", false); err != nil {
		t.Fatal(err)
	}
	ev, err := aliceBot.WaitReply(c, task.Message.ID, time.Second)
	if err != nil || ev == nil || !strings.Contains(ev.Body, "no budget") {
		t.Fatalf("failure event: %v %+v", err, ev)
	}
	if raw := rawOf(ev.ID); strings.Contains(raw.Body, "budget") || !raw.E2E {
		t.Fatal("the failure reason must be sealed")
	}

	// Attachments are sealed on the hub and open for the recipient.
	dir := t.TempDir()
	src := filepath.Join(dir, "plan.txt")
	os.WriteFile(src, []byte("step 1: profit"), 0o600)
	f, err := aliceBot.UploadFile(c, src)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := aliceBot.Send(c, api.SendRequest{To: "bob-bot", Body: "see attached", Attachments: []string{f.ID}}, false); err != nil {
		t.Fatal(err)
	}
	in, _ := bobBot.Pull(c, 1, 0)
	if len(in) != 1 || len(in[0].Attachments) != 1 || in[0].Attachments[0].Name != "plan.txt" {
		t.Fatalf("attachment metadata: %+v", in)
	}
	_, path, err := bobBot.DownloadFile(c, f.ID, t.TempDir())
	if b, _ := os.ReadFile(path); err != nil || string(b) != "step 1: profit" || filepath.Base(path) != "plan.txt" {
		t.Fatalf("download: %v %s %q", err, path, b)
	}
	_, rawPath, _ := admin.DownloadFile(c, f.ID, t.TempDir())
	if b, _ := os.ReadFile(rawPath); strings.Contains(string(b), "profit") {
		t.Fatal("the hub must only hold the encrypted file")
	}

	// A broadcast is sealed for every recipient.
	for _, n := range []string{"bob-bot", "carol-bot"} {
		if _, err := admin.SetLabels(c, n, api.LabelsRequest{Add: []string{"team"}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := aliceBot.Broadcast(c, api.BroadcastRequest{Selector: "team", Type: "note", Body: "all hands at 3"}, false); err != nil {
		t.Fatal(err)
	}
	for _, bc := range []*client.Client{bobBot, carolBot} {
		ms, _ := bc.Pull(c, 5, 0)
		ok := false
		for _, m := range ms {
			ok = ok || m.Body == "all hands at 3"
		}
		if !ok {
			t.Fatalf("broadcast not readable: %+v", ms)
		}
	}

	// An agent too old to have keys gets plain messages.
	plain, err := aliceBot.Send(c, api.SendRequest{To: "old-bot", Body: "hello old friend"}, false)
	if err != nil || plain.Message.E2E {
		t.Fatalf("to an old agent: %v %+v", err, plain)
	}
	if ms, _ := oldBot.Pull(c, 1, 0); len(ms) != 1 || ms[0].Body != "hello old friend" {
		t.Fatalf("old agent: %+v", ms)
	}

	// Swapped keys are refused until trusted again.
	evil, _ := e2e.NewIdentity()
	bobBot.Sealer = &client.Sealer{Me: "bob-bot", ID: evil, Known: known}
	if err := bobBot.RegisterKeys(c); err != nil {
		t.Fatal(err)
	}
	if _, err := aliceBot.Send(c, api.SendRequest{To: "bob-bot", Body: "still there?"}, false); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("changed keys must stop the send: %v", err)
	}
	known.Trust("bob-bot")
	if _, err := aliceBot.Send(c, api.SendRequest{To: "bob-bot", Body: "still there?"}, false); err != nil {
		t.Fatalf("after trusting: %v", err)
	}
}
