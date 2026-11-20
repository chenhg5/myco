// Package agenttools reads local CLI configuration into a secret-free inventory.
// It never starts an MCP server or writes CLI configuration.
package agenttools

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/BurntSushi/toml"
	"github.com/chenhg5/myco/internal/api"
	"gopkg.in/yaml.v3"
)

type Options struct{ Home, ClaudeConfigDir, CodexHome, Project string }
type Inventory struct {
	MCP      []api.MCPServer
	Skills   []api.ToolSkill
	Plugins  []api.ToolPlugin
	Warnings []string
}

func Scan(cli string, o Options) Inventory {
	if o.Home == "" {
		o.Home, _ = os.UserHomeDir()
	}
	if o.ClaudeConfigDir == "" {
		o.ClaudeConfigDir = filepath.Join(o.Home, ".claude")
	}
	if o.CodexHome == "" {
		o.CodexHome = filepath.Join(o.Home, ".codex")
	}
	if o.Project != "" {
		o.Project, _ = filepath.Abs(o.Project)
	}
	var out Inventory
	switch cli {
	case "claude":
		scanClaude(o, &out)
	case "codex":
		scanCodex(o, &out)
	}
	sort.Slice(out.MCP, func(i, j int) bool {
		if out.MCP[i].Scope != out.MCP[j].Scope {
			return out.MCP[i].Scope < out.MCP[j].Scope
		}
		return out.MCP[i].Name < out.MCP[j].Name
	})
	sort.Slice(out.Skills, func(i, j int) bool {
		if out.Skills[i].Scope != out.Skills[j].Scope {
			return out.Skills[i].Scope < out.Skills[j].Scope
		}
		return out.Skills[i].Name < out.Skills[j].Name
	})
	sort.Slice(out.Plugins, func(i, j int) bool { return out.Plugins[i].Name < out.Plugins[j].Name })
	return out
}

func (i Inventory) ProjectOnly(project string) Inventory {
	var out Inventory
	for _, m := range i.MCP {
		if m.Scope == "project" {
			out.MCP = append(out.MCP, m)
		}
	}
	for _, s := range i.Skills {
		if s.Scope == "project" {
			out.Skills = append(out.Skills, s)
		}
	}
	project, _ = filepath.Abs(project)
	for _, warning := range i.Warnings {
		if strings.HasPrefix(warning, project+string(filepath.Separator)) {
			out.Warnings = append(out.Warnings, warning)
		}
	}
	return out
}

func readJSON(path string, out any, inv *Inventory) bool {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		inv.Warnings = append(inv.Warnings, fmt.Sprintf("%s: cannot read", path))
		return false
	}
	if json.Unmarshal(b, out) != nil {
		inv.Warnings = append(inv.Warnings, fmt.Sprintf("%s: invalid JSON", path))
		return false
	}
	return true
}
func readTOML(path string, out any, inv *Inventory) bool {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		inv.Warnings = append(inv.Warnings, fmt.Sprintf("%s: cannot read", path))
		return false
	}
	if toml.Unmarshal(b, out) != nil {
		inv.Warnings = append(inv.Warnings, fmt.Sprintf("%s: invalid TOML", path))
		return false
	}
	return true
}

type mcpConfig struct {
	Command string `json:"command" toml:"command"`
	URL     string `json:"url" toml:"url"`
	Type    string `json:"type" toml:"type"`
	Enabled *bool  `json:"enabled" toml:"enabled"`
}

func appendMCP(out *Inventory, name, scope string, c mcpConfig) {
	if name == "" {
		return
	}
	m := api.MCPServer{Name: name, Scope: scope, Enabled: c.Enabled}
	switch {
	case c.Command != "":
		m.Transport = "stdio"
	case c.URL != "":
		m.Transport = "http"
		if c.Type == "sse" {
			m.Transport = "sse"
		}
		if u, e := url.Parse(c.URL); e == nil {
			m.Host = u.Hostname()
		}
	default:
		m.Transport = c.Type
	}
	if m.Transport == "" {
		m.Transport = "unknown"
	}
	out.MCP = append(out.MCP, m)
}
func appendMCPMap(out *Inventory, servers map[string]mcpConfig, scope string) {
	for name, c := range servers {
		appendMCP(out, name, scope, c)
	}
}

