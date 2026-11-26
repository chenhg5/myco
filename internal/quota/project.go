package quota

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Project-level account overrides have not been resolved by this collector.
// Fail closed instead of assigning the global login to a different connection.
func projectAccountOverride(in Input) bool {
	if in.Workdir == "" {
		return false
	}
	dir := canonical(in.Workdir)
	home := canonical(in.Home)
	for dir != "" && dir != home {
		var names []string
		switch in.CLI {
		case "claude":
			names = []string{".claude/settings.json", ".claude/settings.local.json"}
		case "codex":
			names = []string{".codex/config.toml"}
		case "opencode":
			names = []string{"opencode.json", "opencode.jsonc", ".opencode/opencode.json", ".opencode/opencode.jsonc"}
		}
		for _, name := range names {
			path := filepath.Join(dir, filepath.FromSlash(name))
			raw, err := os.ReadFile(path)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil || len(raw) > 4<<20 {
				return true
			}
			var cfg map[string]any
			if in.CLI == "codex" {
				if _, err = toml.Decode(string(raw), &cfg); err != nil {
					return true
				}
			} else if in.CLI == "opencode" {
				if !readJSONC(path, &cfg) {
					return true
				}
			} else if json.Unmarshal(raw, &cfg) != nil {
				return true
			}
			switch in.CLI {
			case "codex":
				for _, key := range []string{"model_provider", "model_providers", "openai_base_url", "cli_auth_credentials_store", "profile", "profiles"} {
					if _, ok := cfg[key]; ok {
						return true
					}
				}
			case "opencode":
				for _, key := range []string{"model", "provider"} {
					if _, ok := cfg[key]; ok {
						return true
					}
				}
			case "claude":
				for _, key := range []string{"apiKeyHelper", "forceLoginMethod", "forceLoginOrgUUID"} {
					if _, ok := cfg[key]; ok {
						return true
					}
				}
				if env, ok := cfg["env"].(map[string]any); ok {
					for key := range env {
						if strings.HasPrefix(key, "ANTHROPIC_") || key == "CLAUDE_CONFIG_DIR" || key == "CLAUDE_CODE_OAUTH_TOKEN" || strings.HasPrefix(key, "CLAUDE_CODE_USE_") {
							return true
						}
					}
				}
			}
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return false
}
