package integration_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/client"
)

// A plain member's nodes are private to them by default: other people and
// other people's agents neither find them nor give them work; the member
// and their own agents do; the hub's owner sees them (to manage them) but
// cannot send. Replies always get through. Only people change visibility.
func TestPrivateNodes(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	glenn := h.admin()
	if _, err := glenn.UpdateMember(c, "owner", api.MemberUpdateRequest{Name: "glenn"}); err != nil {
		t.Fatal(err)
	}
	member := func(name string) *client.Client {
		inv, err := glenn.InviteMember(c, api.InviteRequest{Name: name})
		if err != nil {
			t.Fatal(err)
		}
		p, err := h.client("").Pair(c, inv.Link[strings.Index(inv.Link, "/c/")+3:], name+"-mac")
		if err != nil {
			t.Fatal(err)
		}
		return h.client(p.Token)
	}
	laohu, matthew := member("laohu"), member("matthew")
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
	glennBot, gb := enroll(glenn, "glenn-bot")
	laohuBot, lb := enroll(laohu, "laohu-claude")
	laohuHelper, _ := enroll(laohu, "laohu-helper")
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
	// Other people's agents do not find it, nor does matthew's app.
	if strings.Contains(names(glennBot), "laohu-claude") {
		t.Fatal("glenn's agent lists laohu's private node")
	}
	if _, err := glennBot.GetAgent(c, "laohu-claude"); err == nil {
		t.Fatal("glenn's agent looked up laohu's private node")
	}
	ov, err := matthew.Overview(c, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range ov.Agents {
		if strings.HasPrefix(a.Agent.Name, "laohu-") {
			t.Fatalf("matthew's app sees %s", a.Agent.Name)
		}
	}
	// laohu's own agent and app do; the hub's owner sees it, marked private.
	if !strings.Contains(names(laohuHelper), "laohu-claude") {
		t.Fatal("laohu's own agent does not find his node")
	}
	if a, err := glenn.GetAgent(c, "laohu-claude"); err != nil || a.Visibility != "private" {
		t.Fatalf("owner's view: %v %+v", err, a)
	}

	// Sending: others are refused; laohu and his agents are not.
	if _, err := glennBot.Send(c, api.SendRequest{To: "laohu-claude", Body: "hi"}, false); err == nil {
		t.Fatal("glenn's agent gave laohu's private node a task")
	}
	if _, err := glenn.ConsoleSend(c, api.SendRequest{To: "laohu-claude", Body: "hi"}); err == nil {
		t.Fatal("the hub's owner gave a member's private node a task")
	}
	if _, err := laohu.ConsoleSend(c, api.SendRequest{To: "laohu-claude", Body: "from laohu"}); err != nil {
		t.Fatalf("laohu from his app: %v", err)
	}
	if _, err := laohuHelper.Send(c, api.SendRequest{To: "laohu-claude", Body: "from helper"}, false); err != nil {
		t.Fatalf("laohu's own agent: %v", err)
	}
	// A broadcast to everyone leaves it out.
	if bc, err := glennBot.Broadcast(c, api.BroadcastRequest{Selector: "harness=claude", Body: "status?", IncludeSelf: true}, false); err != nil {
		t.Fatal(err)
	} else {
		for _, m := range bc.Messages {
			if strings.HasPrefix(m.To, "laohu-") {
				t.Fatalf("broadcast reached %s", m.To)
			}
		}
		for _, sk := range bc.Skipped {
			if strings.HasPrefix(sk.Agent, "laohu-") {
				t.Fatalf("broadcast names the private node as skipped: %+v", sk)
			}
		}
	}
	// It can still ask others, and their replies get back.
	q, err := laohuBot.Send(c, api.SendRequest{To: "glenn-bot", Body: "which API?"}, false)
	if err != nil {
		t.Fatal(err)
	}
	glennBot.Pull(c, 10, 0)
	if _, err := glennBot.Reply(c, q.Message.ID, "the orders API", false); err != nil {
		t.Fatalf("reply to a private node: %v", err)
	}

	// Only people change visibility: not the agent, not another member.
	if _, err := laohuBot.SetVisibility(c, "laohu-claude", "team"); err == nil {
		t.Fatal("an agent changed its own visibility")
	}
	if _, err := matthew.SetVisibility(c, "laohu-claude", "team"); err == nil {
		t.Fatal("another member changed laohu's node")
	}
	if a, err := laohu.SetVisibility(c, "laohu-claude", "team"); err != nil || a.Visibility != "team" {
		t.Fatalf("laohu shares his node: %v %+v", err, a)
	}
	if !strings.Contains(names(glennBot), "laohu-claude") {
		t.Fatal("shared node still hidden")
	}
	if _, err := glennBot.Send(c, api.SendRequest{To: "laohu-claude", Body: "now ok"}, false); err != nil {
		t.Fatalf("after sharing: %v", err)
	}
	if _, err := laohu.SetVisibility(c, "laohu-claude", "secret"); err == nil {
		t.Fatal("accepted an unknown visibility")
	}
}
