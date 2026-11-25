package integration_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/client"
)

// Several people on one hub: each manages what they own, sees and talks to
// everything, and administrative actions land in the audit log.
func TestMembersOwnershipPermissionsAndAudit(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	admin := h.admin()

	// The hub starts with an owner, who renames themselves.
	ms, err := admin.Members(c, false)
	if err != nil || len(ms) != 1 || ms[0].Role != "owner" || ms[0].Name != "owner" {
		t.Fatalf("members: %v %+v", err, ms)
	}
	if _, err := admin.UpdateMember(c, "owner", api.MemberUpdateRequest{Name: "alice"}); err != nil {
		t.Fatal(err)
	}
	h.register(api.RegisterRequest{Name: "alice-bot"})

	// Inviting bob gives him an app link of his own.
	inv, err := admin.InviteMember(c, api.InviteRequest{Name: "bob", DisplayName: "Bob"})
	if err != nil || inv.Member.Role != "member" || !strings.Contains(inv.Link, "/c/myco_pair_") {
		t.Fatalf("invite: %v %+v", err, inv)
	}
	// The link is a one-time code: his app trades it for its own token.
	code := inv.Link[strings.Index(inv.Link, "/c/")+3:]
	paired, err := h.client("").Pair(c, code, "bob-mac")
	if err != nil || paired.Member == nil || paired.Member.Name != "bob" || !strings.HasPrefix(paired.Token, "myco_console_") {
		t.Fatalf("pair: %v %+v", err, paired)
	}
	if _, err := h.client("").Pair(c, code, "someone-else"); err == nil {
		t.Fatal("a pairing link must work only once")
	}
	if _, err := h.client(code).Whoami(c); err == nil {
		t.Fatal("the pairing code itself must not be a credential")
	}
	bob := h.client(paired.Token)
	who, err := bob.Whoami(c)
	if err != nil || who.Member == nil || who.Member.Name != "bob" || who.CanManage || who.Device == nil || who.Device.Name != "bob-mac" {
		t.Fatalf("whoami: %v %+v", err, who)
	}

	// He adds his Windows PC with a link of his own, and later removes it.
	pl, err := bob.CreatePairing(c)
	if err != nil {
		t.Fatal(err)
	}
	win, err := h.client("").Pair(c, pl.Link[strings.Index(pl.Link, "/c/")+3:], "bob-pc")
	if err != nil || win.Member.Name != "bob" {
		t.Fatalf("second device: %v %+v", err, win)
	}
	devs, err := bob.Devices(c)
	if err != nil || len(devs) != 2 {
		t.Fatalf("devices: %v %+v", err, devs)
	}
	if err := bob.RemoveDevice(c, win.Device.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.client(win.Token).Whoami(c); err == nil {
		t.Fatal("a removed device must lose access")
	}
	if err := bob.RemoveDevice(c, mustFirstDevice(t, admin)); err == nil {
		t.Fatal("removing someone else's device must fail")
	}

	// What bob adds through his own link is his.
	enr, err := bob.CreateEnrollment(c, api.CreateEnrollmentRequest{NamePrefix: "bob-", MaxUses: 1}, false)
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.client(enr.Token).Enroll(c, api.EnrollRequest{Name: "bob-claude", Binding: api.Binding{Harness: "claude", SessionID: "s-bob"}}, false)
	if err != nil || res.Agent.Owner != "bob" {
		t.Fatalf("enroll: %v %+v", err, res)
	}
	mine := h.client(res.Token)

	// He manages only his own things.
	denied := func(what string, err error) {
		t.Helper()
		var ce *client.Error
		if err == nil || !errors.As(err, &ce) || ce.Code != "permission_denied" {
			t.Fatalf("%s: want permission_denied, got %v", what, err)
		}
	}
	_, err = bob.RemoveAgent(c, "alice-bot", false)
	denied("remove someone else's node", err)
	_, err = bob.SetLabels(c, "alice-bot", api.LabelsRequest{Add: []string{"x"}})
	denied("label someone else's node", err)
	_, err = bob.InviteMember(c, api.InviteRequest{Name: "carol"})
	denied("invite", err)
	_, err = bob.CreateEnrollment(c, api.CreateEnrollmentRequest{NamePrefix: "x-", Harness: "exec", AllowFrom: []string{"bob-claude"}}, false)
	denied("exec link", err)
	_, err = bob.Audit(c, client.AuditFilter{})
	denied("audit", err)
	_, err = bob.SetAgentOwner(c, "alice-bot", "bob")
	denied("take a node", err)
	if _, err := bob.SetLabels(c, "bob-claude", api.LabelsRequest{Add: []string{"group=web"}}); err != nil {
		t.Fatalf("label own node: %v", err)
	}

	// He talks to anyone; messages carry the person behind the sender.
	sent, err := bob.ConsoleSend(c, api.SendRequest{To: "alice-bot", Body: "能帮我看下发布清单吗"})
	if err != nil || sent.Message.From != "@console:bob" || sent.Message.FromOwner != "bob" {
		t.Fatalf("console send: %v %+v", err, sent)
	}
	msg, err := mine.Send(c, api.SendRequest{To: "alice-bot", Body: "发布日期能改到周五吗"}, false)
	if err != nil || msg.Message.FromOwner != "bob" {
		t.Fatalf("agent send: %v %+v", err, msg)
	}

	// He may clear what waits for his own node, not for someone else's.
	_, err = bob.ConsoleDismiss(c, msg.Message.ID)
	denied("dismiss a message of someone else's node", err)
	// (His node is private to him by default; the hub's owner sees it but
	// cannot write to it until he shares it.)
	if _, err := admin.ConsoleSend(c, api.SendRequest{To: "bob-claude", Type: "note", Body: "x"}); err == nil {
		t.Fatal("the hub's owner wrote to a member's private node")
	}
	if _, err := bob.SetVisibility(c, "bob-claude", "team"); err != nil {
		t.Fatal(err)
	}
	toHim, err := admin.ConsoleSend(c, api.SendRequest{To: "bob-claude", Type: "note", Body: "旧消息"})
	if err != nil {
		t.Fatal(err)
	}
	if m, err := bob.ConsoleDismiss(c, toHim.Message.ID); err != nil || m.Status != "done" {
		t.Fatalf("dismiss own node's message: %v %+v", err, m)
	}

	if _, err := bob.KickAgent(c, "bob-claude", false); err != nil {
		t.Fatalf("kick own node: %v", err)
	}

	// The owner sees all of it in the audit log.
	log, err := admin.Audit(c, client.AuditFilter{})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range log {
		got = append(got, e.Actor+" "+e.Action+" "+e.Target)
	}
	all := strings.Join(got, "\n")
	for _, want := range []string{"admin member.invite bob", "bob device.add bob-mac", "bob device.remove bob-pc", "bob link.create", "bob agent.edit bob-claude", "bob agent.kick bob-claude", "bob message.dismiss bob-claude", "bob message.task alice-bot", "admin agent.register alice-bot"} {
		if !strings.Contains(all, want) {
			t.Errorf("audit lacks %q:\n%s", want, all)
		}
	}
	if strings.Contains(all, "能帮我看下") {
		t.Error("audit must not contain message bodies")
	}

	// Ownership moves; removing bob ends his app access.
	if a, err := admin.SetAgentOwner(c, "bob-claude", "alice"); err != nil || a.Owner != "alice" {
		t.Fatalf("set owner: %v %+v", err, a)
	}
	if _, err := admin.RemoveMember(c, "bob"); err != nil {
		t.Fatal(err)
	}
	if _, err := bob.Whoami(c); err == nil {
		t.Fatal("a removed member's app link must stop working")
	}
	if _, err := admin.RemoveMember(c, "alice"); err == nil {
		t.Fatal("the owner cannot be removed")
	}
}

// mustFirstDevice returns the owner's first device id (creating one).
func mustFirstDevice(t *testing.T, admin *client.Client) string {
	t.Helper()
	ct, err := admin.CreateConsoleToken(context.Background(), "alice-mac")
	if err != nil {
		t.Fatal(err)
	}
	return ct.ConsoleToken.ID
}
