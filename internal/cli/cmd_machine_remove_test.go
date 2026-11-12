package cli

import (
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/config"
	"github.com/chenhg5/myco/internal/testutil"
)

func TestMachineRemoveRevokesEnrollmentAndCleansNodes(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	hub := newHub(t)
	admin := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(home, "admin.yaml")}}
	code, out, e := admin.run("link", "create", "--format", "json")
	if code != 0 {
		t.Fatalf("link: %s", e)
	}
	var link struct {
		JoinURL string `json:"join_url"`
	}
	if err := json.Unmarshal([]byte(out), &link); err != nil {
		t.Fatal(err)
	}
	machine := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_JOIN_TOKEN": strings.TrimPrefix(link.JoinURL, hub+"/j/")}}
	if code, _, e := machine.run("machine", "join", "--machine", "test-box", "--no-service"); code != 0 {
		t.Fatalf("join machine: %s", e)
	}
	session := &harnessT{t, map[string]string{}}
	code, out, e = session.run("join", "--harness", "inbox", "--format", "json")
	if code != 0 {
		t.Fatalf("join agent: %s", e)
	}
	var joined struct{ Agent struct{ Name string } }
	if err := json.Unmarshal([]byte(out), &joined); err != nil {
		t.Fatal(err)
	}
	profile, err := config.Load(filepath.Join(home, ".myco", "agents", joined.Agent.Name+".yaml"))
	if err != nil {
		t.Fatal(err)
	}
	agentClient, _ := client.New(hub, profile.Token)
	ctx := context.Background()
	if err := agentClient.ReportMachine(ctx, api.MachineReport{Fingerprint: strings.Repeat("f", 32), Name: "test-box", Hostname: "test-box"}); err != nil {
		t.Fatal(err)
	}
	adminClient, _ := client.New(hub, "admin-token-123456")
	machines, err := adminClient.Machines(ctx)
	if err != nil || len(machines) != 1 {
		t.Fatalf("machines: %v %v", machines, err)
	}
	id := machines[0].ID
	if _, err := adminClient.SetAlias(ctx, "test-helper", joined.Agent.Name, false); err != nil {
		t.Fatal(err)
	}
	if _, err := adminClient.CreateProject(ctx, api.CreateProjectRequest{Name: "test-work", Title: "Test Work"}); err != nil {
		t.Fatal(err)
	}
	if _, err := adminClient.PutProjectMember(ctx, "test-work", joined.Agent.Name, api.ProjectMemberRequest{}); err != nil {
		t.Fatal(err)
	}
	code, out, e = admin.run("machine", "remove", "--id", id, "--dry-run", "--format", "json")
	if code != ExitDryRun || !strings.Contains(out, joined.Agent.Name) || !strings.Contains(out, "test-box-machine") {
		t.Fatalf("dry run: %d %s %s", code, out, e)
	}
	if _, err := adminClient.GetAgent(ctx, joined.Agent.Name); err != nil {
		t.Fatalf("dry run removed agent: %v", err)
	}
	code, out, e = admin.run("machine", "remove", "--id", id, "--format", "json")
	if code != 0 || !strings.Contains(out, joined.Agent.Name) {
		t.Fatalf("remove: %d %s %s", code, out, e)
	}
	if _, err := adminClient.GetAgent(ctx, joined.Agent.Name); err == nil {
		t.Fatal("agent remains")
	}
	if _, err := adminClient.GetAgent(ctx, "test-box-machine"); err == nil {
		t.Fatal("machine node remains")
	}
	aliases, err := adminClient.ListAliases(ctx)
	if err != nil || slices.ContainsFunc(aliases, func(a api.Alias) bool { return a.Alias == "test-helper" }) {
		t.Fatalf("aliases: %v %v", aliases, err)
	}
	p, err := adminClient.GetProject(ctx, "test-work", false)
	if err != nil || slices.ContainsFunc(p.Members, func(m api.ProjectMember) bool { return m.Agent != nil && m.Agent.Name == joined.Agent.Name }) {
		t.Fatalf("project members: %v %v", p, err)
	}
	mf, err := config.LoadMachine(filepath.Join(home, ".myco", "config.yaml"))
	if err != nil || mf == nil {
		t.Fatalf("machine config: %v", err)
	}
	enroller, _ := client.New(hub, mf.EnrollToken)
	if _, err := enroller.Enroll(ctx, api.EnrollRequest{Binding: api.Binding{Harness: "inbox", SessionID: "fresh", Environment: "test-box"}}, false); err == nil {
		t.Fatal("revoked machine credential enrolled a new node")
	}
}
