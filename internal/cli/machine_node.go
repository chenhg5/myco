package cli

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/config"
	"github.com/chenhg5/myco/internal/domain"
	"github.com/chenhg5/myco/internal/harness"
	"github.com/chenhg5/myco/internal/harness/builtin"
	"github.com/chenhg5/myco/internal/harness/execnode"
	"github.com/chenhg5/myco/internal/harness/machinenode"
	"github.com/chenhg5/myco/internal/osutil"
)

// spawnBinary is the executable each spawnable CLI needs on PATH.
func spawnBinary(cli string) string {
	if b := builtin.Binary(cli); b != "" {
		return b
	}
	return cli // claude, codex
}

// machineNodeHubs returns this machine's memberships that should have a
// machine node: joined with a normal link (not as an exec node), not a
// claude.ai container, and spawning not disabled by the machine's owner.
func (e *env) machineNodeHubs() (*config.Machine, []config.MachineHub, error) {
	m, err := config.LoadMachine(e.defaultPath)
	if err != nil || m == nil {
		return nil, nil, err
	}
	if m.Spawn.Disabled || e.in.Getenv("CLAUDE_CODE_REMOTE_SESSION_ID") != "" {
		return m, nil, nil
	}
	var out []config.MachineHub
	for _, h := range m.Hubs() {
		if h.Harness != execnode.Name {
			out = append(out, h)
		}
	}
	return m, out, nil
}

// ensureMachineNode enrolls this machine's machine node on hub h
// (idempotent: the same machine gets the same node back) and writes its
// runner profile.
func (e *env) ensureMachineNode(h config.MachineHub) (*api.Agent, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	host, user := hostname(), orDefault(osutil.UserName(e.in.Getenv), "user")
	b := api.Binding{Harness: machinenode.Name, Environment: host, Workdir: home,
		SessionID: "machine:" + host + ":" + user}
	c, err := client.New(h.Hub, h.EnrollToken)
	if err != nil {
		return nil, err
	}
	res, err := c.Enroll(e.in.Ctx, api.EnrollRequest{Name: domain.MachineNodeName(h.NamePrefix), Binding: b,
		DisplayName: "machine " + h.Machine,
		Description: "machine node of " + h.Machine + ": starts new agent sessions here (myco agent spawn)"}, false)
	if err != nil {
		return nil, err
	}
	if err := saveProfile(e.defaultPath, h.Hub, res.Agent.Name, res.Token, b, config.Runner{Harness: machinenode.Name, Workdir: home}); err != nil {
		return nil, err
	}
	return &res.Agent, nil
}

// ensureMachineNodesLogged is ensureMachineNode for `runner --all`, on every
// hub this machine joined: it reports done (true) once each node exists or
// does not apply, and retries otherwise.
func (e *env) ensureMachineNodesLogged(log *slog.Logger, done map[string]bool) bool {
	_, hubs, err := e.machineNodeHubs()
	if err != nil {
		log.Warn("reading machine.yaml failed", "err", err)
		return false
	}
	all := true
	for _, h := range hubs {
		if done[h.Hub] {
			continue
		}
		a, err := e.ensureMachineNode(h)
		if err != nil {
			var ce *client.Error
			if errors.As(err, &ce) && (ce.Code == "permission_denied" || ce.Code == "invalid_argument") {
				// An older hub, or credentials that may not create one: don't retry.
				log.Warn("machine node unavailable; myco agent spawn will not reach this machine", "hub", h.Hub, "err", err)
				done[h.Hub] = true
				continue
			}
			log.Warn("could not set up the machine node yet; retrying", "hub", h.Hub, "err", err)
			all = false
			continue
		}
		done[h.Hub] = true
		if len(a.AllowFrom) == 0 {
			log.Warn("machine node accepts spawn requests from any agent on the hub; restrict it with: myco agent update --name "+a.Name+" --allow-from <agent>[,<agent>...]",
				"agent", a.Name, "hub", h.Hub)
		}
	}
	return all
}

