package integration_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/app"
	"github.com/chenhg5/myco/internal/client"
)

// An agent asks to act on a computer; its owner approves (less than asked);
// the computer's App, hosting the remote.desktop provider, carries out what
// the agent runs and returns a private screenshot; input needs control;
// another agent cannot use the grant; revoking ends it all.
func TestAgentActionsGrantJobArtifact(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	admin := h.admin()
	if _, err := admin.UpdateMember(c, "owner", api.MemberUpdateRequest{Name: "glenn"}); err != nil {
		t.Fatal(err)
	}
	ct, err := admin.CreateConsoleToken(c, "glenn-mac")
	if err != nil {
		t.Fatal(err)
	}
	glenn := h.client(ct.Token)
	node := h.client(h.register(api.RegisterRequest{Name: "mac-machine", Binding: api.Binding{Harness: "machine"}}))
	if err := node.ReportMachine(c, api.MachineReport{Fingerprint: "fp-mac-0123456789abcdef", Name: "studio", Hostname: "studio", OS: "darwin"}); err != nil {
		t.Fatal(err)
	}
	ms, err := glenn.Machines(c)
	if err != nil || len(ms) == 0 {
		t.Fatalf("machines: %v %v", ms, err)
	}
	machine := ms[0].ID
	agent := h.client(h.register(api.RegisterRequest{Name: "cloud-claude", Binding: api.Binding{Harness: "claude", SessionID: "s1"}}))
	other := h.client(h.register(api.RegisterRequest{Name: "other", Binding: api.Binding{Harness: "codex", SessionID: "s2"}}))

	// No host yet: the grant can still be asked for (its owner decides).
	req := api.ActionGrantRequest{Target: api.ActionTarget{Kind: "machine", ID: machine},
		Permissions: []string{"remote.desktop.control"}, Reason: "check the build window", TTLSeconds: 600}
	if dry, err := agent.RequestActionGrant(c, req, true); err != nil || len(dry.Requested) != 2 {
		t.Fatalf("dry run: %+v %v", dry, err)
	}
	if _, err := agent.RequestActionGrant(c, api.ActionGrantRequest{Target: req.Target, Permissions: []string{"remote.desktop.*"}, Reason: "x"}, false); err == nil {
		t.Fatal("a wildcard permission was accepted")
	}
	g, err := agent.RequestActionGrant(c, req, false)
	if err != nil {
		t.Fatal(err)
	}
	if g.State != api.GrantPending || g.Owner != "glenn" || len(g.Requested) != 2 {
		t.Fatalf("grant: %+v", g)
	}
	// Pending: nothing runs.
	if _, err := agent.RunAction(c, api.ActionInvocation{GrantID: g.ID, Action: "remote.desktop.observe", Version: 1, IdempotencyKey: "k0"}); err == nil {
		t.Fatal("a pending grant ran an action")
	}
	// The owner sees a typed approval and allows view only, for 2 minutes.
	aps, err := glenn.Approvals(c, "pending")
	if err != nil || len(aps) != 1 || aps[0].Kind != "action_grant" {
		t.Fatalf("approvals: %+v %v", aps, err)
	}
	if _, err := glenn.DecideActionGrant(c, aps[0].ID, api.ActionGrantNarrow{TTLSeconds: 3600}); err == nil {
		t.Fatal("approved for longer than asked")
	}
	if _, err := glenn.DecideActionGrant(c, aps[0].ID, api.ActionGrantNarrow{Permissions: []string{"remote.desktop.view"}, TTLSeconds: 120}); err != nil {
		t.Fatal(err)
	}
	g, err = agent.ActionGrant(c, g.ID, time.Second)
	if err != nil || g.State != api.GrantActive || len(g.Permissions) != 1 || g.Permissions[0] != "remote.desktop.view" || g.ExpiresAt == nil {
		t.Fatalf("active grant: %+v %v", g, err)
	}
	// Another agent cannot see or use it.
	if _, err := other.ActionGrant(c, g.ID, 0); err == nil {
		t.Fatal("another agent read the grant")
	}
	if _, err := other.RunAction(c, api.ActionInvocation{GrantID: g.ID, Action: "remote.desktop.observe", Version: 1, IdempotencyKey: "x"}); err == nil {
		t.Fatal("another agent used the grant")
	}
	// No host: unsupported, clearly.
	_, err = agent.RunAction(c, api.ActionInvocation{GrantID: g.ID, Action: "remote.desktop.observe", Version: 1, IdempotencyKey: "k1"})
	if e, ok := client.AsError(err); !ok || e.Code != "unsupported" {
		t.Fatalf("no host: %v", err)
	}
	// An agent cannot host; the owner's App can.
	reg := api.ActionHostRegister{MachineID: machine, Provider: "remote.desktop", Generation: "gen1", Actions: []api.ActionDescriptor{
		{Name: "remote.desktop.observe", Version: 1, Title: "Observe", Permission: "remote.desktop.view"},
		{Name: "remote.desktop.input", Version: 1, Title: "Input", Permission: "remote.desktop.control"},
	}}
	if _, err := agent.RegisterActionHost(c, reg); err == nil {
		t.Fatal("an agent registered a host")
	}
	if _, err := glenn.RegisterActionHost(c, api.ActionHostRegister{MachineID: machine, Provider: "os.shell", Generation: "g", Actions: reg.Actions}); err == nil {
		t.Fatal("an unknown provider was registered")
	}
	host, err := glenn.RegisterActionHost(c, reg)
	if err != nil {
		t.Fatal(err)
	}
	acts, err := agent.GrantActions(c, g.ID)
	if err != nil || len(acts) != 1 || acts[0].Name != "remote.desktop.observe" {
		t.Fatalf("actions (view only): %+v %v", acts, err)
	}
	// Input needs control, which was not granted.
	if _, err := agent.RunAction(c, api.ActionInvocation{GrantID: g.ID, Action: "remote.desktop.input", Version: 1, IdempotencyKey: "in1",
		Args: json.RawMessage(`{"frame_id":"f","events":[]}`)}); err == nil {
		t.Fatal("input ran on a view-only grant")
	}
	// Observe: queued, the host takes it, uploads a screenshot, completes.
	job, err := agent.RunAction(c, api.ActionInvocation{GrantID: g.ID, Action: "remote.desktop.observe", Version: 1, IdempotencyKey: "obs1", Args: json.RawMessage(`{"max_width":800}`)})
	if err != nil || job.State != api.JobQueued {
		t.Fatalf("observe: %+v %v", job, err)
	}
	again, err := agent.RunAction(c, api.ActionInvocation{GrantID: g.ID, Action: "remote.desktop.observe", Version: 1, IdempotencyKey: "obs1", Args: json.RawMessage(`{"max_width":800}`)})
	if err != nil || again.ID != job.ID {
		t.Fatalf("same key, same body: %+v %v", again, err)
	}
	if _, err := agent.RunAction(c, api.ActionInvocation{GrantID: g.ID, Action: "remote.desktop.observe", Version: 1, IdempotencyKey: "obs1", Args: json.RawMessage(`{"max_width":900}`)}); err == nil {
		t.Fatal("same key, different body was accepted")
	}
	jobs, err := glenn.ActionHostJobs(c, host.ID, 2*time.Second)
	if err != nil || len(jobs) != 1 || jobs[0].Job.ID != job.ID || jobs[0].Grant.ID != g.ID || string(jobs[0].Args) != `{"max_width":800}` {
		t.Fatalf("host jobs: %+v %v", jobs, err)
	}
	png := []byte("\x89PNG fake screenshot")
	if _, err := glenn.UploadActionArtifact(c, job.ID, host.ID, "gen0", "image/png", png); err == nil {
		t.Fatal("an old generation uploaded")
	}
	art, err := glenn.UploadActionArtifact(c, job.ID, host.ID, "gen1", "image/png", png)
	if err != nil {
		t.Fatal(err)
	}
	result, _ := json.Marshal(map[string]any{"frame_id": "f1", "width": 800, "height": 500, "artifact": art})
	if _, err := glenn.CompleteActionJob(c, job.ID, api.ActionJobComplete{HostID: host.ID, Generation: "gen1", State: api.JobSucceeded,
		Result: result, ArtifactIDs: []string{art.ID}}); err != nil {
		t.Fatal(err)
	}
	if _, err := glenn.CompleteActionJob(c, job.ID, api.ActionJobComplete{HostID: host.ID, Generation: "gen1", State: api.JobFailed}); err == nil {
		t.Fatal("a finished job was changed")
	}
	done, err := agent.ActionJob(c, job.ID, time.Second)
	if err != nil || done.State != api.JobSucceeded || len(done.Artifacts) != 1 {
		t.Fatalf("done: %+v %v", done, err)
	}
	data, typ, err := agent.ActionArtifact(c, art.ID)
	if err != nil || !bytes.Equal(data, png) || typ != "image/png" {
		t.Fatalf("artifact: %q %s %v", data, typ, err)
	}
	if _, _, err := other.ActionArtifact(c, art.ID); err == nil {
		t.Fatal("another agent downloaded the screenshot")
	}

	// A job still running when its owner revokes: its late result is not
	// kept, the screenshot is gone, nothing more runs.
	job2, err := agent.RunAction(c, api.ActionInvocation{GrantID: g.ID, Action: "remote.desktop.observe", Version: 1, IdempotencyKey: "obs2"})
	if err != nil {
		t.Fatal(err)
	}
	if jobs, err := glenn.ActionHostJobs(c, host.ID, 2*time.Second); err != nil || len(jobs) != 1 {
		t.Fatalf("host jobs 2: %+v %v", jobs, err)
	}
	// The host asks whether to stop while it runs: not yet; the agent
	// cancelling says stop at once, and so does a revoke (waking a wait).
	if st, err := glenn.ActionHostJob(c, job2.ID, host.ID, "gen1", 0); err != nil || st.Stop || st.GrantState != api.GrantActive || st.GrantExpiresAt == nil {
		t.Fatalf("host job state: %+v %v", st, err)
	}
	if _, err := glenn.ActionHostJob(c, job2.ID, host.ID, "gen0", 0); err == nil {
		t.Fatal("an old generation read a job")
	}
	stopped := make(chan *api.ActionHostJobState, 1)
	go func() { st, _ := glenn.ActionHostJob(c, job2.ID, host.ID, "gen1", 20*time.Second); stopped <- st }()
	time.Sleep(200 * time.Millisecond)
	owned, err := glenn.OwnerActionGrants(c, api.GrantActive)
	if err != nil || len(owned) != 1 {
		t.Fatalf("owner grants: %+v %v", owned, err)
	}
	if _, err := agent.RevokeActionGrant(c, g.ID); err == nil {
		t.Fatal("an agent revoked through the owner's route")
	}
	if rg, err := glenn.RevokeActionGrant(c, g.ID); err != nil || rg.State != api.GrantRevoked {
		t.Fatalf("revoke: %+v %v", rg, err)
	}
	select {
	case st := <-stopped:
		if st == nil || !st.Stop || st.GrantState != api.GrantRevoked {
			t.Fatalf("host job after revoke: %+v", st)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the host was not told to stop when the grant was revoked")
	}
	late, err := glenn.CompleteActionJob(c, job2.ID, api.ActionJobComplete{HostID: host.ID, Generation: "gen1", State: api.JobSucceeded, Result: json.RawMessage(`{"secret":1}`)})
	if err != nil || late.State != api.JobCancelled || len(late.Result) != 0 {
		t.Fatalf("late result: %+v %v", late, err)
	}
	if _, _, err := agent.ActionArtifact(c, art.ID); err == nil {
		t.Fatal("the screenshot outlived the grant")
	}
	if _, err := agent.RunAction(c, api.ActionInvocation{GrantID: g.ID, Action: "remote.desktop.observe", Version: 1, IdempotencyKey: "obs3"}); err == nil {
		t.Fatal("a revoked grant ran an action")
	}

	// Denied: the grant says so; ending a pending one works too.
	g2, err := agent.RequestActionGrant(c, req, false)
	if err != nil {
		t.Fatal(err)
	}
	aps, _ = glenn.Approvals(c, "pending")
	if len(aps) != 1 {
		t.Fatalf("pending approvals: %+v", aps)
	}
	if _, err := glenn.DecideApproval(c, aps[0].ID, false); err != nil {
		t.Fatal(err)
	}
	if g2, err = agent.ActionGrant(c, g2.ID, 0); err != nil || g2.State != api.GrantDenied {
		t.Fatalf("denied: %+v %v", g2, err)
	}
	g3, err := agent.RequestActionGrant(c, req, false)
	if err != nil {
		t.Fatal(err)
	}
	if g3, err = agent.EndActionGrant(c, g3.ID, false); err != nil || g3.State != api.GrantEnded {
		t.Fatalf("ended: %+v %v", g3, err)
	}
	aps, _ = glenn.Approvals(c, "pending")
	if _, err := glenn.DecideActionGrant(c, aps[0].ID, api.ActionGrantNarrow{}); err == nil {
		t.Fatal("approved a request its agent had ended")
	}
}

// A shared page's export: an agent that may see the page gets an active
// grant at once (no approval: the file is what it can already read); only a
// renderer of someone who also sees the page runs it; seeing is checked
// again, and its owner can revoke.
func TestPageExportGrantsAndRenderers(t *testing.T) {
	c := ctx(t)
	h := startHub(t, filepath.Join(t.TempDir(), "hub.db"), app.DefaultMessagePolicy())
	admin := h.admin()
	if _, err := admin.UpdateMember(c, "owner", api.MemberUpdateRequest{Name: "glenn"}); err != nil {
		t.Fatal(err)
	}
	ct, err := admin.CreateConsoleToken(c, "glenn-mac")
	if err != nil {
		t.Fatal(err)
	}
	glenn := h.client(ct.Token)
	maker := h.client(h.register(api.RegisterRequest{Name: "maker", Binding: api.Binding{Harness: "claude", SessionID: "s1"}}))
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	w, _ := z.Create("index.html")
	_, _ = w.Write([]byte("<!doctype html><title>board</title><p>board</p>"))
	_ = z.Close()
	page, err := maker.PublishPage(c, client.PageInput{Name: "白板"}, bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	// Another person, with an agent of their own: they do not see the page.
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
	outsider := h.client(res.Token)

	target := api.ActionTarget{Kind: "page", ID: page.ID}
	g, err := maker.RequestActionGrant(c, api.ActionGrantRequest{Target: target, Permissions: []string{"page.export"}, Reason: "export the board"}, false)
	if err != nil || g.State != api.GrantActive || g.Owner != "glenn" || g.ExpiresAt == nil || g.ApprovalID != "" {
		t.Fatalf("page grant: %+v %v", g, err)
	}
	if aps, _ := glenn.Approvals(c, "pending"); len(aps) != 0 {
		t.Fatalf("a page export asked for approval: %+v", aps)
	}
	if _, err := outsider.RequestActionGrant(c, api.ActionGrantRequest{Target: target, Permissions: []string{"page.export"}, Reason: "x"}, false); err == nil {
		t.Fatal("an agent that cannot see the page got a grant")
	}
	if _, err := maker.RequestActionGrant(c, api.ActionGrantRequest{Target: target, Permissions: []string{"remote.desktop.view"}, Reason: "x"}, false); err == nil {
		t.Fatal("a computer's permission was granted on a page")
	}
	export := api.ActionDescriptor{Name: "page.export", Version: 1, Title: "Export", Permission: "page.export"}
	run := func(key string) (*api.ActionJob, error) {
		return maker.RunAction(c, api.ActionInvocation{GrantID: g.ID, Action: "page.export", Version: 1, IdempotencyKey: key, Args: json.RawMessage(`{"format":"svg"}`)})
	}
	if _, err := run("k0"); err == nil {
		t.Fatal("an export ran with no renderer")
	}
	// A renderer of someone who cannot see the page does not count.
	lh, err := laohu.RegisterActionHost(c, api.ActionHostRegister{Provider: "page", Generation: "l1", Actions: []api.ActionDescriptor{export}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run("k1"); err == nil {
		t.Fatal("an export ran on a renderer whose person cannot see the page")
	}
	host, err := glenn.RegisterActionHost(c, api.ActionHostRegister{Provider: "page", Generation: "g1", Actions: []api.ActionDescriptor{export}})
	if err != nil || host.MachineID != "" {
		t.Fatalf("renderer: %+v %v", host, err)
	}
	job, err := run("k2")
	if err != nil {
		t.Fatal(err)
	}
	if jobs, err := laohu.ActionHostJobs(c, lh.ID, 0); err != nil || len(jobs) != 0 {
		t.Fatalf("the other person's renderer got the job: %+v %v", jobs, err)
	}
	jobs, err := glenn.ActionHostJobs(c, host.ID, 2*time.Second)
	if err != nil || len(jobs) != 1 || jobs[0].Job.ID != job.ID || jobs[0].Grant.Target.ID != page.ID {
		t.Fatalf("renderer jobs: %+v %v", jobs, err)
	}
	art, err := glenn.UploadActionArtifact(c, job.ID, host.ID, "g1", "image/svg+xml", []byte("<svg/>"))
	if err != nil {
		t.Fatal(err)
	}
	result, _ := json.Marshal(map[string]any{"media_type": "image/svg+xml", "artifact": art})
	if _, err := glenn.CompleteActionJob(c, job.ID, api.ActionJobComplete{HostID: host.ID, Generation: "g1", State: api.JobSucceeded, Result: result, ArtifactIDs: []string{art.ID}}); err != nil {
		t.Fatal(err)
	}
	if data, typ, err := maker.ActionArtifact(c, art.ID); err != nil || string(data) != "<svg/>" || typ != "image/svg+xml" {
		t.Fatalf("export: %q %s %v", data, typ, err)
	}
	if _, _, err := outsider.ActionArtifact(c, art.ID); err == nil {
		t.Fatal("another agent downloaded the export")
	}
	// The page's owner sees the grant and can revoke it.
	owned, err := glenn.OwnerActionGrants(c, api.GrantActive)
	if err != nil || len(owned) != 1 || owned[0].ID != g.ID {
		t.Fatalf("owner grants: %+v %v", owned, err)
	}
	if _, err := glenn.RevokeActionGrant(c, g.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := run("k3"); err == nil {
		t.Fatal("a revoked page grant ran an export")
	}
	if _, _, err := maker.ActionArtifact(c, art.ID); err == nil {
		t.Fatal("the export outlived its grant")
	}
}
