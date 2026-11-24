package cli

import (
	"errors"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/chenhg5/myco/internal/config"
	"github.com/chenhg5/myco/internal/harness"
	"github.com/chenhg5/myco/internal/provider"
)

// providerChoice is the provider an agent's next run uses, and how it was
// chosen (for the runtime report and the console).
type providerChoice struct {
	settings *harness.ProviderSettings // nil: the machine's own setting
	host     string
	name     string // display name, never a key
	source   string // pinned | machine | env_file | missing
	model    string // model the provider sets, if any
	warn     string // why a pinned provider was not used
}

// providerCLIs maps harnesses to the cc-switch app whose providers they take.
var providerCLIs = map[string]string{"claude": "claude", "codex": "codex"}

// resolveProvider picks the provider for cli: the pinned cc-switch provider
// (ref), else provider variables from the agent's env file, else whatever
// the machine is set to (cc-switch's current provider, when it has one).
func resolveProvider(cli, ref, envFile string) providerChoice {
	app, supported := providerCLIs[cli]
	var ch providerChoice
	if ref != "" {
		if !supported {
			ch.warn = "pinned providers are supported for Claude Code and Codex only; using the machine's setting"
		} else if p, err := provider.Resolve(provider.Dir(), ref, app); err != nil {
			ch = providerChoice{name: strings.TrimPrefix(ref, provider.RefPrefix), source: "missing", warn: err.Error() + "; using the machine's setting"}
			if errors.Is(err, provider.ErrNotFound) {
				ch.warn = "the pinned cc-switch provider no longer exists; using the machine's setting"
			}
		} else {
			return providerChoice{settings: &harness.ProviderSettings{Env: p.Env, Model: p.Model, ConfigArgs: p.CodexArgs},
				name: p.Name, host: p.Host, source: "pinned", model: orStr(p.Model, p.Env["ANTHROPIC_MODEL"])}
		}
	}
	if ch.source == "" && supported && envFile != "" {
		// Claude Code's settings.json env beats the process environment, so
		// provider variables from the env file must be passed as settings.
		if vars, err := config.ReadEnvFile(envFile); err == nil {
			env := map[string]string{}
			for _, kv := range vars {
				k, v, _ := strings.Cut(kv, "=")
				if isProviderVar(k) && app == "claude" || strings.HasPrefix(k, "OPENAI_") && app == "codex" {
					env[k] = v
				}
			}
			if len(env) > 0 {
				name := "env file"
				base, model := env["ANTHROPIC_BASE_URL"], env["ANTHROPIC_MODEL"]
				ps := &harness.ProviderSettings{Env: env}
				if app == "codex" {
					// Custom URLs select an isolated provider. A key alone (as
					// myco join keeps from the session) is only passed on:
					// Codex's config.toml keeps choosing the provider, which
					// may well be a custom one whose env_key is OPENAI_API_KEY.
					base, model = env["OPENAI_BASE_URL"], env["OPENAI_MODEL"]
					if base != "" {
						ps.ConfigArgs, ps.Env = provider.CustomCodex(base, env["OPENAI_API_KEY"], model, env["OPENAI_WIRE_API"])
					} else if model != "" {
						ps.ConfigArgs = []string{"-c", "model=" + strconv.Quote(model)}
					}
				}
				ps.Model = model
				if u, err := url.Parse(base); err == nil && u.Host != "" {
					name = u.Host
				}
				return providerChoice{settings: ps, name: provider.DisplayName(name, provider.Provider{}, false), host: hostFromBase(base), source: "env_file", model: model}
			}
		}
	}
	if supported {
		home, _ := os.UserHomeDir()
		host, model := provider.MachineEndpoint(home, app, os.Getenv)
		p, ok := provider.Current(provider.Dir(), app)
		name := provider.DisplayName(host, p, ok)
		if ch.source == "missing" {
			ch.name, ch.host = name, host
			return ch
		}
		return providerChoice{name: name, host: host, source: "machine", model: model, warn: ch.warn}
	}

	if ch.source == "" {
		ch.source = "machine"
	}
	return ch
}

// isProviderVar: variables that select Claude Code's model provider.
func isProviderVar(k string) bool {
	if strings.HasPrefix(k, "ANTHROPIC_") || strings.HasPrefix(k, "CLAUDE_CODE_USE_") {
		return true
	}
	switch k {
	case "CLAUDE_CODE_OAUTH_TOKEN", "CLAUDE_CODE_SUBAGENT_MODEL", "API_TIMEOUT_MS", "AWS_REGION", "AWS_PROFILE", "CLOUD_ML_REGION":
		return true
	}
	return false
}

func orStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func hostFromBase(base string) string {
	if u, err := url.Parse(base); err == nil {
		return u.Host
	}
	return ""
}
