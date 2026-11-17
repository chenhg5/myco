package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/chenhg5/myco/internal/config"
	"github.com/chenhg5/myco/internal/domain"
)

// EnvAgent names the agent profile (~/.myco/agents/<name>.yaml) to act as
// when a command runs outside that agent's session.
const EnvAgent = "MYCO_AGENT"

// agentProfiles are the profiles on this machine that are agents someone
// could mean: not the machine node (one per machine, not an agent of its
// own), not one another session took over, and with a token.
func agentProfiles(defaultPath string) []config.Profile {
	ps, _ := config.ListProfiles(config.ProfilesDir(defaultPath))
	var out []config.Profile
	for _, p := range ps {
		c := p.Config
		if c.Token == "" || c.Replaced != "" || c.Harness == domain.HarnessMachine {
			continue
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Config.Agent < out[j].Config.Agent })
	return out
}

// adoptProfile picks the agent to act as when nothing chose one: the
// command runs outside the agent's session (a terminal, another agent's
// tool call), no --config / MYCO_CONFIG, and ~/.myco/config.yaml holds no
// identity. MYCO_AGENT=<name> picks one; otherwise the only agent profile
// on this machine is used. It returns the names when several could be
// meant (nothing is picked then).
func (e *env) adoptProfile() (several []string, err error) {
	if e.explicitConfig || e.cfgPath != e.defaultPath || e.cfg.Token != "" {
		return nil, nil
	}
	ps := agentProfiles(e.defaultPath)
	var pick *config.Profile
	if name := strings.TrimSpace(e.in.Getenv(EnvAgent)); name != "" {
		for i := range ps {
			if ps[i].Config.Agent == name {
				pick = &ps[i]
				break
			}
		}
		if pick == nil {
			return nil, errorf("not_found", ExitUsage, "agents on this computer: "+namesOrNone(ps),
				"%s=%s: no agent profile with that name in %s", EnvAgent, name, config.ProfilesDir(e.defaultPath))
		}
	} else if len(ps) == 1 {
		pick = &ps[0]
	} else if len(ps) > 1 {
		names := make([]string, len(ps))
		for i, p := range ps {
			names[i] = p.Config.Agent
		}
		return names, nil
	}
	if pick == nil {
		return nil, nil
	}
	cfg, err := config.Load(pick.Path)
	if err != nil {
		return nil, err
	}
	cfg.ApplyEnv(e.in.Getenv)
	e.cfgPath, e.cfg = pick.Path, cfg
	return nil, nil
}

func namesOrNone(ps []config.Profile) string {
	if len(ps) == 0 {
		return "none"
	}
	names := make([]string, len(ps))
	for i, p := range ps {
		names[i] = p.Config.Agent
	}
	return strings.Join(names, ", ")
}

// severalAgentsError says which agents could be meant and how to pick one.
func severalAgentsError(names []string) error {
	return errorf("not_configured", ExitUsage,
		fmt.Sprintf("pick one: %s=<name> myco … (one of: %s), or --config ~/.myco/agents/<name>.yaml", EnvAgent, strings.Join(names, ", ")),
		"several agents on this computer and none chosen for this command")
}
