package cli

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/config"
	"github.com/chenhg5/myco/internal/harness"
	"github.com/chenhg5/myco/internal/machineid"
)

// reportToHub tells the hub, in the background and best effort, which
// machine this agent runs on and with which CLI and model, so the console
// can show one entry per device and each node's model.
func (e *env) reportToHub(c *client.Client, me *api.Agent, h harness.Harness, rc config.Runner, access harness.Access, log *slog.Logger) {
	go func() {
		ctx, cancel := context.WithTimeout(e.in.Ctx, 30*time.Second)
		defer cancel()
		host, _ := os.Hostname()
		r := api.MachineReport{Fingerprint: machineid.Fingerprint(), Hostname: host, OS: runtime.GOOS, Arch: runtime.GOARCH, MycoVersion: Version}
		if m, err := config.LoadMachine(e.defaultPath); err == nil && m != nil {
			r.Name = m.Machine
		}
		// A claude.ai cloud container is a temporary sandbox, not a machine.
		if e.in.Getenv("CLAUDE_CODE_REMOTE_SESSION_ID") == "" {
			if err := c.ReportMachine(ctx, r); err != nil {
				log.Debug("machine report failed (older hub?)", "err", err)
			}
		}
		rt := api.Runtime{CLI: h.Name(), Access: string(access)}
		switch {
		case rc.Model != "":
			rt.Model, rt.ModelSource = rc.Model, "configured"
		default:
			if m := cliConfigModel(h.Name(), e.in.Getenv); m != "" {
				rt.Model, rt.ModelSource = m, "cli-config"
			} else if h.Name() != "exec" && h.Name() != "shell" && h.Name() != "machine" {
				rt.ModelSource = "default"
			}
		}
		if ck, ok := h.(harness.Checker); ok {
			if v := ck.Check(ctx).Version; v != "" && len(v) < 60 {
				rt.CLIVersion = v
			}
		}
		if err := c.ReportRuntime(ctx, rt); err != nil {
			log.Debug("runtime report failed (older hub?)", "err", err)
		}
	}()
}

var tomlModel = regexp.MustCompile(`(?m)^\s*model\s*=\s*"([^"]+)"`)

// cliConfigModel is the model a CLI uses by default, from its own settings
// (Claude Code: settings.json "model"; Codex: config.toml model = "...").
func cliConfigModel(cli string, getenv func(string) string) string {
	home, _ := os.UserHomeDir()
	switch cli {
	case "claude":
		dir := getenv("CLAUDE_CONFIG_DIR")
		if dir == "" {
			dir = filepath.Join(home, ".claude")
		}
		b, err := os.ReadFile(filepath.Join(dir, "settings.json"))
		if err != nil {
			return ""
		}
		var s struct {
			Model string `json:"model"`
		}
		_ = json.Unmarshal(b, &s)
		return strings.TrimSpace(s.Model)
	case "codex":
		dir := getenv("CODEX_HOME")
		if dir == "" {
			dir = filepath.Join(home, ".codex")
		}
		b, err := os.ReadFile(filepath.Join(dir, "config.toml"))
		if err != nil {
			return ""
		}
		// Only the top-level key (before the first [table]).
		top := "\n" + string(b)
		if i := strings.Index(top, "\n["); i >= 0 {
			top = top[:i]
		}
		if m := tomlModel.FindStringSubmatch(top); m != nil {
			return m[1]
		}
	}
	return ""
}
