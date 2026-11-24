package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/config"
)

func TestProjectInviteCLIWaitAndOldHub(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		out := api.ProjectInviteResponse{ID: "inv", Status: "pending", ExpiresAt: time.Now().Add(time.Hour)}
		if r.Method == "GET" {
			out.Status = "approved"
		}
		if strings.HasSuffix(r.URL.Path, "/claim") {
			out.Status = "ready"
			out.Link = "https://fixture.invalid/j/credential"
			out.Sentence = "Join myco: " + out.Link
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	defer server.Close()
	cli := &harnessT{t, map[string]string{"MYCO_HUB": server.URL, "MYCO_TOKEN": "agent-fixture", "MYCO_CONFIG": filepath.Join(t.TempDir(), "agent.yaml")}}
	code, out, stderr := cli.run("project", "invite-agent", "--name", "fixture", "--wait", "2s")
	if code != 0 {
		t.Fatalf("CLI %d %s", code, stderr)
	}
	var result api.ProjectInviteResponse
	if err := json.Unmarshal([]byte(out), &result); err != nil || result.Link == "" || calls.Load() != 3 {
		t.Fatalf("wait/claim failed %s %v %d", out, err, calls.Load())
	}
	before := calls.Load()
	if code, _, _ := cli.run("project", "invite-agent", "--name", "fixture", "--wait=-1s"); code == 0 || calls.Load() != before {
		t.Fatal("invalid flags created invitation")
	}
	old := httptest.NewServer(http.NotFoundHandler())
	defer old.Close()
	cli.env["MYCO_HUB"] = old.URL
	code, _, stderr = cli.run("project", "invite-agent", "--name", "fixture")
	if code == 0 || !strings.Contains(stderr, "upgrade") {
		t.Fatalf("missing old Hub upgrade hint %s", stderr)
	}
}

// An invitation must win over existing machine credentials, including when
// that machine already belongs to the same Hub under a different enrollment.
func TestProjectInvitationCLIJoinIgnoresMachineEnrollment(t *testing.T) {
	hub := newHub(t)
	ctx := context.Background()
	admin, _ := client.New(hub, "admin-token-123456")
	p, err := admin.CreateProject(ctx, api.CreateProjectRequest{Name: "invite-cli"})
	if err != nil {
		t.Fatal(err)
	}
	reg, err := admin.Register(ctx, api.RegisterRequest{Name: "coordinator", Binding: api.Binding{Harness: "inbox", SessionID: "coordinator-session"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	role := "coordinator"
	if _, err = admin.PutProjectMember(ctx, p.Name, reg.Agent.Name, api.ProjectMemberRequest{Role: &role}); err != nil {
		t.Fatal(err)
	}
	if _, err = admin.SetProjectCoordinatorInvites(ctx, p.Name, true); err != nil {
		t.Fatal(err)
	}
	coord, _ := client.New(hub, reg.Token)
	invite, err := coord.InviteProjectAgent(ctx, p.Name, api.ProjectInviteRequest{DisplayName: "Invited helper", Note: "Documentation"}, false)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	path := filepath.Join(home, "agent.yaml")
	machine := &config.Machine{MachineHub: config.MachineHub{Hub: hub, Machine: "existing", EnrollToken: "wrong-machine-credential"}}
	if err = config.SaveMachine(path, machine); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(home, "machine.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	cli := &harnessT{t, map[string]string{"HOME": home, "MYCO_CONFIG": path, "MYCO_HUB": "http://wrong.invalid", "MYCO_ENROLL_TOKEN": strings.TrimPrefix(invite.Link, hub+"/j/"), "CODEX_HOME": filepath.Join(home, "codex"), "CLAUDE_CONFIG_DIR": filepath.Join(home, "claude")}}
	code, _, stderr := cli.run("join", "--invitation", "--hub", hub, "--harness", "inbox", "--session-id", "new-session", "--name", "invited-helper", "--no-env", "--no-memory")
	if code != 0 {
		t.Fatalf("invitation join: %d %s", code, stderr)
	}
	agent, err := admin.GetAgent(ctx, "invited-helper")
	if err != nil {
		t.Fatal(err)
	}
	if agent.Owner != p.Owner || agent.InvitedBy != "coordinator" || agent.DisplayName != "Invited helper" {
		t.Fatalf("wrong invitation identity: %+v", agent)
	}
	project, err := admin.GetProject(ctx, p.Name, false)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range project.Members {
		if m.Name == agent.Name && m.Role == "member" && m.Note == "Documentation" {
			found = true
		}
	}
	if !found {
		t.Fatal("agent did not join project as ordinary member")
	}
	after, err := os.ReadFile(filepath.Join(home, "machine.yaml"))
	if err != nil || string(before) != string(after) {
		t.Fatal("invitation altered machine membership")
	}
}