// saveProfile writes an agent's runner profile (~/.myco/agents/<name>.yaml),
// keeping runner settings already there. It does not touch the env's current
// config, so it is safe to call while runner --all serves other agents.
func saveProfile(defaultPath, hub, agent, token string, b api.Binding, rc config.Runner) error {
	path := profilePathFor(defaultPath, hub, agent)
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

// profilePathFor is agent's profile on hub: <agent>.yaml, or, when that file
// belongs to an agent of the same name on another hub, <agent>@<hub host>.yaml.
func profilePathFor(defaultPath, hub, agent string) string {
	path := config.ProfilePath(defaultPath, agent)
	if c, err := config.Load(path); err == nil && c.Hub != "" && !config.SameHub(c.Hub, hub) {
		return config.ProfilePath(defaultPath, agent+"@"+hubSlug(hub))
	}
	return path
}

// hubSlug is a hub URL as a file-name-safe word (its host and port).
func hubSlug(hub string) string {
	u, err := url.Parse(hub)
	h := hub
	if err == nil && u.Host != "" {
		h = u.Host
	}
	return strings.NewReplacer(":", "-", "/", "-").Replace(strings.ToLower(h))
}

// machineNode builds the harness that serves this machine's machine node.
func (e *env) machineNode(hub string, me *api.Agent, c *client.Client, sh runnerShared) (harness.Harness, error) {
	mf, err := config.LoadMachine(e.defaultPath)
	if err != nil {
		return nil, err
	}
	var m *config.MachineHub
	if mf != nil {
		if h, ok := mf.For(hub); ok {
			m = &h
		}
	}
	if m == nil {
		return nil, errorf("not_configured", ExitUsage, "join this machine with a link first: curl -fsSL <link> | bash",
			"machine node %s needs this machine's credentials (~/.myco/machine.yaml)", me.Name)
	}
	if mf.Spawn.Disabled {
		return nil, errorf("permission_denied", ExitPermission, "set spawn.disabled: false in ~/.myco/machine.yaml to allow it",
			"spawning is disabled on this machine")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	maxAccess, err := harness.ParseAccess(mf.Spawn.MaxAccess)
	if err != nil {
		return nil, errorf("invalid_config", ExitUsage, "spawn.max_access must be readonly, workspace or full", "~/.myco/machine.yaml: %v", err)
	}
	host := hostname()
	return &machinenode.Node{
		Machine: m.Machine, Prefix: m.NamePrefix,
		Policy: machinenode.Policy{Home: home, Dirs: mf.Spawn.Dirs, MaxAccess: maxAccess},
		LoadPolicy: func() (machinenode.Policy, error) {
			mf, err := config.LoadMachine(e.defaultPath)
			if err != nil || mf == nil {
				return machinenode.Policy{}, errors.New("cannot read machine.yaml")
			}
			a, err := harness.ParseAccess(mf.Spawn.MaxAccess)
			if err != nil {
				return machinenode.Policy{}, err
			}
			return machinenode.Policy{Home: home, Dirs: mf.Spawn.Dirs, MaxAccess: a}, nil
		},
		Senders: me.AllowFrom, CurrentSenders: cachedAllowFrom(e.in.Ctx, c, 30*time.Second),
		Deps: machinenode.Deps{
			NewHarness: func(cli string, opts harness.Options) (harness.Harness, error) { return sh.registry.New(cli, opts) },
			Installed: func(cli string) bool {
				_, err := exec.LookPath(spawnBinary(cli))
				return err == nil
			},
			Enroll: func(ctx context.Context, p machinenode.Plan, sr api.SpawnRequest, sessionID string) (string, error) {
				token := m.EnrollToken
				b := api.Binding{Harness: p.CLI, SessionID: sessionID, Environment: host, Workdir: p.Workdir}
				if p.CLI == machinenode.ExecCLI {
					// A shell node joins with the one-use exec token the hub
					// made for this request; it carries the allowed senders.
					token = sr.EnrollToken
					b.SessionID = "exec:" + host + ":" + orDefault(osutil.UserName(e.in.Getenv), "user") + ":" + p.Name
				}
				mc, err := client.New(m.Hub, token)
				if err != nil {
					return "", err
				}
				res, err := mc.Enroll(ctx, api.EnrollRequest{Name: p.Name, DisplayName: sr.DisplayName, Description: sr.Description,
					Labels: sr.Labels, Binding: b}, false)
				if err != nil {
					return "", err
				}
				// The first turn ran with the machine node's env_file (provider
				// settings such as ANTHROPIC_BASE_URL / ANTHROPIC_API_KEY, captured
				// with `myco runner env capture --agent <machine node>`); later
				// turns need the same, or the session cannot log in.
				rc := config.Runner{Harness: p.CLI, Workdir: p.Workdir, Access: string(p.Access), Model: sr.Model,
					EnvFile: machineEnvFile(e.defaultPath, me.Name)}
				if p.CLI == machinenode.ExecCLI {
					rc = config.Runner{Harness: p.CLI, Workdir: p.Workdir}
				}
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

// machineEnvFile is the machine node's runner.env_file, which spawned
// sessions inherit ("" if it has none).
func machineEnvFile(defaultPath, machineNode string) string {
	cfg, err := config.Load(config.ProfilePath(defaultPath, machineNode))
	if err != nil {
		return ""
	}
	return cfg.Runner.EnvFile
}
