package cli

import (
	"errors"
	"net/url"
	"strings"

	"github.com/chenhg5/myco/internal/config"
	"github.com/chenhg5/myco/internal/harness"
	"github.com/chenhg5/myco/internal/provider"
)

// providerChoice is the provider an agent's next run uses, and how it was
// chosen (for the runtime report and the console).
type providerChoice struct {
	settings *harness.ProviderSettings // nil: the machine's own setting
	name     string                    // display name, never a key
	source   string                    // pinned | machine | env_file | missing
	model    string                    // model the provider sets, if any
	warn     string                    // why a pinned provider was not used
}

// providerCLIs maps harnesses to the cc-switch app whose providers they take.
var providerCLIs = map[string]string{"claude": "claude"}

// resolveProvider picks the provider for cli: the pinned cc-switch provider
// (ref), else provider variables from the agent's env file, else whatever
// the machine is set to (cc-switch's current provider, when it has one).
func resolveProvider(cli, ref, envFile string) providerChoice {
	app, supported := providerCLIs[cli]
	var ch providerChoice
	if ref != "" {
		if !supported {
			ch.warn = "pinned providers are supported for claude only; using the machine's setting"
		} else if p, err := provider.Resolve(provider.Dir(), ref, app); err != nil {
			ch = providerChoice{name: strings.TrimPrefix(ref, provider.RefPrefix), source: "missing", warn: err.Error() + "; using the machine's setting"}
			if errors.Is(err, provider.ErrNotFound) {
				ch.warn = "the pinned cc-switch provider no longer exists; using the machine's setting"
			}
		} else {
			return providerChoice{settings: &harness.ProviderSettings{Env: p.Env, Model: p.Model},
				name: p.Name, source: "pinned", model: orStr(p.Model, p.Env["ANTHROPIC_MODEL"])}
		}
	}
	if ch.source == "" && supported && envFile != "" {
		// Claude Code's settings.json env beats the process environment, so
		// provider variables from the env file must be passed as settings.
		if vars, err := config.ReadEnvFile(envFile); err == nil {
			env := map[string]string{}
			for _, kv := range vars {
				k, v, _ := strings.Cut(kv, "=")
				if isProviderVar(k) {
					env[k] = v
				}
			}
			if len(env) > 0 {
				name := "env file"
				if u, err := url.Parse(env["ANTHROPIC_BASE_URL"]); err == nil && u.Host != "" {
					name = u.Host
				}
				return providerChoice{settings: &harness.ProviderSettings{Env: env}, name: name, source: "env_file", model: env["ANTHROPIC_MODEL"]}
			}
		}
	}
	if supported {
		if p, ok := provider.Current(provider.Dir(), app); ok {
			if ch.source == "missing" {
				ch.name = p.Name
				return ch
			}
			return providerChoice{name: p.Name, source: "machine", warn: ch.warn}
		}
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
