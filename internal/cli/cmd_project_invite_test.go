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

func TestAutomationCoordinatorCLIAndWindowCompatibility(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		if r.Method == "GET" {
			_ = json.NewEncoder(w).Encode(api.AutomationList{Automations: []api.Automation{}})
			return
		}
		var req api.AutomationRequest
		if r.Method != "DELETE" {
			if e := json.NewDecoder(r.Body).Decode(&req); e != nil {
				t.Error(e)
			}
		}
		if r.Method == "POST" && (req.StartsAt == nil || *req.StartsAt != "2026-11-26T00:00:00Z" || req.EndsAt == nil || *req.EndsAt != "2026-11-29T00:00:00Z" || req.Once == nil || !*req.Once) {
			t.Error("CLI lost date/once fields")
		}
		_ = json.NewEncoder(w).Encode(api.ProjectAutomationResponse{Status: "pending", ApprovalID: "approval_fixture"})
	}))
	defer server.Close()
	cli := &harnessT{t, map[string]string{"MYCO_HUB": server.URL, "MYCO_TOKEN": "agent-fixture", "MYCO_CONFIG": filepath.Join(t.TempDir(), "agent.yaml")}}
	if code, _, err := cli.run("automation", "list"); code == 0 || !strings.Contains(err, "--project") {
		t.Fatalf("agent did not require project %d %s", code, err)
	}
	commands := [][]string{{"automation", "create", "--project", "demo", "--name", "demo", "--to", "worker", "--body", "check", "--cron", "0 9 * * *", "--starts", "2026-11-26", "--ends", "2026-11-28", "--once"}, {"automation", "list", "--project", "demo"}, {"automation", "update", "--project", "demo", "--id", "aut_demo", "--disabled"}, {"automation", "remove", "--project", "demo", "--id", "aut_demo"}}
	for _, args := range commands {
		if code, _, err := cli.run(args...); code != 0 {
			t.Fatalf("%v: %d %s", args, code, err)
		}
	}
	if len(paths) != 4 || paths[0] != "POST /v1/projects/demo/automations" || paths[1] != "GET /v1/projects/demo/automations" || paths[2] != "PATCH /v1/projects/demo/automations/aut_demo" || paths[3] != "DELETE /v1/projects/demo/automations/aut_demo" {
		t.Fatalf("wrong identity route %v", paths)
	}
	old := httptest.NewServer(http.NotFoundHandler())
	defer old.Close()
	cli.env["MYCO_HUB"] = old.URL
	if code, _, err := cli.run("automation", "list", "--project", "demo"); code == 0 || !strings.Contains(err, "upgrade") {
		t.Fatal("no old Hub hint", err)
	}
	// The existing human route also gives an actionable error for new fields.
	if err := automationUpgradeError(&client.Error{Status: 400, Message: `json: unknown field "starts_at"`}); !strings.Contains(err.Error(), "Hub") {
		t.Fatal("new date field did not recommend upgrade")
	}
}
