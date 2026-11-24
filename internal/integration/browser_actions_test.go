package integration_test

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/client"
)

// Exercise the real Hub transport. The trusted desktop is simulated here;
// the component adapter is tested separately in internal/console.
func TestBrowserBoundProfileGrantJob(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	admin := h.admin()
	token, err := admin.CreateConsoleToken(c, "browser-test-console")
	if err != nil {
		t.Fatal(err)
	}
	owner := h.client(token.Token)
	machineClient := h.client(h.register(api.RegisterRequest{Name: "browser-machine", Binding: api.Binding{Harness: "machine"}}))
	if err := machineClient.ReportMachine(c, api.MachineReport{Fingerprint: "browser-fp-0123456789abcdef", Name: "browser-computer", OS: "darwin"}); err != nil {
		t.Fatal(err)
	}
	machines, err := owner.Machines(c)
	if err != nil || len(machines) != 1 {
		t.Fatalf("machines: %v %v", machines, err)
	}
	machine := machines[0].ID
	agent := h.client(h.register(api.RegisterRequest{Name: "browser-agent", Binding: api.Binding{Harness: "claude", SessionID: "browser-session"}}))
	resource := &api.ActionResource{Provider: "browser", ID: "profile-work", Binding: strings.Repeat("a", 64), Name: "Work", Summary: "Dedicated browser profile"}
	req := api.ActionGrantRequest{Target: api.ActionTarget{Kind: "machine", ID: machine, Resource: resource}, Permissions: []string{"browser.view"}, Reason: "Review a saved run"}
	if _, err := agent.RequestActionGrant(c, req, false); err == nil {
		t.Fatal("unopened profile accepted")
	}
	reg := api.ActionHostRegister{MachineID: machine, Provider: "browser", Resource: resource, Generation: "browser-generation", Actions: []api.ActionDescriptor{
		{Name: "browser.run.status", Version: 1, Permission: "browser.view"},
		{Name: "browser.script.run", Version: 1, Permission: "browser.run"},
	}}
	bad := reg
	bad.Actions = []api.ActionDescriptor{{Name: "browser.page.act", Version: 1, Permission: "browser.view"}}
	if _, err := owner.RegisterActionHost(c, bad); err == nil {
		t.Fatal("control action disguised as view")
	}
	if _, err := agent.RegisterActionHost(c, reg); err == nil {
		t.Fatal("agent registered profile host")
	}
	host, err := owner.RegisterActionHost(c, reg)
	if err != nil {
		t.Fatal(err)
	}
	targets, err := agent.ActionTargets(c)
	if err != nil || len(targets) != 1 || targets[0].Resource.Provider != "browser" {
		t.Fatalf("targets: %+v %v", targets, err)
	}
	for _, permissions := range [][]string{{"browser.view", "remote.desktop.view"}, {"browser.view", "server.ssh.read"}} {
		mixed := req
		mixed.Permissions = permissions
		if _, err := agent.RequestActionGrant(c, mixed, false); err == nil {
			t.Fatal("mixed providers accepted")
		}
	}
	unbound := req
	unbound.Target.Resource = nil
	if _, err := agent.RequestActionGrant(c, unbound, false); err == nil {
		t.Fatal("unbound browser grant accepted")
	}
	grant, err := agent.RequestActionGrant(c, req, false)
	if err != nil {
		t.Fatal(err)
	}
	if grant.State != api.GrantPending {
		t.Fatalf("grant: %+v", grant)
	}
	if _, err := owner.DecideActionGrant(c, grant.ApprovalID, api.ActionGrantNarrow{Permissions: []string{"browser.run"}}); err == nil {
		t.Fatal("owner approval escalated requested permission")
	}
	if _, err := owner.DecideActionGrant(c, grant.ApprovalID, api.ActionGrantNarrow{}); err != nil {
		t.Fatal(err)
	}
	acts, err := agent.GrantActions(c, grant.ID)
	if err != nil || len(acts) != 1 || acts[0].Name != "browser.run.status" {
		t.Fatalf("actions: %+v %v", acts, err)
	}
	for _, action := range []string{"browser.script.run", "browser.page.act"} {
		if _, err := agent.RunAction(c, api.ActionInvocation{GrantID: grant.ID, Action: action, Version: 1, IdempotencyKey: action}); err == nil {
			t.Fatal("view grant escalated")
		}
	}
	job, err := agent.RunAction(c, api.ActionInvocation{GrantID: grant.ID, Action: "browser.run.status", Version: 1, Args: json.RawMessage(`{"run_id":"run-owned"}`), IdempotencyKey: "status"})
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := owner.ActionHostJobs(c, host.ID, time.Second)
	if err != nil || len(jobs) != 1 || jobs[0].Job.ID != job.ID {
		t.Fatalf("jobs: %+v %v", jobs, err)
	}
	png := []byte("\x89PNG fake private browser screenshot")
	art, err := owner.UploadActionArtifact(c, job.ID, host.ID, reg.Generation, "image/png", png)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := owner.CompleteActionJob(c, job.ID, api.ActionJobComplete{HostID: host.ID, Generation: reg.Generation, State: api.JobSucceeded, Result: json.RawMessage(`{"status":"completed","output":{"count":2}}`), ArtifactIDs: []string{art.ID}}); err != nil {
		t.Fatal(err)
	}
	result, err := agent.ActionJob(c, job.ID, time.Second)
	if err != nil || result.State != api.JobSucceeded || len(result.Artifacts) != 1 {
		t.Fatalf("result: %+v %v", result, err)
	}
	data, typ, err := agent.ActionArtifact(c, art.ID)
	if err != nil || typ != "image/png" || !bytes.Equal(data, png) {
		t.Fatalf("screenshot: %s %v", typ, err)
	}
	// A local policy revision replaces the host and immediately revokes the old binding.
	changed := *resource
	changed.Binding = strings.Repeat("b", 64)
	reg.Resource = &changed
	reg.Generation = "next-generation"
	host, err = owner.RegisterActionHost(c, reg)
	if err != nil {
		t.Fatal(err)
	}
	g, err := agent.ActionGrant(c, grant.ID, 0)
	if err != nil || g.State != api.GrantRevoked {
		t.Fatalf("binding change: %+v %v", g, err)
	}
	if _, err := agent.RequestActionGrant(c, req, false); err == nil {
		t.Fatal("stale binding accepted")
	}
	// Exact-generation withdrawal closes unused pending grants as well as live jobs.
	req.Target.Resource = &changed
	live, err := agent.RequestActionGrant(c, req, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := owner.DecideActionGrant(c, live.ApprovalID, api.ActionGrantNarrow{}); err != nil {
		t.Fatal(err)
	}
	running, err := agent.RunAction(c, api.ActionInvocation{GrantID: live.ID, Action: "browser.run.status", Version: 1, Args: json.RawMessage(`{"run_id":"owned"}`), IdempotencyKey: "running"})
	if err != nil {
		t.Fatal(err)
	}
	if jobs, err := owner.ActionHostJobs(c, host.ID, time.Second); err != nil || len(jobs) != 1 {
		t.Fatalf("running: %+v %v", jobs, err)
	}
	queued, err := agent.RunAction(c, api.ActionInvocation{GrantID: live.ID, Action: "browser.run.status", Version: 1, Args: json.RawMessage(`{"run_id":"owned"}`), IdempotencyKey: "queued"})
	if err != nil {
		t.Fatal(err)
	}
	pending, err := agent.RequestActionGrant(c, req, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.UnregisterActionHost(c, host.ID, "old-generation"); err == nil {
		t.Fatal("wrong generation withdrew host")
	} else if e, ok := client.AsError(err); !ok || e.Status != 409 {
		t.Fatalf("wrong generation: %v", err)
	}
	if err := agent.UnregisterActionHost(c, host.ID, reg.Generation); err == nil {
		t.Fatal("agent withdrew host")
	}
	if err := owner.UnregisterActionHost(c, host.ID, reg.Generation); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{live.ID, pending.ID} {
		g, err := agent.ActionGrant(c, id, 0)
		if err != nil || g.State != api.GrantRevoked {
			t.Fatalf("withdrawn grant: %+v %v", g, err)
		}
	}
	for _, id := range []string{running.ID, queued.ID} {
		j, err := agent.ActionJob(c, id, 0)
		if err != nil || j.State != api.JobCancelled || !j.CancelRequested || j.Error == nil || j.Error.Code != "revoked" {
			t.Fatalf("withdrawn job: %+v %v", j, err)
		}
	}
	if targets, err := agent.ActionTargets(c); err != nil || len(targets) != 0 {
		t.Fatalf("withdrawn targets: %+v %v", targets, err)
	}
	if approvals, err := owner.Approvals(c, "pending"); err != nil || len(approvals) != 0 {
		t.Fatalf("withdrawn approval card: %+v %v", approvals, err)
	}
	if _, err := owner.DecideActionGrant(c, pending.ApprovalID, api.ActionGrantNarrow{}); err == nil {
		t.Fatal("withdrawn pending grant approved")
	}
	if err := owner.UnregisterActionHost(c, host.ID, reg.Generation); err == nil {
		t.Fatal("withdrawal of absent host should be not_found")
	}

}
