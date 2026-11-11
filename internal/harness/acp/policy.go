package acp

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/chenhg5/myco/internal/harness"
)

// policy answers the agent's permission requests from the node's access
// level, as Claude Code's flags do for it:
//
//   - full: everything.
//   - workspace: reading, searching, fetching, and changing files inside the
//     working directory (or a directory the run was given).
//   - readonly and the default: reading, searching and fetching.
//
// Running myco itself (the agent's channel to reply, report progress and
// delegate) is always allowed; other commands only with full access.
type policy struct {
	access harness.Access
	dirs   []string // working directory and extra directories
}

type permissionRequest struct {
	ToolCall update `json:"toolCall"`
	Options  []struct {
		OptionID string `json:"optionId"`
		Kind     string `json:"kind"`
	} `json:"options"`
}

func (p policy) answer(params json.RawMessage) map[string]any {
	var req permissionRequest
	_ = json.Unmarshal(params, &req)
	want := []string{"reject_once", "reject_always"}
	if p.allows(req.ToolCall) {
		want = []string{"allow_once", "allow_always"}
	}
	for _, kind := range want {
		for _, o := range req.Options {
			if o.Kind == kind {
				return map[string]any{"outcome": map[string]string{"outcome": "selected", "optionId": o.OptionID}}
			}
		}
	}
	// No matching option (e.g. no reject option offered): decline.
	return map[string]any{"outcome": map[string]string{"outcome": "cancelled"}}
}

func (p policy) allows(t update) bool {
	if p.access == harness.AccessFull {
		return true
	}
	switch t.Kind {
	case "read", "search", "fetch", "think":
		return true
	case "execute":
		return isMyco(command(t))
	case "edit", "delete", "move":
		if p.access != harness.AccessWorkspace || len(t.Locations) == 0 {
			return false
		}
		for _, l := range t.Locations {
			if !p.inside(l.Path) {
				return false
			}
		}
		return true
	}
	return false
}

func (p policy) inside(path string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	for _, d := range p.dirs {
		if rel, err := filepath.Rel(d, filepath.Clean(path)); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// command is the shell command of an execute call: rawInput.command when
// the agent sends it, else its title (Gemini titles a call with the command).
func command(t update) string {
	var in struct {
		Command any `json:"command"`
	}
	if json.Unmarshal(t.RawInput, &in) == nil {
		switch c := in.Command.(type) {
		case string:
			return c
		case []any:
			var parts []string
			for _, x := range c {
				if s, ok := x.(string); ok {
					parts = append(parts, s)
				}
			}
			return strings.Join(parts, " ")
		}
	}
	return t.Title
}

// isMyco: a single myco invocation, nothing chained to it and nothing
// substituted into it. Single-quoted text may hold anything; double-quoted
// text no substitution; backslashes only inside single quotes (they would
// change where quotes end).
func isMyco(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	if cmd != "myco" && !strings.HasPrefix(cmd, "myco ") {
		return false
	}
	var quote rune
	for _, r := range cmd {
		switch {
		case quote == '\'':
			if r == '\'' {
				quote = 0
			}
		case quote == '"':
			if r == '"' {
				quote = 0
			} else if r == '`' || r == '$' || r == '\\' {
				return false
			}
		case r == '\'' || r == '"':
			quote = r
		case strings.ContainsRune(";&|`$<>()\\\n", r):
			return false
		}
	}
	return quote == 0
}
