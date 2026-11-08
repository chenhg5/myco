package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	. "github.com/chenhg5/myco/internal/cli/cmdkit"
	"github.com/chenhg5/myco/internal/config"
	"github.com/chenhg5/myco/internal/provider"
)

// agentProfilePath is the profile of --agent, or of the current session.
func (e *env) agentProfilePath() (string, error) {
	path := e.cfgPath
	if n := e.in.String("agent"); n != "" {
		path = config.ProfilePath(e.defaultPath, n)
		if _, err := os.Stat(path); err != nil {
			return "", errorf("not_found", ExitNotFound, "list profiles: ls "+config.ProfilesDir(e.defaultPath),
				"no profile for agent %s on this machine (%s)", n, path)
		}
	} else if filepath.Dir(path) != config.ProfilesDir(e.defaultPath) {
		return "", &Error{Code: "invalid_usage", Message: "not inside an enrolled session; say which agent",
			Suggestion: "pass --agent <name>", Exit: ExitUsage}
	}
	return path, nil
}

func runnerProviderCommands() *Command {
	list := &Command{
		Name:    "list",
		Summary: "List the model providers an agent can be pinned to (from cc-switch)",
		Long: `Reads cc-switch's providers (~/.cc-switch) read-only. Keys are never printed. Pin one to an agent
with: myco runner provider set --agent <name> --provider cc-switch:<id>`,
		Examples: []string{"myco runner provider list", "myco runner provider list --format json"},
		Run: handler(func(e *env) error {
			ps, err := provider.List(provider.Dir(), "claude")
			if err != nil {
				return errorf("not_found", ExitNotFound, "install cc-switch, or put provider variables in the agent's env file: myco runner env capture",
					"%v", err)
			}
			return e.p.Print(map[string]any{"providers": ps}, func(w io.Writer) {
				if len(ps) == 0 {
					fmt.Fprintln(w, "cc-switch has no Claude providers")
				}
				for _, p := range ps {
					cur := ""
					if p.Current {
						cur = "  (current on this machine)"
					}
					host := p.Host
					if host == "" {
						host = "default endpoint"
					}
					fmt.Fprintf(w, "%-28s %s  %s%s\n", p.Ref, p.Name, host, cur)
				}
			})
		}),
	}
	set := &Command{
		Name:     "set",
		Summary:  "Pin an agent's model provider, or let it follow the machine again",
		Mutating: true,
		Long: `--provider cc-switch:<id> pins a cc-switch provider: it is read again before every run, so key
changes made in cc-switch apply, and nothing secret is copied. --provider machine removes the pin
(the CLI then uses whatever this machine is set to). Applies from the agent's next message; no
restart needed. Claude Code agents only, for now.`,
		Examples: []string{
			"myco runner provider set --agent mac-claude-docs --provider cc-switch:deepseek",
			"myco runner provider set --agent mac-claude-docs --provider machine",
		},
		Flags: []Flag{
			{Name: "agent", Kind: String, Help: "Agent whose profile to update (default: the current session's identity)"},
			{Name: "provider", Kind: String, Help: "cc-switch:<id> (see: myco runner provider list), or machine"},
		},
		Run: handler(func(e *env) error {
			path, err := e.agentProfilePath()
			if err != nil {
				return err
			}
			cfg, err := config.Load(path)
			if err != nil {
				return err
			}
			ref := strings.TrimSpace(e.in.String("provider"))
			out := map[string]any{"agent": cfg.Agent, "profile": path}
			switch {
			case ref == "":
				return &Error{Code: "invalid_usage", Message: "--provider is required", Suggestion: "myco runner provider list", Exit: ExitUsage}
			case ref == "machine" || ref == "none":
				ref = ""
				out["provider"] = "machine"
			default:
				if h := profileHarness(cfg); h != "claude" {
					return errorf("invalid_argument", ExitUsage, "", "pinned providers are supported for Claude Code agents only (this agent runs %s)", h)
				}
				p, err := provider.Resolve(provider.Dir(), ref, "claude")
				if err != nil {
					return errorf("not_found", ExitNotFound, "myco runner provider list", "%v", err)
				}
				out["provider"], out["name"] = p.Ref, p.Name
				ref = p.Ref
			}
			if e.in.Bool("dry-run") {
				out["action"] = "set_provider"
				return e.dryRun(out)
			}
			cfg.Runner.Provider = ref
			if err := config.Save(path, cfg); err != nil {
				return err
			}
			return e.p.Print(out, func(w io.Writer) {
				if ref == "" {
					fmt.Fprintf(w, "%s now follows this machine's provider (from its next message)\n", cfg.Agent)
				} else {
					fmt.Fprintf(w, "%s now uses %s (from its next message)\n", cfg.Agent, out["name"])
				}
			})
		}),
	}
	show := &Command{
		Name:    "show",
		Summary: "Show which provider an agent's next run will use",
		Flags:   []Flag{{Name: "agent", Kind: String, Help: "Agent (default: the current session's identity)"}},
		Run: handler(func(e *env) error {
			path, err := e.agentProfilePath()
			if err != nil {
				return err
			}
			cfg, err := config.Load(path)
			if err != nil {
				return err
			}
			ch := resolveProvider(profileHarness(cfg), cfg.Runner.Provider, cfg.Runner.EnvFile)
			out := map[string]any{"agent": cfg.Agent, "pinned": cfg.Runner.Provider, "provider": ch.name, "source": ch.source}
			if ch.warn != "" {
				out["warning"] = ch.warn
			}
			return e.p.Print(out, func(w io.Writer) {
				name := ch.name
				if name == "" {
					name = "the CLI's own setting"
				}
				fmt.Fprintf(w, "%s: %s (%s)\n", cfg.Agent, name, ch.source)
				if ch.warn != "" {
					fmt.Fprintln(w, "warning: "+ch.warn)
				}
			})
		}),
	}
	return (&Command{Name: "provider", Summary: "Choose the model provider an agent's CLI uses"}).Add(list, set, show)
}

// profileHarness is the harness the runner uses for a profile.
func profileHarness(cfg *config.Config) string {
	if h := cfg.Runner.Harness; h != "" {
		return h
	}
	if h := runnerHarnessFor(cfg.Harness); h != "" {
		return h
	}
	return "claude"
}
