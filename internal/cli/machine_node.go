package cli

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/config"
	"github.com/chenhg5/myco/internal/domain"
	"github.com/chenhg5/myco/internal/harness"
	"github.com/chenhg5/myco/internal/harness/builtin"
	"github.com/chenhg5/myco/internal/harness/execnode"
	"github.com/chenhg5/myco/internal/harness/machinenode"
)

// spawnBinary is the executable each spawnable CLI needs on PATH.
func spawnBinary(cli string) string {
	if p, ok := builtin.Presets[cli]; ok {
		return p.Binary
	}
	return cli // claude, codex
}

// machineNodeApplies reports whether this machine should have a machine
// node: it joined with a normal link, is not a claude.ai container, and its
// owner has not disabled spawning.
func (e *env) machineNodeApplies() (*config.Machine, bool, error) {
	m, err := config.LoadMachine(e.defaultPath)
	if err != nil || m == nil {
		return nil, false, err
	}
	if m.Harness == execnode.Name || m.Spawn.Disabled || e.in.Getenv("CLAUDE_CODE_REMOTE_SESSION_ID") != "" {
		return m, false, nil
	}
	return m, true, nil
}

// ensureMachineNode enrolls this machine's machine node (idempotent: the same
// machine gets the same node back) and writes its runner profile.
func (e *env) ensureMachineNode() (*api.Agent, error) {
	m, ok, err := e.machineNodeApplies()
	if err != nil || !ok {
		return nil, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	host, user := hostname(), orDefault(e.in.Getenv("USER"), "user")
	b := api.Binding{Harness: machinenode.Name, Environment: host, Workdir: home,
		SessionID: "machine:" + host + ":" + user}
	c, err := client.New(m.Hub, m.EnrollToken)
	if err != nil {
		return nil, err
	}
	res, err := c.Enroll(e.in.Ctx, api.EnrollRequest{Name: domain.MachineNodeName(m.NamePrefix), Binding: b,
		DisplayName: "machine " + m.Machine,
		Description: "machine node of " + m.Machine + ": starts new agent sessions here (myco agent spawn)"}, false)
	if err != nil {
		return nil, err
	}
	if err := saveProfile(e.defaultPath, m.Hub, res.Agent.Name, res.Token, b, config.Runner{Harness: machinenode.Name, Workdir: home}); err != nil {
		return nil, err
	}
	return &res.Agent, nil
}

// ensureMachineNodeLogged is ensureMachineNode for `runner --all`: it reports
// done (true) once the node exists or does not apply, and retries otherwise.
func (e *env) ensureMachineNodeLogged(log *slog.Logger) bool {
	a, err := e.ensureMachineNode()
	if err != nil {
		var ce *client.Error
		if errors.As(err, &ce) && (ce.Code == "permission_denied" || ce.Code == "invalid_argument") {
			// An older hub, or credentials that may not create one: don't retry.
			log.Warn("machine node unavailable; myco agent spawn will not reach this machine", "err", err)
			return true
		}
		log.Warn("could not set up the machine node yet; retrying", "err", err)
		return false
	}
	if a != nil && len(a.AllowFrom) == 0 {
		log.Warn("machine node accepts spawn requests from any agent on the hub; restrict it with: myco agent update --name "+a.Name+" --allow-from <agent>[,<agent>...]",
			"agent", a.Name)
	}
	return true
}

// saveProfile writes an agent's runner profile (~/.myco/agents/<name>.yaml),
// keeping runner settings already there. It does not touch the env's current
// config, so it is safe to call while runner --all serves other agents.
func saveProfile(defaultPath, hub, agent, token string, b api.Binding, rc config.Runner) error {
	path := config.ProfilePath(defaultPath, agent)
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	cfg.Hub, cfg.Agent, cfg.Token, cfg.Harness, cfg.SessionID = hub, agent, token, b.Harness, b.SessionID
	if cfg.Runner.Harness == "" {
		cfg.Runner = rc
	}
	return config.Save(path, cfg)
}

// machineNode builds the harness that serves this machine's machine node.
func (e *env) machineNode(me *api.Agent, c *client.Client, sh runnerShared) (harness.Harness, error) {
	m, err := config.LoadMachine(e.defaultPath)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, errorf("not_configured", ExitUsage, "join this machine with a link first: curl -fsSL <link> | bash",
			"machine node %s needs this machine's credentials (~/.myco/machine.yaml)", me.Name)
	}
	if m.Spawn.Disabled {
		return nil, errorf("permission_denied", ExitPermission, "set spawn.disabled: false in ~/.myco/machine.yaml to allow it",
			"spawning is disabled on this machine")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	maxAccess, err := harness.ParseAccess(m.Spawn.MaxAccess)
	if err != nil {
		return nil, errorf("invalid_config", ExitUsage, "spawn.max_access must be readonly, workspace or full", "~/.myco/machine.yaml: %v", err)
	}
	host := hostname()
	return &machinenode.Node{
		Machine: m.Machine, Prefix: m.NamePrefix,
		Policy:  machinenode.Policy{Home: home, Dirs: m.Spawn.Dirs, MaxAccess: maxAccess},
		Senders: me.AllowFrom, CurrentSenders: cachedAllowFrom(e.in.Ctx, c, 30*time.Second),
		Deps: machinenode.Deps{
			NewHarness: func(cli string, opts harness.Options) (harness.Harness, error) { return sh.registry.New(cli, opts) },
			Installed: func(cli string) bool {
				_, err := exec.LookPath(spawnBinary(cli))
				return err == nil
			},
			Enroll: func(ctx context.Context, p machinenode.Plan, sr api.SpawnRequest, sessionID string) (string, error) {
				mc, err := client.New(m.Hub, m.EnrollToken)
				if err != nil {
					return "", err
				}
				b := api.Binding{Harness: p.CLI, SessionID: sessionID, Environment: host, Workdir: p.Workdir}
				res, err := mc.Enroll(ctx, api.EnrollRequest{Name: p.Name, DisplayName: sr.DisplayName, Description: sr.Description,
					Labels: sr.Labels, Binding: b}, false)
				if err != nil {
					return "", err
				}
				rc := config.Runner{Harness: p.CLI, Workdir: p.Workdir, Access: string(p.Access), Model: sr.Model}
				return res.Agent.Name, saveProfile(e.defaultPath, m.Hub, res.Agent.Name, res.Token, b, rc)
			},
			Spawned: func() {
				if sh.rescan != nil {
					select {
					case sh.rescan <- struct{}{}:
					default:
					}
				}
			},
		},
	}, nil
}
