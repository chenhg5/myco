package console

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/sshcap"
)

var sshActionList = func() []api.ActionDescriptor {
	out := []api.ActionDescriptor{}
	for _, action := range []string{"overview", "services.list", "services.status", "services.logs", "services.start", "services.stop", "services.restart", "containers.list", "containers.logs", "containers.start", "containers.stop", "containers.restart"} {
		perm := "server.ssh.read"
		for _, suffix := range []string{".start", ".stop", ".restart"} {
			if strings.HasSuffix(action, suffix) {
				perm = "server.ssh.manage"
			}
		}
		out = append(out, api.ActionDescriptor{Name: "server.ssh." + action, Version: 1, Title: action, Permission: perm,
			InputSchema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string","maxLength":160},"lines":{"type":"integer","minimum":1,"maximum":200}},"additionalProperties":false}`)})
	}
	return out
}()

// SSH hosts are enabled per private profile AND workspace. No shared page data
// is consulted here; only the person's local native opt-in can publish a host.
func (b *Backend) sshProfiles() (*sshcap.Manager, []sshcap.Profile) {
	env, _ := b.osInit()
	m, err := env.SSHManager()
	if err != nil {
		return nil, nil
	}
	ps, err := m.List()
	if err != nil {
		return m, nil
	}
	return m, ps
}
func sshResource(p sshcap.Profile) *api.ActionResource {
	return &api.ActionResource{Provider: "server.ssh", ID: p.ID, Binding: p.Binding, Name: p.Name, Summary: p.Summary()}
}
func (b *Backend) hostSSH(ctx context.Context, h Hub, p sshcap.Profile) time.Duration {
	m, _ := b.sshProfiles()
	if m == nil {
		return 5 * time.Minute
	}
	c, err := b.client(h)
	if err != nil {
		return 5 * time.Minute
	}
	call, cancel := context.WithTimeout(ctx, 15*time.Second)
	machine, err := b.thisMachine(call, c)
	cancel()
	if err != nil {
		return 2 * time.Minute
	}
	call, cancel = context.WithTimeout(ctx, 15*time.Second)
	host, err := c.RegisterActionHost(call, api.ActionHostRegister{MachineID: machine, Provider: "server.ssh", Resource: sshResource(p), Generation: "ssh-" + rand.Text(), Actions: sshActionList})
	cancel()
	if err != nil {
		return 5 * time.Minute
	} // old hubs fail closed: no unscoped fallback
	for ctx.Err() == nil {
		current, err := m.Get(p.ID)
		if err != nil || !current.AgentEnabled || current.AgentHub != h.ID || current.Binding != p.Binding {
			break
		}
		jobs, err := c.ActionHostJobs(ctx, host.ID, actionHostPoll)
		if err != nil {
			return actionHostRetake
		}
		for _, j := range jobs {
			done := runSSHJob(ctx, c, host, j, m, h.ID)
			done.HostID, done.Generation = host.ID, host.Generation
			end, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
			_, _ = c.CompleteActionJob(end, j.Job.ID, done)
			cancel()
		}
	}
	return 0
}
func runSSHJob(ctx context.Context, c *client.Client, host *api.ActionHost, hj api.ActionHostJob, m *sshcap.Manager, hub string) api.ActionJobComplete {
	var perm string
	for _, d := range sshActionList {
		if d.Name == hj.Job.Action && d.Version == hj.Job.Version {
			perm = d.Permission
		}
	}
	if perm == "" {
		return actionFailure("unsupported", "this SSH host does not offer that action version", false, nil)
	}
	g := hj.Grant
	r := g.Target.Resource
	if g.State != api.GrantActive || g.Target.Kind != "machine" || g.Target.ID != host.MachineID || g.ExpiresAt == nil || !time.Now().Before(*g.ExpiresAt) || r == nil || host.Resource == nil || r.Provider != "server.ssh" || *r != *host.Resource || !(slices.Contains(g.Permissions, perm) || perm == "server.ssh.read" && slices.Contains(g.Permissions, "server.ssh.manage")) {
		return actionFailure("permission_denied", "the grant does not allow this operation on this bound SSH target", false, nil)
	}
	var args struct {
		Name  string `json:"name"`
		Lines int    `json:"lines"`
	}
	raw := hj.Args
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&args) != nil || d.Decode(new(any)) != io.EOF {
		return actionFailure("invalid_argument", "SSH arguments only accept name and bounded lines; the target is fixed by the grant", false, nil)
	}
	op := sshcap.Operation{Action: strings.TrimPrefix(hj.Job.Action, "server.ssh."), Name: args.Name, Lines: args.Lines}
	_, _, err := sshcap.Command(op)
	if err != nil {
		return sshJobError(err)
	}
	run, stop, _, early := watchHostJob(ctx, c, host, hj.Job.ID)
	if early != nil {
		return *early
	}
	defer stop()
	result, err := m.RunFor(run, r.ID, r.Binding, op, true, hub)
	if err != nil {
		return sshJobError(err)
	}
	raw, err = json.Marshal(result)
	if err != nil {
		return actionFailure("failed", "could not encode the SSH result", false, nil)
	}
	if len(raw) > 200<<10 {
		art, err := c.UploadActionArtifact(run, hj.Job.ID, host.ID, host.Generation, "application/json", raw)
		if err != nil {
			return actionFailure("unavailable", "could not upload the private SSH result; check status before retrying a mutation", false, nil)
		}
		summary, _ := json.Marshal(map[string]any{"artifact": art, "exit_code": result.ExitCode, "truncated": result.Truncated, "duration_ms": result.DurationMS})
		return api.ActionJobComplete{State: api.JobSucceeded, Result: summary, ArtifactIDs: []string{art.ID}}
	}
	return api.ActionJobComplete{State: api.JobSucceeded, Result: raw}
}
func sshJobError(err error) api.ActionJobComplete {
	var e *sshcap.Error
	if errors.As(err, &e) {
		state := api.JobFailed
		if e.Code == "cancelled" {
			state = api.JobCancelled
		}
		return api.ActionJobComplete{State: state, Error: &api.ActionError{Code: e.Code, Message: e.Message}}
	}
	return actionFailure("failed", "the local SSH operation could not run", false, nil)
}
