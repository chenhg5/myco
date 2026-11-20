package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// mcpAllowRules names configured servers individually: Claude Code takes
// mcp__<server> as "every tool of that server" (in every version; the
// mcp__<server>__* form is newer), and no rule for all servers at once. We
// never inspect or copy server env, headers, command arguments, or URL values.
func mcpAllowRules(workdir string, env []string) []string {
	names := map[string]bool{}
	if state := claudeState(env); state != "" {
		var user struct {
			Servers  map[string]json.RawMessage `json:"mcpServers"`
			Projects map[string]struct {
				Servers map[string]json.RawMessage `json:"mcpServers"`
			} `json:"projects"`
		}
		if readMCPNames(state, &user) {
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
	out := make([]string, 0, len(names))
	for name := range names {
		if name == "" || strings.ContainsAny(name, "\x00\n\r*") {
			continue
		}
		out = append(out, "mcp__"+toolName(name))
	}
	sort.Strings(out)
	return out
}

func readMCPNames(path string, out any) bool {
	b, err := os.ReadFile(path)
	return err == nil && json.Unmarshal(b, out) == nil
}

// toolName is a server's name as Claude Code writes it in its tools' names:
// anything but letters, digits, _ and - becomes _.
func toolName(name string) string {
	return strings.Map(func(r rune) rune {
		if r == '_' || r == '-' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return '_'
	}, name)
}

// claudeState is Claude Code's own state file (user and per-project MCP
// servers): in CLAUDE_CONFIG_DIR when the run sets one, else ~/.claude.json.
func claudeState(env []string) string {
	dir := os.Getenv("CLAUDE_CONFIG_DIR")
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, "CLAUDE_CONFIG_DIR="); ok {
			dir = v
		}
	}
	if dir != "" {
		return filepath.Join(dir, ".claude.json")
	}
	home, _ := os.UserHomeDir()
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".claude.json")
}
