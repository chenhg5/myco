package integration_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/client"
)

// A plain member's nodes are private to them by default: other people and
// other people's agents neither find them nor give them work; the member
// and their own agents do; the hub's owner sees them (to manage them) but
// cannot send. Replies always get through. Only people change visibility.
func TestPrivateNodes(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	alice := h.admin()
	if _, err := alice.UpdateMember(c, "owner", api.MemberUpdateRequest{Name: "alice"}); err != nil {
		t.Fatal(err)
	}
	member := func(name string) *client.Client {
		inv, err := alice.InviteMember(c, api.InviteRequest{Name: name})
		if err != nil {
			t.Fatal(err)
		}
		p, err := h.client("").Pair(c, inv.Link[strings.Index(inv.Link, "/c/")+3:], name+"-mac")
		if err != nil {
			t.Fatal(err)
		}
		return h.client(p.Token)
	}
	bob, carol := member("bob"), member("carol")
	enroll := func(owner *client.Client, name string) (*client.Client, *api.Agent) {
		enr, err := owner.CreateEnrollment(c, api.CreateEnrollmentRequest{NamePrefix: name, MaxUses: 1}, false)
		if err != nil {
			t.Fatal(err)
		}
		res, err := h.client(enr.Token).Enroll(c, api.EnrollRequest{Name: name, Binding: api.Binding{Harness: "claude", SessionID: "s-" + name}}, false)
		if err != nil {
			t.Fatal(err)
		}
		return h.client(res.Token), &res.Agent
	}
	aliceBot, gb := enroll(alice, "alice-bot")
	bobBot, lb := enroll(bob, "bob-claude")
	bobHelper, _ := enroll(bob, "bob-helper")
	if gb.Visibility != "team" || lb.Visibility != "private" {
		t.Fatalf("defaults: owner's %q, member's %q", gb.Visibility, lb.Visibility)
	}

	names := func(cl *client.Client) string {
		t.Helper()
		l, err := cl.ListAgents(c, "", false)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, a := range l {
			out = append(out, a.Name)
		}
		return strings.Join(out, ",")
	}
	// Other people's agents do not find it, nor does carol's app.
	if strings.Contains(names(aliceBot), "bob-claude") {
		t.Fatal("alice's agent lists bob's private node")
	}
	if _, err := aliceBot.GetAgent(c, "bob-claude"); err == nil {
		t.Fatal("alice's agent looked up bob's private node")
	}
	ov, err := carol.Overview(c, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range ov.Agents {
		if strings.HasPrefix(a.Agent.Name, "bob-") {
			t.Fatalf("carol's app sees %s", a.Agent.Name)
		}
	}
	// bob's own agent and app do; the hub's owner sees it, marked private.
	if !strings.Contains(names(bobHelper), "bob-claude") {
		t.Fatal("bob's own agent does not find his node")
	}
	if a, err := alice.GetAgent(c, "bob-claude"); err != nil || a.Visibility != "private" {
		t.Fatalf("owner's view: %v %+v", err, a)
	}

	// Sending: others are refused; bob and his agents are not.
	if _, err := aliceBot.Send(c, api.SendRequest{To: "bob-claude", Body: "hi"}, false); err == nil {
		t.Fatal("alice's agent gave bob's private node a task")
	}
	if _, err := alice.ConsoleSend(c, api.SendRequest{To: "bob-claude", Body: "hi"}); err == nil {
		t.Fatal("the hub's owner gave a member's private node a task")
	}
	if _, err := bob.ConsoleSend(c, api.SendRequest{To: "bob-claude", Body: "from bob"}); err != nil {
		t.Fatalf("bob from his app: %v", err)
	}
	if _, err := bobHelper.Send(c, api.SendRequest{To: "bob-claude", Body: "from helper"}, false); err != nil {
		t.Fatalf("bob's own agent: %v", err)
	}
	// A broadcast to everyone leaves it out.
	if bc, err := aliceBot.Broadcast(c, api.BroadcastRequest{Selector: "harness=claude", Body: "status?", IncludeSelf: true}, false); err != nil {
		t.Fatal(err)
	} else {
		for _, m := range bc.Messages {
			if strings.HasPrefix(m.To, "bob-") {
				t.Fatalf("broadcast reached %s", m.To)
			}
		}
		for _, sk := range bc.Skipped {
			if strings.HasPrefix(sk.Agent, "bob-") {
				t.Fatalf("broadcast names the private node as skipped: %+v", sk)
			}
		}
	}
	// It can still ask others, and their replies get back.
	q, err := bobBot.Send(c, api.SendRequest{To: "alice-bot", Body: "which API?"}, false)
	if err != nil {
		t.Fatal(err)
	}
	aliceBot.Pull(c, 10, 0)
	if _, err := aliceBot.Reply(c, q.Message.ID, "the orders API", false); err != nil {
		t.Fatalf("reply to a private node: %v", err)
	}

	// Only people change visibility: not the agent, not another member.
	if _, err := bobBot.SetVisibility(c, "bob-claude", "team"); err == nil {
		t.Fatal("an agent changed its own visibility")
	}
	if _, err := carol.SetVisibility(c, "bob-claude", "team"); err == nil {
		t.Fatal("another member changed bob's node")
	}
	if a, err := bob.SetVisibility(c, "bob-claude", "team"); err != nil || a.Visibility != "team" {
		t.Fatalf("bob shares his node: %v %+v", err, a)
	}
	if !strings.Contains(names(aliceBot), "bob-claude") {
		t.Fatal("shared node still hidden")
	}
	if _, err := aliceBot.Send(c, api.SendRequest{To: "bob-claude", Body: "now ok"}, false); err != nil {
		t.Fatalf("after sharing: %v", err)
	}
	if _, err := bob.SetVisibility(c, "bob-claude", "secret"); err == nil {
		t.Fatal("accepted an unknown visibility")
	}
}
