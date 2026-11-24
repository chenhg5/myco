package cli

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	. "github.com/myco-ai/myco/internal/cli/cmdkit"
	"github.com/myco-ai/myco/internal/config"
	"github.com/myco-ai/myco/internal/provider"
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
		Examples: []string{"myco runner provider list", "myco runner provider list --cli codex"},
		Flags:    []Flag{{Name: "cli", Kind: Enum, Values: []string{"claude", "codex"}, Default: "claude", Help: "Whose providers: Claude Code or Codex"}},
		Run: handler(func(e *env) error {
			ps, err := provider.List(provider.Dir(), e.in.String("cli"))
			if err != nil {
				return errorf("not_found", ExitNotFound, "install cc-switch, or put provider variables in the agent's env file: myco runner env capture",
					"%v", err)
			}
			return e.p.Print(map[string]any{"providers": ps}, func(w io.Writer) {
				if len(ps) == 0 {
					fmt.Fprintf(w, "cc-switch has no %s providers\n", e.in.String("cli"))
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
restart needed. Claude Code and Codex agents.
--provider custom pins --base-url and --model; read the key using --key-file <path>
(or --key-file - for stdin). Codex accepts --wire-api chat|responses (default responses).
Keys stay in a local private env file; neither keys nor full endpoints are sent to the hub.`,
		Examples: []string{
			"myco runner provider set --agent mac-claude-docs --provider cc-switch:deepseek",
			"myco runner provider set --agent mac-claude-docs --provider machine",
		},
		Flags: []Flag{
			{Name: "agent", Kind: String, Help: "Agent whose profile to update (default: the current session's identity)"},
			{Name: "provider", Kind: String, Help: "cc-switch:<id>, custom, or machine"},
			{Name: "base-url", Kind: String, Help: "Custom http(s) provider base URL"},
			{Name: "model", Kind: String, Help: "Custom provider model"},
			{Name: "wire-api", Kind: Enum, Values: []string{"chat", "responses"}, Help: "Codex custom provider protocol (default responses)"},
			{Name: "key-file", Kind: String, Help: "Read the key from a private file (- for stdin); never put it in argv"},
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
			var manual *provider.Manual
			if ref != "custom" && (e.in.IsSet("base-url") || e.in.IsSet("key-file") || e.in.IsSet("model") || e.in.IsSet("wire-api")) {
				return errorf("invalid_argument", ExitUsage, "use --provider custom", "custom flags require --provider custom")
			}
			switch {
			case ref == "":
				return &Error{Code: "invalid_usage", Message: "--provider is required", Suggestion: "myco runner provider list", Exit: ExitUsage}
			case ref == "custom":
				file := e.in.String("key-file")
				if file == "" {
					return errorf("invalid_argument", ExitUsage, "pass --key-file <path> or --key-file -", "--key-file is required")
				}
				var reader io.Reader
				if file == "-" {
					reader = e.in.Stdin
				} else {
					f, err := os.Open(file)
					if err != nil {
						return errorf("invalid_argument", ExitUsage, "check --key-file", "cannot open key file")
					}
					defer f.Close()
					reader = f
				}
				key, err := io.ReadAll(io.LimitReader(reader, 16385))
				if err != nil || len(key) > 16384 {
					return errorf("invalid_argument", ExitUsage, "use a key file up to 16 KB", "cannot read key")
				}
				manual = &provider.Manual{BaseURL: e.in.String("base-url"), APIKey: string(key), Model: e.in.String("model"), WireAPI: e.in.String("wire-api")}
				if err := manual.Validate(providerCLIs[profileHarness(cfg)]); err != nil {
					return errorf("invalid_argument", ExitUsage, "check custom provider fields", "%v", err)
				}
				ref = ""
				out["provider"], out["host"], out["name"] = "env_file", hostFromBase(manual.BaseURL), "custom address"
			case ref == "machine" || ref == "none":
				ref = ""
				out["provider"] = "machine"
			default:
				h := profileHarness(cfg)
				if providerCLIs[h] == "" {
					return errorf("invalid_argument", ExitUsage, "", "pinned providers are supported for Claude Code and Codex agents (this agent runs %s)", h)
				}
				p, err := provider.Resolve(provider.Dir(), ref, providerCLIs[h])
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
			if manual != nil {
				file := cfg.Runner.EnvFile
				if file == "" {
					sum := sha256.Sum256([]byte(path))
					file = filepath.Join(config.Dir(e.defaultPath), "env", fmt.Sprintf("provider-%x.env", sum[:12]))
				}
				if err := provider.WriteEnv(file, cfg.Agent, manual.Variables(providerCLIs[profileHarness(cfg)])); err != nil {
					return err
				}
				cfg.Runner.EnvFile = file
			} else if ref == "" && cfg.Runner.EnvFile != "" {
				if err := provider.WriteEnv(cfg.Runner.EnvFile, cfg.Agent, nil); err != nil {
					return err
				}
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
			out := map[string]any{"agent": cfg.Agent, "pinned": cfg.Runner.Provider, "provider": ch.name, "provider_host": ch.host, "source": ch.source}
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
