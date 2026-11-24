package integration_test

import (
	"path/filepath"
	"testing"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/client"
)

// Which computers a project's coordinator may start agents on is a setting
// of the project: it sees them, asks the owner for one (a card they
// approve), and may then ask that computer for sessions; the owner can
// take it back.
func TestProjectMachinesRequestAndSpawn(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	admin := h.admin()
	if _, err := admin.UpdateMember(c, "owner", api.MemberUpdateRequest{Name: "alice"}); err != nil {
		t.Fatal(err)
	}
	ct, err := admin.CreateConsoleToken(c, "alice-mac")
	if err != nil {
		t.Fatal(err)
	}
	alice := h.client(ct.Token)
	node := h.client(h.register(api.RegisterRequest{Name: "wsl-machine", AllowFrom: []string{"someone-else"}, Binding: api.Binding{Harness: "machine"}}))
	if err := node.ReportMachine(c, api.MachineReport{Fingerprint: "fp-wsl-0123456789abcdef", Name: "wsl", Hostname: "laptop", OS: "linux"}); err != nil {
		t.Fatal(err)
	}
	coord := h.client(h.register(api.RegisterRequest{Name: "coord", Binding: api.Binding{Harness: "claude", SessionID: "s1"}}))
	helper := h.client(h.register(api.RegisterRequest{Name: "helper", Binding: api.Binding{Harness: "codex", SessionID: "s2"}}))
	if _, err := alice.CreateProject(c, api.CreateProjectRequest{Name: "cc", Title: "sample-app"}); err != nil {
		t.Fatal(err)
	}
	coordRole := "coordinator"
	if _, err := alice.PutProjectMember(c, "cc", "coord", api.ProjectMemberRequest{Role: &coordRole}); err != nil {
		t.Fatal(err)
	}
	if _, err := alice.PutProjectMember(c, "cc", "helper", api.ProjectMemberRequest{}); err != nil {
		t.Fatal(err)
	}
	spawn := func() error {
		_, err := coord.Send(c, api.SendRequest{To: "wsl-machine", Type: "task", Body: `{"cli":"codex","dir":"~/code/app"}`}, false)
		return err
	}

	// It sees the computers, none allowed yet; asking it to start one is refused.
	ms, err := coord.ProjectMachines(c, "cc")
	if err != nil || len(ms.Machines) != 1 || ms.Machines[0].Name != "wsl" || ms.Machines[0].Allowed {
		t.Fatalf("machines: %+v %v", ms, err)
	}
	if err := spawn(); err == nil {
		t.Fatal("started an agent on a computer the project may not use")
	}
	// Only a coordinator asks.
	if _, _, err := helper.RequestProjectMachine(c, "cc", "wsl", "x"); err == nil {
		t.Fatal("a member that does not coordinate asked for a computer")
	}
	ap, allowed, err := coord.RequestProjectMachine(c, "cc", "wsl", "要在 WSL 上开一个 Codex 跑迁移")
	if err != nil || allowed || ap.Status != "pending" || ap.Reason == "" {
		t.Fatalf("request: %+v %v %v", ap, allowed, err)
	}
	if again, _, err := coord.RequestProjectMachine(c, "cc", "wsl", ""); err != nil || again.ID != ap.ID {
		t.Fatalf("asking again should give the same card: %+v %v", again, err)
	}
	// alice approves: the project may now use it, and the coordinator starts one.
	list, err := alice.Approvals(c, "pending")
	if err != nil || len(list) != 1 || list[0].ID != ap.ID || list[0].Agent != "coord" {
		t.Fatalf("approvals: %+v %v", list, err)
	}
	done, err := alice.DecideApproval(c, ap.ID, true)
	if err != nil || done.Status != "done" || done.Result != 200 {
		t.Fatalf("approve: %+v %v", done, err)
	}
	if ms, _ := coord.ProjectMachines(c, "cc"); len(ms.Machines) != 1 || !ms.Machines[0].Allowed {
		t.Fatalf("after approval: %+v", ms)
	}
	sent, err := coord.Send(c, api.SendRequest{To: "wsl-machine", Type: "task", Body: `{"cli":"codex","dir":"~/code/app"}`}, false)
	if err != nil || sent.Message.GrantedBy != "project:cc" {
		t.Fatalf("spawn on an allowed computer: %+v %v", sent, err)
	}
	if _, allowed, err := coord.RequestProjectMachine(c, "cc", "wsl", ""); err != nil || !allowed {
		t.Fatalf("asking for an allowed one: %v %v", allowed, err)
	}
	// Agents do not set it themselves; alice takes it back.
	if _, err := coord.SetProjectMachine(c, "cc", "wsl", true); err == nil {
		t.Fatal("a coordinator allowed a computer itself")
	}
	if p, err := alice.SetProjectMachine(c, "cc", "wsl", false); err != nil || len(p.Machines) != 0 {
		t.Fatalf("disallow: %+v %v", p, err)
	}
	if err := spawn(); err == nil {
		t.Fatal("started an agent after the computer was taken back")
	} else if e, ok := client.AsError(err); !ok || e.Code != "permission_denied" {
		t.Fatalf("refusal: %v", err)
	}
}
