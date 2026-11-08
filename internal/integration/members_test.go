package integration_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/client"
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
	if _, err := admin.UpdateMember(c, "owner", api.MemberUpdateRequest{Name: "glenn"}); err != nil {
		t.Fatal(err)
	}
	h.register(api.RegisterRequest{Name: "glenn-bot"})

	// Inviting laohu gives him an app link of his own.
	inv, err := admin.InviteMember(c, api.InviteRequest{Name: "laohu", DisplayName: "老胡"})
	if err != nil || inv.Member.Role != "member" || !strings.Contains(inv.Link, "/c/myco_console_") {
		t.Fatalf("invite: %v %+v", err, inv)
	}
	laohu := h.client(inv.Link[strings.Index(inv.Link, "/c/")+3:])
	who, err := laohu.Whoami(c)
	if err != nil || who.Member == nil || who.Member.Name != "laohu" || who.CanManage {
		t.Fatalf("whoami: %v %+v", err, who)
	}

	// What laohu adds through his own link is his.
	enr, err := laohu.CreateEnrollment(c, api.CreateEnrollmentRequest{NamePrefix: "laohu-", MaxUses: 1}, false)
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.client(enr.Token).Enroll(c, api.EnrollRequest{Name: "laohu-claude", Binding: api.Binding{Harness: "claude", SessionID: "s-laohu"}}, false)
	if err != nil || res.Agent.Owner != "laohu" {
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
	_, err = laohu.RemoveAgent(c, "glenn-bot", false)
	denied("remove someone else's node", err)
	_, err = laohu.SetLabels(c, "glenn-bot", api.LabelsRequest{Add: []string{"x"}})
	denied("label someone else's node", err)
	_, err = laohu.InviteMember(c, api.InviteRequest{Name: "matthew"})
	denied("invite", err)
	_, err = laohu.CreateEnrollment(c, api.CreateEnrollmentRequest{NamePrefix: "x-", Harness: "exec", AllowFrom: []string{"laohu-claude"}}, false)
	denied("exec link", err)
	_, err = laohu.Audit(c, client.AuditFilter{})
	denied("audit", err)
	_, err = laohu.SetAgentOwner(c, "glenn-bot", "laohu")
	denied("take a node", err)
	if _, err := laohu.SetLabels(c, "laohu-claude", api.LabelsRequest{Add: []string{"group=web"}}); err != nil {
		t.Fatalf("label own node: %v", err)
	}

	// He talks to anyone; messages carry the person behind the sender.
	sent, err := laohu.ConsoleSend(c, api.SendRequest{To: "glenn-bot", Body: "能帮我看下发布清单吗"})
	if err != nil || sent.Message.From != "@console:laohu" || sent.Message.FromOwner != "laohu" {
		t.Fatalf("console send: %v %+v", err, sent)
	}
	msg, err := mine.Send(c, api.SendRequest{To: "glenn-bot", Body: "发布日期能改到周五吗"}, false)
	if err != nil || msg.Message.FromOwner != "laohu" {
		t.Fatalf("agent send: %v %+v", err, msg)
	}

	if _, err := laohu.KickAgent(c, "laohu-claude", false); err != nil {
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
	for _, want := range []string{"admin member.invite laohu", "laohu link.create", "laohu agent.labels laohu-claude", "laohu agent.kick laohu-claude", "laohu message.task glenn-bot", "admin agent.register glenn-bot"} {
		if !strings.Contains(all, want) {
			t.Errorf("audit lacks %q:\n%s", want, all)
		}
	}
	if strings.Contains(all, "能帮我看下") {
		t.Error("audit must not contain message bodies")
	}

	// Ownership moves; removing laohu ends his app access.
	if a, err := admin.SetAgentOwner(c, "laohu-claude", "glenn"); err != nil || a.Owner != "glenn" {
		t.Fatalf("set owner: %v %+v", err, a)
	}
	if _, err := admin.RemoveMember(c, "laohu"); err != nil {
		t.Fatal(err)
	}
	if _, err := laohu.Whoami(c); err == nil {
		t.Fatal("a removed member's app link must stop working")
	}
	if _, err := admin.RemoveMember(c, "glenn"); err == nil {
		t.Fatal("the owner cannot be removed")
	}
}
