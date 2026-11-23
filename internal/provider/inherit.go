package provider

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// Inherit picks, from the environment a session runs in (KEY=VALUE), the
// variables its CLI needs to reach its model the same way when a runner
// starts it later. A runner is a background service: it does not load the
// shell profile, so a key exported in ~/.zshrc (or a gateway's base URL) is
// missing there, and the CLI falls back to other auth and fails.
//
//   - Codex: CODEX_HOME; the env_key of every provider in its config.toml
//     (a gateway such as magpie, or a cc-switch provider, names the variable
//     its key is in); and, when it uses the built-in OpenAI provider,
//     OPENAI_API_KEY and OPENAI_BASE_URL.
//   - Claude Code: ANTHROPIC_* and its provider switches (Bedrock, Vertex,
//     Foundry), CLAUDE_CONFIG_DIR. A value that Claude Code's settings.json
//     sets the same is left out: it comes from the file, and a provider
//     switched there later (cc-switch) still applies.
//   - Both: the proxy variables.
//
// Values are secrets: callers store them in a private file and print names
// only.
func Inherit(app, home string, environ []string) map[string]string {
	vars := map[string]string{}
	for _, kv := range environ {
		if k, v, ok := strings.Cut(kv, "="); ok && k != "" {
			vars[k] = v
		}
	}
	getenv := func(k string) string { return vars[k] }
	out := map[string]string{}
	take := func(k string) {
		if v := getenv(k); v != "" {
			out[k] = v
		}
	}
	for _, k := range []string{"HTTPS_PROXY", "HTTP_PROXY", "ALL_PROXY", "NO_PROXY", "https_proxy", "http_proxy", "all_proxy", "no_proxy"} {
		take(k)
	}
	switch app {
	case "codex":
		take("CODEX_HOME")
		dir := getenv("CODEX_HOME")
		if dir == "" {
			dir = filepath.Join(home, ".codex")
		}
		keys, builtin := codexKeyVars(filepath.Join(dir, "config.toml"))
		for _, k := range keys {
			take(k)
		}
		if builtin {
			for _, k := range []string{"OPENAI_API_KEY", "OPENAI_BASE_URL", "OPENAI_ORGANIZATION", "OPENAI_PROJECT"} {
				take(k)
			}
		}
	case "claude":
		fromFile := claudeSettingsEnv(home, getenv)
		for _, k := range claudeVarNames(vars) {
			if v := getenv(k); v != "" && fromFile[k] != v {
				out[k] = v
			}
		}
	default:
		return nil
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// claudeExact are Claude Code provider variables outside ANTHROPIC_*.
var claudeExact = []string{"CLAUDE_CODE_OAUTH_TOKEN", "CLAUDE_CODE_USE_BEDROCK", "CLAUDE_CODE_USE_VERTEX",
	"CLAUDE_CODE_USE_FOUNDRY", "CLAUDE_CODE_SUBAGENT_MODEL", "CLAUDE_CONFIG_DIR", "API_TIMEOUT_MS",
	"AWS_REGION", "AWS_PROFILE", "CLOUD_ML_REGION"}

// claudeVarNames lists the Claude Code variables present: ANTHROPIC_* and
// the exact ones.
func claudeVarNames(vars map[string]string) []string {
	var names []string
	for k := range vars {
		if strings.HasPrefix(k, "ANTHROPIC_") {
			names = append(names, k)
		}
	}
	names = append(names, claudeExact...)
	sort.Strings(names)
	return names
}

// claudeSettingsEnv is the env block of Claude Code's settings.json.
func claudeSettingsEnv(home string, getenv func(string) string) map[string]string {
	dir := getenv("CLAUDE_CONFIG_DIR")
	if dir == "" {
		dir = filepath.Join(home, ".claude")
	}
	var doc struct {
		Env map[string]string `json:"env"`
	}
	if b, err := os.ReadFile(filepath.Join(dir, "settings.json")); err == nil {
		_ = json.Unmarshal(b, &doc)
	}
	return doc.Env
}

// codexKeyVars reads Codex's config.toml: the env_key of each provider it
// defines, and whether the active one is the built-in OpenAI provider
// (none set, "openai", or one it does not define).
func codexKeyVars(path string) (keys []string, builtin bool) {
	var doc struct {
		Provider  string `toml:"model_provider"`
		Providers map[string]struct {
			EnvKey string `toml:"env_key"`
		} `toml:"model_providers"`
	}
	if _, err := toml.DecodeFile(path, &doc); err != nil {
		return nil, true
	}
	seen := map[string]bool{}
	for _, p := range doc.Providers {
		if k := strings.TrimSpace(p.EnvKey); k != "" && !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	_, defined := doc.Providers[doc.Provider]
	return keys, doc.Provider == "" || doc.Provider == "openai" || !defined
}
