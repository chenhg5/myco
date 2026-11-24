package console

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/app"
	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/hubserver"
	"github.com/myco-ai/myco/internal/sshcap"
	"github.com/myco-ai/myco/internal/testsshd"
)

// A real Hub, human approval, trusted host, real OpenSSH client and isolated sshd.
// Linux reads use real /proc; service/container daemons are test-only fixtures so
// no test can start/stop a service on the developer's computer.
func TestSSHJobsRealTransportAndBoundApproval(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	fixture := t.TempDir()
	script := filepath.Join(fixture, "remote")
	marker := filepath.Join(fixture, "mutation")
	text := `#!/bin/sh
case "$SSH_ORIGINAL_COMMAND" in
 *"systemctl --no-ask-password restart"*) echo started > '` + marker + `'; sleep 20;;
 *"systemctl --no-pager --plain"*) printf 'fixture.service loaded active running Fixture service\n';;
 *"docker ps"*) printf '{"ID":"fixture","Names":"fixture","State":"running"}\n';;
 *"docker start"*) echo container-started > '` + marker + `'; echo fixture;;
 *"docker logs"*) i=0; while [ "$i" -lt 5000 ]; do printf 'long container log ...............................................................\n'; i=$((i+1)); done;;
 *"journalctl"*) i=0; while [ "$i" -lt 5000 ]; do printf '""""""""""""""""""""""""""""""""""""""""\n'; i=$((i+1)); done;;
 *) exec /bin/sh -c "$SSH_ORIGINAL_COMMAND";;
esac
`
	if err := os.WriteFile(script, []byte(text), 0700); err != nil {
		t.Fatal(err)
	}
	home, config := testsshd.StartWithCommand(t, script)
	m := &sshcap.Manager{Home: home, Config: config}
	p, err := m.Create(ctx, sshcap.Create{Name: "Fixture Linux", Destination: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	p, err = m.SetAgentFor(ctx, p.ID, true, "test-hub")
	if err != nil {
		t.Fatal(err)
	}
	h, err := hubserver.Build(hubserver.Options{DBPath: filepath.Join(t.TempDir(), "hub.db"), AdminToken: "admin-token-123456", Policy: app.DefaultMessagePolicy(), OnlineWindow: time.Minute, FilesDir: t.TempDir(), Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	hs := httptest.NewServer(h.Handler)
	defer func() { hs.Close(); h.Close() }()
	admin, _ := client.New(hs.URL, "admin-token-123456")
	ct, err := admin.CreateConsoleToken(ctx, "ssh-owner")
	if err != nil {
		t.Fatal(err)
	}
	owner, _ := client.New(hs.URL, ct.Token)
	reg := func(name, harness string) *client.Client {
		r, err := admin.Register(ctx, api.RegisterRequest{Name: name, Binding: api.Binding{Harness: harness, SessionID: name}}, false)
		if err != nil {
			t.Fatal(err)
		}
		c, _ := client.New(hs.URL, r.Token)
		return c
	}
	node := reg("ssh-machine", "machine")
	if err := node.ReportMachine(ctx, api.MachineReport{Fingerprint: "fp-ssh-0123456789abcdef", Name: "controller", OS: "linux"}); err != nil {
		t.Fatal(err)
	}
	machines, err := owner.Machines(ctx)
	if err != nil || len(machines) != 1 {
		t.Fatalf("machines %v %v", machines, err)
	}
	machine := machines[0].ID
	agent := reg("ssh-agent", "claude")
	other := reg("other-agent", "codex")
	host, err := owner.RegisterActionHost(ctx, api.ActionHostRegister{MachineID: machine, Resource: sshResource(p), Provider: "server.ssh", Generation: "g1", Actions: sshActionList})
	if err != nil {
		t.Fatal(err)
	}
	// Another workspace member cannot publish on this controller, even with
	// their own authenticated console. Private target discovery excludes them.
	inv, err := admin.InviteMember(ctx, api.InviteRequest{Name: "outsider"})
	if err != nil {
		t.Fatal(err)
	}
	anonymous, _ := client.New(hs.URL, "")
	paired, err := anonymous.Pair(ctx, inv.Link[strings.Index(inv.Link, "/c/")+3:], "outsider-device")
	if err != nil {
		t.Fatal(err)
	}
	stranger, _ := client.New(hs.URL, paired.Token)
	if _, err := stranger.RegisterActionHost(ctx, api.ActionHostRegister{MachineID: machine, Resource: sshResource(p), Provider: "server.ssh", Generation: "steal", Actions: sshActionList}); err == nil {
		t.Fatal("non-owner published another person's SSH target")
	}
	en, err := stranger.CreateEnrollment(ctx, api.CreateEnrollmentRequest{NamePrefix: "outside-", MaxUses: 1}, false)
	if err != nil {
		t.Fatal(err)
	}
	enrolling, _ := client.New(hs.URL, en.Token)
	enrolled, err := enrolling.Enroll(ctx, api.EnrollRequest{Name: "outside-agent", Binding: api.Binding{Harness: "codex", SessionID: "outside"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	outside, _ := client.New(hs.URL, enrolled.Token)
	if ts, err := outside.ActionTargets(ctx); err != nil || len(ts) != 0 {
		t.Fatal("private resource leaked to another person")
	}
	targets, err := agent.ActionTargets(ctx)
	if err != nil || len(targets) != 1 || targets[0].Resource.Binding != p.Binding {
		t.Fatalf("targets %v %v", targets, err)
	}
	req := api.ActionGrantRequest{Target: targets[0], Permissions: []string{"server.ssh.read"}, Reason: "inspect the Linux fixture"}
	// Agent-supplied labels cannot disguise the actual endpoint in approval.
	req.Target.Resource.Name = "fake benign target"
	req.Target.Resource.Summary = "localhost"
	g, err := agent.RequestActionGrant(ctx, req, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(g.TargetName, p.Summary()) || strings.Contains(g.TargetName, "fake benign") {
		t.Fatalf("untrusted approval label: %s", g.TargetName)
	}
	aps, err := owner.Approvals(ctx, "pending")
	if err != nil || len(aps) != 1 {
		t.Fatal(err)
	}
	if _, err := agent.RunAction(ctx, api.ActionInvocation{GrantID: g.ID, Action: "server.ssh.overview", Version: 1, IdempotencyKey: "pending"}); err == nil {
		t.Fatal("pending grant executed")
	}
	if _, err := owner.DecideApproval(ctx, aps[0].ID, true); err != nil {
		t.Fatal(err)
	}
	g, err = agent.ActionGrant(ctx, g.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	run := func(grant, action, args string) *api.ActionJob {
		t.Helper()
		j, err := agent.RunAction(ctx, api.ActionInvocation{GrantID: grant, Action: action, Version: 1, IdempotencyKey: action + args, Args: json.RawMessage(args)})
		if err != nil {
			t.Fatal(err)
		}
		jobs, err := owner.ActionHostJobs(ctx, host.ID, time.Second)
		if err != nil || len(jobs) != 1 {
			t.Fatalf("jobs %v %v", jobs, err)
		}
		done := runSSHJob(ctx, owner, host, jobs[0], m, "test-hub")
		done.HostID, done.Generation = host.ID, host.Generation
		if _, err := owner.CompleteActionJob(ctx, j.ID, done); err != nil {
			t.Fatal(err)
		}
		j, err = agent.ActionJob(ctx, j.ID, 0)
		if err != nil {
			t.Fatal(err)
		}
		return j
	}
	j := run(g.ID, "server.ssh.overview", `{}`)
	if j.State != api.JobSucceeded || !strings.Contains(string(j.Result), "MemTotal:") {
		t.Fatalf("real overview %+v", j)
	}
	for _, a := range []string{"services.list", "containers.list"} {
		j := run(g.ID, "server.ssh."+a, `{}`)
		if j.State != api.JobSucceeded {
			t.Fatalf("%s %+v", a, j)
		}
	}
	j = run(g.ID, "server.ssh.services.logs", `{"name":"fixture.service","lines":200}`)
	if j.State != api.JobSucceeded {
		t.Fatalf("logs state=%s error=%+v", j.State, j.Error)
	}
	if len(j.Artifacts) != 1 {
		t.Fatalf("large private result not artifact: state=%s resultBytes=%d artifacts=%d", j.State, len(j.Result), len(j.Artifacts))
	}
	data, _, err := agent.ActionArtifact(ctx, j.Artifacts[0].ID)
	if err != nil || !strings.Contains(string(data), `"truncated":true`) {
		t.Fatalf("bounded artifact %v", err)
	}
	if _, _, err := other.ActionArtifact(ctx, j.Artifacts[0].ID); err == nil {
		t.Fatal("other agent read private result")
	}
	if _, err := agent.RunAction(ctx, api.ActionInvocation{GrantID: g.ID, Action: "server.ssh.containers.start", Version: 1, IdempotencyKey: "readonly", Args: json.RawMessage(`{"name":"fixture"}`)}); err == nil {
		t.Fatal("read grant mutated")
	}
	j = run(g.ID, "server.ssh.overview", `{"profile_id":"other-server"}`)
	if j.Error == nil || j.Error.Code != "invalid_argument" {
		t.Fatalf("args target override %+v", j)
	}
	j = run(g.ID, "server.ssh.containers.logs", `{"name":"fixture","lines":200}`)
	if j.State != api.JobSucceeded || len(j.Result) < 128<<10 {
		t.Fatalf("large inline result state=%s bytes=%d error=%+v", j.State, len(j.Result), j.Error)
	}
	// A different profile on the same controller cannot take this job.
	p2, err := m.Create(ctx, sshcap.Create{Name: "Other profile", Destination: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	host2, err := owner.RegisterActionHost(ctx, api.ActionHostRegister{MachineID: machine, Resource: sshResource(p2), Provider: "server.ssh", Generation: "g2", Actions: sshActionList})
	if err != nil {
		t.Fatal(err)
	}
	j, err = agent.RunAction(ctx, api.ActionInvocation{GrantID: g.ID, Action: "server.ssh.overview", Version: 1, IdempotencyKey: "bound"})
	if err != nil {
		t.Fatal(err)
	}
	if jobs, err := owner.ActionHostJobs(ctx, host2.ID, 0); err != nil || len(jobs) != 0 {
		t.Fatal("another profile took the job")
	}
	jobs, err := owner.ActionHostJobs(ctx, host.ID, time.Second)
	if err != nil || len(jobs) != 1 {
		t.Fatal(err)
	}
	copied := jobs[0]
	copied.Grant.Target.Resource = sshResource(p2)
	if d := runSSHJob(ctx, owner, host, copied, m, "test-hub"); d.Error == nil || d.Error.Code != "permission_denied" {
		t.Fatalf("host accepted different binding %+v", d)
	}
	// Enable manage separately and observe cancellation while a mutation is running.
	req.Target = targets[0]
	req.Permissions = []string{"server.ssh.manage"}
	req.Reason = "exercise revocation"
	mg, err := agent.RequestActionGrant(ctx, req, false)
	if err != nil {
		t.Fatal(err)
	}
	aps, _ = owner.Approvals(ctx, "pending")
	for _, ap := range aps {
		if _, err := owner.DecideApproval(ctx, ap.ID, true); err != nil {
			t.Fatal(err)
		}
	}
	j = run(mg.ID, "server.ssh.containers.start", `{"name":"fixture"}`)
	if j.State != api.JobSucceeded {
		t.Fatalf("manage %+v", j)
	}
	j, err = agent.RunAction(ctx, api.ActionInvocation{GrantID: mg.ID, Action: "server.ssh.services.restart", Version: 1, IdempotencyKey: "cancel", Args: json.RawMessage(`{"name":"fixture.service"}`)})
	if err != nil {
		t.Fatal(err)
	}
	jobs, err = owner.ActionHostJobs(ctx, host.ID, time.Second)
	if err != nil || len(jobs) != 1 {
		t.Fatal(err)
	}
	done := make(chan api.ActionJobComplete, 1)
	go func() { done <- runSSHJob(ctx, owner, host, jobs[0], m, "test-hub") }()
	for end := time.Now().Add(5 * time.Second); ; {
		b, _ := os.ReadFile(marker)
		if strings.TrimSpace(string(b)) == "started" {
			break
		}
		if time.Now().After(end) {
			t.Fatal("mutation never started")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := owner.RevokeActionGrant(ctx, mg.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case d := <-done:
		if d.Error == nil || d.Error.Code != "outcome_unknown" {
			t.Fatalf("cancel lost outcome uncertainty %+v", d)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("SSH mutation continued after revoke")
	}
	// Switching workspace disables old-workspace runs immediately.
	if _, err := m.SetAgentFor(ctx, p.ID, true, "other-hub"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.RunFor(ctx, p.ID, p.Binding, sshcap.Operation{Action: "overview"}, true, "test-hub"); err == nil {
		t.Fatal("agent crossed workspace")
	}
	// A changed binding is never substituted into an already approved grant.
	changed := *sshResource(p)
	changed.Binding = strings.Repeat("a", 64)
	if _, err := owner.RegisterActionHost(ctx, api.ActionHostRegister{MachineID: machine, Resource: &changed, Provider: "server.ssh", Generation: "changed", Actions: sshActionList}); err != nil {
		t.Fatal(err)
	}
	if old, err := agent.ActionGrant(ctx, g.ID, 0); err != nil || old.State != api.GrantRevoked {
		t.Fatalf("changed target kept grant %+v %v", old, err)
	}
}

func TestCancelledSSHHostCannotRemoveReplacement(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &actionHosts{epochs: map[string]uint64{"ssh:profile": 2}, running: map[string]context.CancelFunc{"ssh:profile": cancel}, retry: map[string]time.Time{}}
	a.finish("ssh:profile", 1, time.Minute)
	if a.running["ssh:profile"] == nil || !a.retry["ssh:profile"].IsZero() || ctx.Err() != nil {
		t.Fatal("cancelled host removed/backed off its replacement")
	}
	a.finish("ssh:profile", 2, 0)
	if a.running["ssh:profile"] != nil {
		t.Fatal("current host did not finish")
	}
}
