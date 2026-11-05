package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	. "github.com/chenhg5/myco/internal/cli/cmdkit"
	"github.com/chenhg5/myco/internal/config"
)

// defaultCapturePrefixes / defaultCaptureVars cover Anthropic-compatible
// provider settings used by Claude Code (third-party base URL + key, model
// overrides, Bedrock/Vertex/Foundry switches). Exact names for CLAUDE_* so
// runtime variables of a surrounding Claude Code session are not captured.
var (
	defaultCapturePrefixes = []string{"ANTHROPIC_"}
	defaultCaptureVars     = []string{"CLAUDE_CODE_OAUTH_TOKEN", "CLAUDE_CODE_USE_BEDROCK", "CLAUDE_CODE_USE_VERTEX",
		"CLAUDE_CODE_USE_FOUNDRY", "AWS_REGION", "AWS_PROFILE", "CLOUD_ML_REGION", "ANTHROPIC_VERTEX_PROJECT_ID"}
)

func runnerEnvCommands() *Command {
	capture := &Command{
		Name:     "capture",
		Summary:  "Copy provider variables from this shell into an agent's private env file",
		Mutating: true,
		Long: `Background runners (launchd/systemd) do not load your shell profile, so a CLI configured through
exported variables (e.g. ANTHROPIC_BASE_URL + ANTHROPIC_API_KEY for an Anthropic-compatible
provider) falls back to other auth and fails. Run this in a shell where the CLI works: it writes
the matching variables to ~/.myco/env/<agent>.env (chmod 600) and sets runner.env_file in the
agent's profile. Only variable names are printed, never values.`,
		Examples: []string{
			"myco runner env capture --agent mac-claude-loops",
			"myco runner env capture --agent mac-codex --prefix OPENAI_ --var HTTPS_PROXY",
		},
		Flags: []Flag{
			{Name: "agent", Kind: String, Help: "Agent whose profile to update (default: the current session's identity)"},
			{Name: "prefix", Kind: List, Help: "Capture variables starting with these prefixes (default: ANTHROPIC_ plus Claude provider switches)"},
			{Name: "var", Kind: List, Help: "Also capture these exact variable names"},
		},
		Run: handler(func(e *env) error {
			path := e.cfgPath
			if n := e.in.String("agent"); n != "" {
				path = config.ProfilePath(e.defaultPath, n)
				if _, err := os.Stat(path); err != nil {
					return errorf("not_found", ExitNotFound, "list profiles: ls "+config.ProfilesDir(e.defaultPath),
						"no profile for agent %s (%s)", n, path)
				}
			} else if filepath.Dir(path) != config.ProfilesDir(e.defaultPath) {
				return &Error{Code: "invalid_usage", Message: "not inside an enrolled session; say which agent",
					Suggestion: "pass --agent <name>", Exit: ExitUsage}
			}
			cfg, err := config.Load(path)
			if err != nil {
				return err
			}
			prefixes := e.in.List("prefix")
			want := map[string]bool{}
			if len(prefixes) == 0 && !e.in.IsSet("var") {
				prefixes = defaultCapturePrefixes
				for _, v := range defaultCaptureVars {
					want[v] = true
				}
			}
			for _, v := range e.in.List("var") {
				want[v] = true
			}
			var lines, names []string
			for _, kv := range os.Environ() {
				k, v, _ := strings.Cut(kv, "=")
				match := want[k]
				for _, p := range prefixes {
					if strings.HasPrefix(k, p) {
						match = true
					}
				}
				if match && v != "" {
					lines = append(lines, k+"="+quoteEnv(v))
					names = append(names, k)
				}
			}
			sort.Strings(lines)
			sort.Strings(names)
			if len(names) == 0 {
				return errorf("not_found", ExitNotFound, "run this in the shell where the CLI works, or pass --var/--prefix",
					"no matching variables in this shell (prefixes: %s)", strings.Join(prefixes, ", "))
			}
			envPath := filepath.Join(config.Dir(e.defaultPath), "env", cfg.Agent+".env")
			out := map[string]any{"agent": cfg.Agent, "env_file": envPath, "variables": names, "profile": path}
			if e.in.Bool("dry-run") {
				out["action"] = "capture_env"
				return e.dryRun(out)
			}
			if err := os.MkdirAll(filepath.Dir(envPath), 0o700); err != nil {
				return err
			}
			body := "# captured by `myco runner env capture`; passed only to " + cfg.Agent + "'s CLI\n" + strings.Join(lines, "\n") + "\n"
			if err := os.WriteFile(envPath, []byte(body), 0o600); err != nil {
				return err
			}
			if err := os.Chmod(envPath, 0o600); err != nil {
				return err
			}
			cfg.Runner.EnvFile = envPath
			if err := config.Save(path, cfg); err != nil {
				return err
			}
			return e.p.Print(out, func(w io.Writer) {
				fmt.Fprintf(w, "captured %d variable(s) for %s: %s\nenv file: %s (600)\nrestart the runner to apply: myco runner service restart\n",
					len(names), cfg.Agent, strings.Join(names, ", "), envPath)
			})
		}),
	}
	return (&Command{Name: "env", Summary: "Manage the environment passed to agents' CLIs"}).Add(capture)
}

// quoteEnv double-quotes values that need it for ReadEnvFile round-trips.
func quoteEnv(v string) string {
	if strings.ContainsAny(v, " \t#\"'") {
		return `"` + strings.ReplaceAll(v, `"`, `\"`) + `"`
	}
	return v
}
