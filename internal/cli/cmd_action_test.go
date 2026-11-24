package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
)

// The agent's side of actions from the command line: ask for a grant, wait
// while its owner approves, run an observe the computer's App answers, and
// get the screenshot saved to a file.
func TestActionCommandsGrantRunSave(t *testing.T) {
	hub := newHub(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dir := t.TempDir()
	admin, err := client.New(hub, "admin-token-123456")
	if err != nil {
		t.Fatal(err)
	}
	ct, err := admin.CreateConsoleToken(ctx, "glenn-mac")
	if err != nil {
		t.Fatal(err)
	}
	owner, _ := client.New(hub, ct.Token)
	reg, err := admin.Register(ctx, api.RegisterRequest{Name: "mac-machine", Binding: api.Binding{Harness: "machine"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	node, _ := client.New(hub, reg.Token)
	if err := node.ReportMachine(ctx, api.MachineReport{Fingerprint: "fp-cli-0123456789abcdef", Name: "studio", Hostname: "studio", OS: "darwin"}); err != nil {
		t.Fatal(err)
	}
	agent := &harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_CONFIG": filepath.Join(dir, "a.yaml"), "MYCO_ADMIN_TOKEN": ""}}
	if code, _, e := (&harnessT{t, map[string]string{"MYCO_HUB": hub, "MYCO_ADMIN_TOKEN": "admin-token-123456", "MYCO_CONFIG": filepath.Join(dir, "a.yaml")}}).
		run("agent", "register", "--name", "cloud-claude", "--harness", "claude", "--session-id", "s1"); code != 0 {
		t.Fatalf("register: %s", e)
	}

	if code, out, e := agent.run("action", "grant", "request", "--machine", "studio", "--permission", "remote.desktop.view", "--reason", "look", "--dry-run"); code != ExitDryRun {
		t.Fatalf("dry run: %d %s %s", code, out, e)
	}
	// The owner approves while the command waits.
	go func() {
		for i := 0; i < 100; i++ {
			if aps, err := owner.Approvals(ctx, "pending"); err == nil && len(aps) == 1 {
				_, _ = owner.DecideApproval(ctx, aps[0].ID, true)
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}()
	code, out, e := agent.run("action", "grant", "request", "--machine", "studio", "--permission", "remote.desktop.view", "--reason", "look", "--wait", "10s")
	if code != 0 {
		t.Fatalf("request: %d %s", code, e)
	}
	var g api.ActionGrant
	if err := json.Unmarshal([]byte(out), &g); err != nil || g.State != api.GrantActive {
		t.Fatalf("grant: %s %v", out, err)
	}
	ms, _ := owner.Machines(ctx)
	host, err := owner.RegisterActionHost(ctx, api.ActionHostRegister{MachineID: ms[0].ID, Provider: "remote.desktop", Generation: "g1",
		Actions: []api.ActionDescriptor{{Name: "remote.desktop.observe", Version: 1, Title: "Observe", Permission: "remote.desktop.view"}}})
	if err != nil {
		t.Fatal(err)
	}
	// The computer's App: takes the job, sends a screenshot.
	go func() {
		jobs, err := owner.ActionHostJobs(ctx, host.ID, 10*time.Second)
		if err != nil || len(jobs) == 0 {
			return
		}
		j := jobs[0].Job
		art, err := owner.UploadActionArtifact(ctx, j.ID, host.ID, "g1", "image/jpeg", []byte("jpeg bytes"))
		if err != nil {
			return
		}
		_, _ = owner.CompleteActionJob(ctx, j.ID, api.ActionJobComplete{HostID: host.ID, Generation: "g1", State: api.JobSucceeded,
			Result: json.RawMessage(`{"frame_id":"f1"}`), ArtifactIDs: []string{art.ID}})
	}()
	save := filepath.Join(dir, "screen.jpg")
	code, out, e = agent.run("action", "run", "--grant", g.ID, "--action", "remote.desktop.observe", "--save", save, "--wait", "10s")
	if code != 0 {
		t.Fatalf("run: %d %s", code, e)
	}
	var res struct {
		Job   api.ActionJob `json:"job"`
		Saved []string      `json:"saved"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil || res.Job.State != api.JobSucceeded || len(res.Saved) != 1 || res.Saved[0] != save {
		t.Fatalf("run: %s %v", out, err)
	}
	if data, err := os.ReadFile(save); err != nil || string(data) != "jpeg bytes" {
		t.Fatalf("saved: %q %v", data, err)
	}
	if code, _, e := agent.run("action", "grant", "end", "--id", g.ID); code != 0 {
		t.Fatalf("end: %s", e)
	}
	// Browser discovery has a distinct provider; never encode its profile as SSH.
	resource := &api.ActionResource{Provider: "browser", ID: "browser-work", Binding: "policy-1", Name: "Work"}
	if _, err := owner.RegisterActionHost(ctx, api.ActionHostRegister{MachineID: ms[0].ID, Provider: "browser", Resource: resource, Generation: "browser-1", Actions: []api.ActionDescriptor{{Name: "browser.profiles.list", Version: 1, Permission: "browser.view"}}}); err != nil {
		t.Fatal(err)
	}
	code, out, e = agent.run("action", "grant", "request", "--machine", "studio", "--browser-profile", resource.ID, "--binding", resource.Binding, "--permission", "browser.view", "--reason", "review a browser run")
	if code != 0 {
		t.Fatalf("browser request: %d %s", code, e)
	}
	var browser api.ActionGrant
	if err := json.Unmarshal([]byte(out), &browser); err != nil || browser.Target.Resource == nil || browser.Target.Resource.Provider != "browser" || browser.Target.Resource.Binding != "policy-1" {
		t.Fatalf("browser target: %s %v", out, err)
	}
	if code, _, _ := agent.run("action", "grant", "request", "--machine", "studio", "--browser-profile", resource.ID, "--ssh-profile", "ssh-profile", "--binding", resource.Binding, "--permission", "browser.view", "--reason", "invalid"); code != ExitUsage {
		t.Fatalf("mixed profile flags: %d", code)
	}

}
