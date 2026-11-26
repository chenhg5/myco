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
	for _, action := range []string{"browser.script.run", "browser.page.act", "browser.page.run"} {
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

func TestLiveBrowserGrantValidatesAuditsAndStops(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	token, err := h.admin().CreateConsoleToken(c, "control-test-console")
	if err != nil {
		t.Fatal(err)
	}
	owner := h.client(token.Token)
	machineClient := h.client(h.register(api.RegisterRequest{Name: "control-machine", Binding: api.Binding{Harness: "machine"}}))
	if err := machineClient.ReportMachine(c, api.MachineReport{Fingerprint: "control-fp-0123456789abcdef", Name: "control-computer", OS: "darwin"}); err != nil {
		t.Fatal(err)
	}
	machines, err := owner.Machines(c)
	if err != nil || len(machines) != 1 {
		t.Fatalf("machines: %v %v", machines, err)
	}
	agent := h.client(h.register(api.RegisterRequest{Name: "control-agent", Binding: api.Binding{Harness: "claude", SessionID: "control-session"}}))
	resource := &api.ActionResource{Provider: "browser", ID: "work", Binding: strings.Repeat("a", 64), Name: "agent 专用", Kind: api.BrowserAgentProfile}
	target := api.ActionTarget{Kind: "machine", ID: machines[0].ID, Resource: resource}
	req := api.ActionGrantRequest{Target: api.ActionTarget{Kind: "machine", ID: target.ID}, Hosts: []string{"EXAMPLE.com.", "*.docs.example.com"}, Permissions: []string{api.BrowserControlPermission}, Reason: "Check the sample report", TTLSeconds: 3600}
	reg := api.ActionHostRegister{MachineID: target.ID, Provider: "browser", Resource: resource, Generation: "control-1",
		Actions: []api.ActionDescriptor{{Name: "browser.run.status", Version: 1, Permission: "browser.view"}}}
	if _, err := owner.RegisterActionHost(c, reg); err != nil {
		t.Fatal(err)
	}
	if _, err := agent.RequestActionGrant(c, req, false); err == nil {
		t.Fatal("an older component should not accept a control grant")
	}
	reg.Actions = append(reg.Actions, api.BrowserControlDescriptors()...)
	reg.Generation = "control-2"
	if _, err := owner.RegisterActionHost(c, reg); err != nil {
		t.Fatal(err)
	}
	if _, err := agent.RequestActionGrant(c, req, false); err == nil {
		t.Fatal("a desktop without host scope support accepted a scoped grant")
	}
	reg.Generation, reg.BrowserHostScope = "control-3", true
	host, err := owner.RegisterActionHost(c, reg)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := agent.RequestActionGrant(c, req, false)
	if err != nil {
		t.Fatal(err)
	}
	if grant.Target.Resource == nil || grant.Target.Resource.ID != resource.ID || grant.Target.Resource.Kind != api.BrowserAgentProfile {
		t.Fatalf("expected dedicated profile to be selected: %+v", grant)
	}
	for _, invalid := range [][]string{{"other.example"}, {}} {
		if _, err := owner.DecideActionGrant(c, grant.ApprovalID, api.ActionGrantNarrow{Hosts: &invalid}); err == nil {
			t.Fatal("approval broadened the requested scope")
		}
	}
	approved := []string{"*.docs.example.com"}
	if _, err := owner.DecideActionGrant(c, grant.ApprovalID, api.ActionGrantNarrow{Hosts: &approved}); err != nil {
		t.Fatal(err)
	}
	grant, err = agent.ActionGrant(c, grant.ID, 0)
	if err != nil || grant.ExpiresAt == nil || time.Until(*grant.ExpiresAt) > 30*time.Minute {
		t.Fatalf("grant: %+v %v", grant, err)
	}
	actions, err := agent.GrantActions(c, grant.ID)
	if err != nil || len(actions) != 7 {
		t.Fatalf("expected seven control-only actions: %+v %v", actions, err)
	}
	run := func(action, key, args string) (*api.ActionJob, error) {
		return agent.RunAction(c, api.ActionInvocation{GrantID: grant.ID, Action: "browser." + action,
			Version: 1, Args: json.RawMessage(args), IdempotencyKey: key})
	}

	// A replay graph has its own larger transport budget. Validation must still
	// finish before the trusted host can claim any website operation.
	badGraph := `{"session":"session","graph":{"nodes":[{"id":"forbidden","type":"js","code":""}],"edges":[]}}`
	if _, err := run("page.run", "invalid-replay", badGraph); err == nil {
		t.Fatal("executable graph queued")
	} else if failure, ok := client.AsError(err); !ok || failure.Code != "invalid_argument" || failure.ErrorNode != "forbidden" {
		t.Fatalf("preflight node did not cross HTTP/client: %+v %v", failure, err)
	}
	largeGraph := map[string]any{"nodes": []any{map[string]any{"id": "note", "type": "note", "note": strings.Repeat("public fixture ", 6000)}}, "edges": [][]string{}}
	replayArgs, _ := json.Marshal(map[string]any{"session": "session", "graph": largeGraph})
	replayJob, err := run("page.run", "large-replay", string(replayArgs))
	if err != nil {
		t.Fatal("graph larger than the generic 64 KiB budget was rejected", err)
	}
	replayJobs, err := owner.ActionHostJobs(c, host.ID, 0)
	if err != nil || len(replayJobs) != 1 || replayJobs[0].Job.ID != replayJob.ID || !bytes.Equal(replayJobs[0].Args, replayArgs) {
		t.Fatalf("replay queue: %+v %v", replayJobs, err)
	}
	if _, err = owner.CompleteActionJob(c, replayJob.ID, api.ActionJobComplete{HostID: host.ID, Generation: host.Generation, State: api.JobSucceeded, Result: json.RawMessage(`{"ok":true,"state":"succeeded","steps":[{"node":"note","state":"ok","ms":0}],"output":{},"url":"https://docs.example.com/","tabs":[]}`)}); err != nil {
		t.Fatal(err)
	}
	largeGraph["nodes"] = []any{map[string]any{"id": "large", "type": "note", "note": strings.Repeat("x", api.BrowserReplayGraphMax)}}
	tooLarge, _ := json.Marshal(map[string]any{"session": "session", "graph": largeGraph})
	if _, err = run("page.run", "too-large-replay", string(tooLarge)); err == nil {
		t.Fatal("graph exceeded its 192 KiB cap")
	}
	for _, args := range []string{`{"profile":"other"}`, `{"profile":"work","headless":true}`, `{"profile":"work","actor":"someone-else"}`} {
		if _, err := run("page.open", args, args); err == nil {
			t.Fatal("unsafe or unbound open was accepted")
		}
	}
	if _, err := run("run.status", "run-status", `{"run_id":"run-sample"}`); err == nil {
		t.Fatal("a control grant included view")
	}
	if _, err := run("page.act", "bad-op", `{"session":"session","op":"eval","value":"0"}`); err == nil {
		t.Fatal("unknown action operation was accepted")
	}
	if _, err := run("page.act", "template", `{"session":"session","op":"type","ref":"e1","value":"${example}"}`); err == nil {
		t.Fatal("a live action accepted a draft parameter")
	}
	job, err := run("page.act", "safe-act", `{"session":"session","op":"type","ref":"e1","value":"not-for-audit"}`)
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := owner.ActionHostJobs(c, host.ID, 0)
	if err != nil || len(jobs) != 1 || jobs[0].Job.ID != job.ID || strings.Join(jobs[0].Grant.Hosts, ",") != approved[0] {
		t.Fatalf("host jobs: %+v %v", jobs, err)
	}
	done := api.ActionJobComplete{HostID: host.ID, Generation: host.Generation, State: api.JobSucceeded,
		Result: json.RawMessage(`{"snapshot_error":{"code":"timeout"},"step_ids":["step1"]}`), BrowserHost: "example.com"}
	bad := done
	bad.BrowserHost = "example.com/private?q=secret"
	if _, err := owner.CompleteActionJob(c, job.ID, bad); err == nil {
		t.Fatal("a complete URL was accepted as the audit hostname")
	}
	if _, err := owner.CompleteActionJob(c, job.ID, done); err != nil {
		t.Fatal(err)
	}
	_, _ = owner.CompleteActionJob(c, job.ID, done)
	audit, err := owner.Audit(c, client.AuditFilter{Target: "example.com"})
	if err != nil || len(audit) != 1 || audit[0].Action != "browser.page.act" || audit[0].Target != "example.com" || audit[0].Detail != "" {
		t.Fatalf("expected one hostname-only audit entry: %+v %v", audit, err)
	}
	replay, err := run("page.act", "safe-act", `{"session":"session","op":"type","ref":"e1","value":"not-for-audit"}`)
	if err != nil || replay.ID != job.ID || !bytes.Contains(replay.Result, []byte("snapshot_error")) {
		t.Fatalf("completed idempotent job lost its snapshot status: %+v %v", replay, err)
	}
	blocked, err := run("page.act", "outside-scope", `{"session":"session","op":"navigate","url":"https://elsewhere.example/path"}`)
	if err != nil {
		t.Fatal(err)
	}
	if jobs, err := owner.ActionHostJobs(c, host.ID, 0); err != nil || len(jobs) != 1 || jobs[0].Job.ID != blocked.ID {
		t.Fatalf("host jobs: %+v %v", jobs, err)
	}
	if _, err := owner.CompleteActionJob(c, blocked.ID, api.ActionJobComplete{
		HostID: host.ID, Generation: host.Generation, State: api.JobFailed, BrowserHost: "elsewhere.example",
		Error: &api.ActionError{Code: "host_not_allowed", Message: "outside the approved domains"},
	}); err != nil {
		t.Fatal(err)
	}
	if audit, err := owner.Audit(c, client.AuditFilter{Target: "elsewhere.example"}); err != nil || len(audit) != 1 || audit[0].Action != "browser.page.act" || audit[0].Detail != "host_not_allowed" {
		t.Fatalf("expected audit for rejected navigation: %+v %v", audit, err)
	}
	snap, err := run("page.screenshot", "screen", `{"session":"session"}`)
	if err != nil {
		t.Fatal(err)
	}
	if jobs, err := owner.ActionHostJobs(c, host.ID, 0); err != nil || len(jobs) != 1 || jobs[0].Job.ID != snap.ID {
		t.Fatalf("host jobs: %+v %v", jobs, err)
	}
	image := []byte("private raster test data")
	artifact, err := owner.UploadActionArtifact(c, snap.ID, host.ID, host.Generation, "image/png", image)
	if err != nil {
		t.Fatal(err)
	}
	if left := time.Until(artifact.ExpiresAt); left > 7*24*time.Hour || left < 7*24*time.Hour-time.Minute {
		t.Fatalf("unexpected screenshot lifetime: %s", left)
	}
	if _, _, err := agent.ActionArtifact(c, artifact.ID); err == nil {
		t.Fatal("staged screenshot was accessible")
	}
	if _, err := owner.CompleteActionJob(c, snap.ID, api.ActionJobComplete{
		HostID: host.ID, Generation: host.Generation, State: api.JobSucceeded,
		BrowserHost: "screenshot.docs.example.com", ArtifactIDs: []string{artifact.ID},
	}); err != nil {
		t.Fatal(err)
	}
	invitation, err := h.admin().InviteMember(c, api.InviteRequest{Name: "browser-outsider"})
	if err != nil {
		t.Fatal(err)
	}
	pair, err := h.client("").Pair(c, invitation.Link[strings.Index(invitation.Link, "/c/")+3:], "other-computer")
	if err != nil {
		t.Fatal(err)
	}
	otherOwner := h.client(pair.Token)
	if _, _, err := otherOwner.OwnerBrowserScreenshot(c, artifact.ID); err == nil {
		t.Fatal("another member read a private screenshot")
	}
	if list, err := otherOwner.OwnerBrowserScreenshots(c); err != nil || len(list) != 0 {
		t.Fatalf("another member could list screenshot metadata: %+v %v", list, err)
	}
	otherAgent := h.client(h.register(api.RegisterRequest{Name: "other-control-agent", Binding: api.Binding{Harness: "claude", SessionID: "other-control-session"}}))
	if _, _, err := otherAgent.ActionArtifact(c, artifact.ID); err == nil {
		t.Fatal("another agent of the same owner read the screenshot")
	}
	if audit, err := owner.Audit(c, client.AuditFilter{Target: "screenshot.docs.example.com"}); err != nil || len(audit) != 1 || audit[0].Detail != "screenshot" {
		t.Fatalf("expected presence-only screenshot audit: %+v %v", audit, err)
	}
	if _, err := owner.RevokeActionGrantWithReason(c, grant.ID, api.BrowserControlOwnerStopped); err != nil {
		t.Fatal(err)
	}
	if data, _, err := agent.ActionArtifact(c, artifact.ID); err != nil || !bytes.Equal(data, image) {
		t.Fatalf("agent could not read the retained screenshot: %v", err)
	}
	if data, _, err := owner.OwnerBrowserScreenshot(c, artifact.ID); err != nil || !bytes.Equal(data, image) {
		t.Fatalf("owner could not read the retained screenshot: %v", err)
	}
	if list, err := owner.OwnerBrowserScreenshots(c); err != nil || len(list) != 1 || list[0].ID != artifact.ID {
		t.Fatalf("owner screenshot list: %+v %v", list, err)
	}
	closed, err := run("page.close", "close-after-stop", `{"session":"session"}`)
	if err != nil || closed.State != api.JobFailed || closed.Error == nil || closed.Error.Code != "session_closed" || closed.Error.Reason != api.BrowserControlOwnerStopped {
		t.Fatalf("after owner stop: %+v %v", closed, err)
	}
}
