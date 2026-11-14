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

// What an agent may do on someone's computer is that person's call: its
// owner chooses its access in the app; the hub's owner and the agent itself
// cannot, and other changes leave it alone.
func TestOnlyTheOwnerChoosesAnAgentsAccess(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	admin := h.admin()
	inv, err := admin.InviteMember(c, api.InviteRequest{Name: "laohu"})
	if err != nil {
		t.Fatal(err)
	}
	paired, err := h.client("").Pair(c, inv.Link[strings.Index(inv.Link, "/c/")+3:], "laohu-mac")
	if err != nil {
		t.Fatal(err)
	}
	laohu := h.client(paired.Token)
	enr, err := laohu.CreateEnrollment(c, api.CreateEnrollmentRequest{NamePrefix: "laohu-", MaxUses: 1}, false)
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.client(enr.Token).Enroll(c, api.EnrollRequest{Name: "laohu-codex", Binding: api.Binding{Harness: "codex", SessionID: "s"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	agent := h.client(res.Token)
	denied := func(what string, err error) {
		t.Helper()
		var ce *client.Error
		if err == nil || !errors.As(err, &ce) || ce.Code != "permission_denied" {
			t.Fatalf("%s: want permission_denied, got %v", what, err)
		}
	}
	access := func() string {
		t.Helper()
		me, err := agent.Me(c)
		if err != nil {
			t.Fatal(err)
		}
		if me.RunSettings == nil {
			return ""
		}
		return me.RunSettings.Access
	}

	if _, err := laohu.SetRunSettings(c, "laohu-codex", api.RunSettings{Access: "full"}); err != nil {
		t.Fatalf("owner sets access: %v", err)
	}
	if got := access(); got != "full" {
		t.Fatalf("access = %q", got)
	}
	_, err = admin.SetRunSettings(c, "laohu-codex", api.RunSettings{Access: "readonly"})
	denied("the hub's owner changes someone's access", err)
	_, err = agent.SetRunSettings(c, "", api.RunSettings{Access: "readonly"})
	denied("the agent changes its own access", err)
	// Choosing the model (the hub's owner, or the agent) keeps it.
	if _, err := admin.SetRunSettings(c, "laohu-codex", api.RunSettings{Model: "m1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := agent.SetRunSettings(c, "", api.RunSettings{Model: "m2", Effort: "high"}); err != nil {
		t.Fatal(err)
	}
	if got := access(); got != "full" {
		t.Fatalf("access after model changes = %q", got)
	}
	if _, err := laohu.SetRunSettings(c, "laohu-codex", api.RunSettings{Access: "root"}); err == nil {
		t.Fatal("an unknown access level must be refused")
	}
}
