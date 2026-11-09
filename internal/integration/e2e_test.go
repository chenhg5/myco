package integration_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/e2e"
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
	laohuApp, matthewApp := device("laohu"), device("matthew")

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
	glennBot := agent("glenn-bot", "owner", true)
	laohuBot := agent("laohu-bot", "laohu", true)
	matthewBot := agent("matthew-bot", "matthew", true)
	oldBot := agent("old-bot", "laohu", false)

	// glenn's agent asks laohu's agent.
	sent, err := glennBot.Send(c, api.SendRequest{To: "laohu-bot", Body: "the launch date is secret"}, false)
	if err != nil || sent.Message.Body != "the launch date is secret" || !sent.Message.E2E {
		t.Fatalf("send: %v %+v", err, sent)
	}
	// What the hub holds (read with its admin token, no keys).
	rawOf := func(id string) api.Message {
		t.Helper()
		ex, err := admin.Exchanges(c, "glenn-bot", "laohu-bot", 0, 200)
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
	got, err := laohuBot.Pull(c, 1, 0)
	if err != nil || len(got) != 1 || got[0].Body != "the launch date is secret" {
		t.Fatalf("recipient: %v %+v", err, got)
	}
	// laohu (the recipient's owner) reads it in the app; matthew cannot.
	th, err := laohuApp.Thread(c, sent.Message.ThreadID)
	if err != nil || len(th) != 1 || th[0].Body != "the launch date is secret" {
		t.Fatalf("owner's app: %v %+v", err, th)
	}
	th, _ = matthewApp.Thread(c, sent.Message.ThreadID)
	if len(th) != 1 || th[0].Unreadable == "" || strings.Contains(th[0].Body, "launch") {
		t.Fatalf("another member must not read it: %+v", th)
	}
	// The answer comes back encrypted too.
	if _, err := laohuBot.Reply(c, got[0].ID, "next friday", false); err != nil {
		t.Fatal(err)
	}
	reply, err := glennBot.WaitReply(c, sent.Message.ID, time.Second)
	if err != nil || reply == nil || reply.Body != "next friday" {
		t.Fatalf("reply: %v %+v", err, reply)
	}

	// A failure reason is sealed for the sender as well.
	task, _ := glennBot.Send(c, api.SendRequest{To: "laohu-bot", Body: "do the impossible"}, false)
	laohuBot.Pull(c, 1, 0)
	if _, err := laohuBot.Fail(c, task.Message.ID, "cannot: no budget", false); err != nil {
		t.Fatal(err)
	}
	ev, err := glennBot.WaitReply(c, task.Message.ID, time.Second)
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
	f, err := glennBot.UploadFile(c, src)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := glennBot.Send(c, api.SendRequest{To: "laohu-bot", Body: "see attached", Attachments: []string{f.ID}}, false); err != nil {
		t.Fatal(err)
	}
	in, _ := laohuBot.Pull(c, 1, 0)
	if len(in) != 1 || len(in[0].Attachments) != 1 || in[0].Attachments[0].Name != "plan.txt" {
		t.Fatalf("attachment metadata: %+v", in)
	}
	_, path, err := laohuBot.DownloadFile(c, f.ID, t.TempDir())
	if b, _ := os.ReadFile(path); err != nil || string(b) != "step 1: profit" || filepath.Base(path) != "plan.txt" {
		t.Fatalf("download: %v %s %q", err, path, b)
	}
	_, rawPath, _ := admin.DownloadFile(c, f.ID, t.TempDir())
	if b, _ := os.ReadFile(rawPath); strings.Contains(string(b), "profit") {
		t.Fatal("the hub must only hold the encrypted file")
	}

	// A broadcast is sealed for every recipient.
	for _, n := range []string{"laohu-bot", "matthew-bot"} {
		if _, err := admin.SetLabels(c, n, api.LabelsRequest{Add: []string{"team"}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := glennBot.Broadcast(c, api.BroadcastRequest{Selector: "team", Type: "note", Body: "all hands at 3"}, false); err != nil {
		t.Fatal(err)
	}
	for _, bc := range []*client.Client{laohuBot, matthewBot} {
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
	plain, err := glennBot.Send(c, api.SendRequest{To: "old-bot", Body: "hello old friend"}, false)
	if err != nil || plain.Message.E2E {
		t.Fatalf("to an old agent: %v %+v", err, plain)
	}
	if ms, _ := oldBot.Pull(c, 1, 0); len(ms) != 1 || ms[0].Body != "hello old friend" {
		t.Fatalf("old agent: %+v", ms)
	}

	// Swapped keys are refused until trusted again.
	evil, _ := e2e.NewIdentity()
	laohuBot.Sealer = &client.Sealer{Me: "laohu-bot", ID: evil, Known: known}
	if err := laohuBot.RegisterKeys(c); err != nil {
		t.Fatal(err)
	}
	if _, err := glennBot.Send(c, api.SendRequest{To: "laohu-bot", Body: "still there?"}, false); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("changed keys must stop the send: %v", err)
	}
	known.Trust("laohu-bot")
	if _, err := glennBot.Send(c, api.SendRequest{To: "laohu-bot", Body: "still there?"}, false); err != nil {
		t.Fatalf("after trusting: %v", err)
	}
}