func scanClaude(o Options, out *Inventory) {
	var state struct {
		MCPServers map[string]mcpConfig `json:"mcpServers"`
		Projects   map[string]struct {
			MCPServers map[string]mcpConfig `json:"mcpServers"`
			Enabled    []string             `json:"enabledMcpjsonServers"`
			Disabled   []string             `json:"disabledMcpjsonServers"`
		} `json:"projects"`
	}
	if readJSON(filepath.Join(o.Home, ".claude.json"), &state, out) {
		appendMCPMap(out, state.MCPServers, "user")
		if o.Project != "" {
			appendMCPMap(out, state.Projects[o.Project].MCPServers, "project")
		}
	}
	scanSkills(filepath.Join(o.ClaudeConfigDir, "skills"), "user", out)
	if o.Project != "" {
		scanSkills(filepath.Join(o.Project, ".claude", "skills"), "project", out)
		var project struct {
			MCPServers map[string]mcpConfig `json:"mcpServers"`
		}
		if readJSON(filepath.Join(o.Project, ".mcp.json"), &project, out) {
			approval := state.Projects[o.Project]
			var settings struct {
				EnableAll bool `json:"enableAllProjectMcpjsonServers"`
			}
			readJSON(filepath.Join(o.ClaudeConfigDir, "settings.json"), &settings, out)
			for name, c := range project.MCPServers {
				if settings.EnableAll || contains(approval.Enabled, name) {
					yes := true
					c.Enabled = &yes
				}
				if contains(approval.Disabled, name) {
					no := false
					c.Enabled = &no
				}
				appendMCP(out, name, "project", c)
			}
		}
	}
	var settings struct {
		EnabledPlugins map[string]bool `json:"enabledPlugins"`
	}
	readJSON(filepath.Join(o.ClaudeConfigDir, "settings.json"), &settings, out)
	installed := struct {
		Plugins map[string][]struct {
			InstallPath string `json:"installPath"`
		} `json:"plugins"`
	}{}
	readJSON(filepath.Join(o.ClaudeConfigDir, "plugins", "installed_plugins.json"), &installed, out)
	names := map[string]bool{}
	for name, v := range settings.EnabledPlugins {
		names[name] = v
	}
	for name := range installed.Plugins {
		if _, ok := names[name]; !ok {
			names[name] = false
		}
	}
	for key, enabled := range names {
		name, market, _ := strings.Cut(key, "@")
		out.Plugins = append(out.Plugins, api.ToolPlugin{Name: name, Marketplace: market, Enabled: enabled})
		if !enabled {
			continue
		}
		for _, v := range installed.Plugins[key] {
			if v.InstallPath == "" {
				continue
			}
			var plugin struct {
				MCPServers map[string]mcpConfig `json:"mcpServers"`
			}
			if readJSON(filepath.Join(v.InstallPath, ".mcp.json"), &plugin, out) {
				appendMCPMap(out, plugin.MCPServers, "plugin")
			}
		}
	}
}

func scanCodex(o Options, out *Inventory) {
	var user struct {
		MCP     map[string]mcpConfig `toml:"mcp_servers"`
		Plugins map[string]struct {
			Enabled *bool `toml:"enabled"`
		} `toml:"plugins"`
	}
	if readTOML(filepath.Join(o.CodexHome, "config.toml"), &user, out) {
		appendMCPMap(out, user.MCP, "user")
		for name, p := range user.Plugins {
			enabled := p.Enabled == nil || *p.Enabled
			out.Plugins = append(out.Plugins, api.ToolPlugin{Name: name, Enabled: enabled})
		}
	}
	scanSkills(filepath.Join(o.CodexHome, "skills"), "user", out)
	if o.Project != "" {
		var project struct {
			MCP map[string]mcpConfig `toml:"mcp_servers"`
		}
		if readTOML(filepath.Join(o.Project, ".codex", "config.toml"), &project, out) {
			appendMCPMap(out, project.MCP, "project")
		}
		scanSkills(filepath.Join(o.Project, ".codex", "skills"), "project", out)
	}
}

func scanSkills(dir, scope string, out *Inventory) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		out.Warnings = append(out.Warnings, fmt.Sprintf("%s: cannot read", dir))
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name(), "SKILL.md")
		b, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			out.Warnings = append(out.Warnings, fmt.Sprintf("%s: cannot read", path))
			continue
		}
		if len(b) > 1<<20 {
			out.Warnings = append(out.Warnings, fmt.Sprintf("%s: too large", path))
			continue
		}
		name, desc := entry.Name(), ""
		if strings.HasPrefix(string(b), "---\n") {
			if end := strings.Index(string(b[4:]), "\n---"); end >= 0 {
				var meta struct {
					Name        string `yaml:"name"`
					Description string `yaml:"description"`
				}
				if yaml.Unmarshal(b[4:4+end], &meta) != nil {
					out.Warnings = append(out.Warnings, fmt.Sprintf("%s: invalid frontmatter", path))
					continue
				}
				if meta.Name != "" {
					name = meta.Name
				}
				desc = meta.Description
			}
		}
		if utf8.RuneCountInString(desc) > 200 {
			desc = string([]rune(desc)[:200])
		}
		skill := api.ToolSkill{Name: name, Description: desc, Scope: scope}
		if strings.Contains(string(b), "<!-- installed by myco:") {
			skill.ManagedBy = "myco"
		}
		out.Skills = append(out.Skills, skill)
	}
}
func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
