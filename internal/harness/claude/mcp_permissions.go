package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// mcpAllowRules names configured servers individually. Claude Code ignores
// mcp__* as an allow rule: only mcp__<literal server>__* works. We never
// inspect or copy server env, headers, command arguments, or URL values.
func mcpAllowRules(workdir string, env []string) []string {
	names := map[string]bool{}
	home, _ := os.UserHomeDir()
	if home != "" {
		var user struct {
			Servers  map[string]json.RawMessage `json:"mcpServers"`
			Projects map[string]struct {
				Servers map[string]json.RawMessage `json:"mcpServers"`
			} `json:"projects"`
		}
		if readMCPNames(filepath.Join(home, ".claude.json"), &user) {
			for name := range user.Servers {
				names[name] = true
			}
			if workdir != "" {
				abs, _ := filepath.Abs(workdir)
				for name := range user.Projects[abs].Servers {
					names[name] = true
				}
			}
		}
	}
	if workdir != "" {
		var project struct {
			Servers map[string]json.RawMessage `json:"mcpServers"`
		}
		if readMCPNames(filepath.Join(workdir, ".mcp.json"), &project) {
			for name := range project.Servers {
				names[name] = true
			}
		}
	}
	_ = env // settings from CLAUDE_CONFIG_DIR do not define mcpServers.
	out := make([]string, 0, len(names))
	for name := range names {
		if name == "" || strings.ContainsAny(name, "\x00\n\r*") {
			continue
		}
		// Claude Code turns spaces and punctuation in configured server names
		// into underscores in the tool's canonical name.
		normalized := strings.Map(func(r rune) rune {
			if r == '.' || r == ' ' {
				return '_'
			}
			return r
		}, name)
		out = append(out, "mcp__"+normalized+"__*")
	}
	sort.Strings(out)
	return out
}

func readMCPNames(path string, out any) bool {
	b, err := os.ReadFile(path)
	return err == nil && json.Unmarshal(b, out) == nil
}
