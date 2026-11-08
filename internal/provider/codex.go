package provider

import (
	"net/url"
	"sort"
	"strings"
)

// CodexKeyEnv carries a pinned Codex provider's API key to the CLI (the
// provider's env_key points at it), so the key is never on the command line.
const CodexKeyEnv = "MYCO_CODEX_API_KEY"

// codexTopKeys are the top-level config.toml keys a provider may set; others
// (sandbox, approvals, MCP servers...) stay with the agent's own settings.
var codexTopKeys = map[string]bool{
	"model": true, "model_provider": true, "model_reasoning_effort": true, "model_reasoning_summary": true,
	"model_verbosity": true, "model_context_window": true, "model_max_output_tokens": true,
	"model_auto_compact_token_limit": true, "disable_response_storage": true, "openai_base_url": true,
	"preferred_auth_method": true,
}

// CodexOverrides turns a Codex provider (cc-switch's config.toml text and
// its API key) into `codex -c key=value` arguments and environment
// variables. -c overrides beat ~/.codex/config.toml, and a per-run override
// does not touch the session store, so resuming keeps working.
func CodexOverrides(configTOML, apiKey string) (args []string, env map[string]string, host, model string) {
	env = map[string]string{}
	top := map[string]string{}
	tables := map[string][][2]string{}
	var order []string
	cur := ""
	for _, raw := range strings.Split(configTOML, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") && !strings.HasPrefix(line, "[[") {
			cur = strings.ReplaceAll(strings.Trim(line, "[] "), `"`, "")
			if strings.HasPrefix(cur, "model_providers.") {
				if _, ok := tables[cur]; !ok {
					order = append(order, cur)
					tables[cur] = nil
				}
			}
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.Trim(strings.TrimSpace(k), `"`), strings.TrimSpace(v)
		if k == "" || v == "" || strings.HasPrefix(v, "[") && !strings.HasSuffix(v, "]") { // multi-line arrays: skip
			continue
		}
		switch {
		case cur == "" && codexTopKeys[k]:
			top[k] = v
		case strings.HasPrefix(cur, "model_providers."):
			tables[cur] = append(tables[cur], [2]string{k, v})
		}
	}
	keys := make([]string, 0, len(top))
	for k := range top {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "-c", k+"="+top[k])
	}
	model = unquote(top["model"])
	if u := unquote(top["openai_base_url"]); u != "" {
		host = hostOf(u)
	}
	active := "model_providers." + unquote(top["model_provider"])
	for _, t := range order {
		var parts []string
		for _, kv := range tables[t] {
			switch kv[0] {
			case "env_key", "requires_openai_auth":
				if apiKey != "" {
					continue // replaced below
				}
			case "base_url":
				if t == active {
					host = hostOf(unquote(kv[1]))
				}
			}
			parts = append(parts, kv[0]+"="+kv[1])
		}
		if apiKey != "" {
			parts = append(parts, `env_key="`+CodexKeyEnv+`"`, "requires_openai_auth=false")
		}
		args = append(args, "-c", t+"={"+strings.Join(parts, ",")+"}")
	}
	if apiKey != "" {
		env[CodexKeyEnv] = apiKey
		env["OPENAI_API_KEY"] = apiKey // the built-in openai provider (openai_base_url)
	}
	return args, env, host, model
}

func unquote(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
		return v[1 : len(v)-1]
	}
	return v
}

func hostOf(u string) string {
	if p, err := url.Parse(u); err == nil && p.Host != "" {
		return p.Host
	}
	return ""
}
