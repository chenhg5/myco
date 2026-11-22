package provider

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// MachineEndpoint observes the CLI's user settings and inherited environment.
// It does not copy keys or alter either CLI's configuration. Workdir overrides
// and command-line overrides belong to the caller, not this observer.
func MachineEndpoint(home, app string, getenv func(string) string) (host, model string) {
	base := ""
	if app == "claude" {
		base, model = getenv("ANTHROPIC_BASE_URL"), getenv("ANTHROPIC_MODEL")
		dir := getenv("CLAUDE_CONFIG_DIR")
		if dir == "" {
			dir = filepath.Join(home, ".claude")
		}
		var doc struct {
			Env   map[string]string `json:"env"`
			Model string            `json:"model"`
		}
		if b, err := os.ReadFile(filepath.Join(dir, "settings.json")); err == nil && json.Unmarshal(b, &doc) == nil {
			if v, ok := doc.Env["ANTHROPIC_BASE_URL"]; ok {
				base = v
			}
			if v, ok := doc.Env["ANTHROPIC_MODEL"]; ok {
				model = v
			} else if doc.Model != "" {
				model = doc.Model
			}
		}
	} else if app == "codex" {
		dir := getenv("CODEX_HOME")
		if dir == "" {
			dir = filepath.Join(home, ".codex")
		}
		var doc struct {
			Model     string `toml:"model"`
			Provider  string `toml:"model_provider"`
			Base      string `toml:"openai_base_url"`
			Providers map[string]struct {
				Base string `toml:"base_url"`
			} `toml:"model_providers"`
		}
		if _, err := toml.DecodeFile(filepath.Join(dir, "config.toml"), &doc); err == nil {
			model = doc.Model
			if p, ok := doc.Providers[doc.Provider]; ok {
				base = p.Base
			}
			if base == "" && (doc.Provider == "" || doc.Provider == "openai") {
				base = doc.Base
				if base == "" {
					base = getenv("OPENAI_BASE_URL")
				}
			}
		} else {
			base = getenv("OPENAI_BASE_URL")
		}
	}
	return hostOf(base), model
}

// DisplayName matches the active cc-switch endpoint rather than claiming its
// active provider is in use when another tool has changed the CLI's settings.
func DisplayName(host string, current Provider, hasCurrent bool) string {
	if strings.EqualFold(host, "127.0.0.1:3425") || strings.EqualFold(host, "localhost:3425") || host == "[::1]:3425" {
		return "magpie 网关"
	}
	if host != "" && hasCurrent && strings.EqualFold(host, current.Host) {
		return current.Name
	}
	return host
}
