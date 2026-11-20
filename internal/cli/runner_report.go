package cli

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/chenhg5/myco/internal/agenttools"
	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/config"
	"github.com/chenhg5/myco/internal/harness"
	"github.com/chenhg5/myco/internal/harness/codex"
	"github.com/chenhg5/myco/internal/machineid"
)

// runtimeReporter tells the hub, in the background and best effort, which
// CLI, model and provider an agent runs with; it reports again whenever the
// provider changes (e.g. picked in the console, or switched in cc-switch).
type runtimeReporter struct {
	c   *client.Client
	log *slog.Logger
	cfg string // runner.model, if set

	mu     sync.Mutex
	base   *api.Runtime // nil until the CLI version is known
	choice providerChoice
	model  string // the agent's run settings from the hub ("" = none)
	effort string
	access string
	quota  *api.Quota // the account's limits, as the CLI last reported them
	// maxAccess is the machine's spawn.max_access as last read, when it has
	// changed since the start ("" = as at the start).
	maxAccess string
	last      api.Runtime
	sending   bool
	// models lists the models the CLI offers, read again for each report
	// (its list changes as the CLI updates); nil when it does not say.
	models func() []api.RuntimeModel
}

// reportToHub reports this machine (once) and the agent's runtime, and
// returns the reporter for later provider changes.
func (e *env) reportToHub(c *client.Client, me *api.Agent, h harness.Harness, rc config.Runner, access, maxAccess harness.Access, choice providerChoice, workdir string, log *slog.Logger) *runtimeReporter {
	rep := &runtimeReporter{c: c, log: log, cfg: rc.Model, choice: choice, models: cliModels(h.Name())}
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
		rt := api.Runtime{CLI: h.Name(), Access: string(access), MaxAccess: string(maxAccess), Features: runnerFeatures(h), Env: runnerEnv()}
		if h.Name() == "claude" || h.Name() == "codex" {
			tools := agenttools.Scan(h.Name(), agenttools.Options{Project: workdir, ClaudeConfigDir: e.in.Getenv("CLAUDE_CONFIG_DIR"), CodexHome: e.in.Getenv("CODEX_HOME")}).ProjectOnly(workdir)
			rt.MCP, rt.Skills, rt.Plugins, rt.ToolWarnings = tools.MCP, tools.Skills, tools.Plugins, tools.Warnings
		}
		if m := cliConfigModel(h.Name(), e.in.Getenv); m != "" {
			rt.Model, rt.ModelSource = m, "cli-config"
		} else if h.Name() != "exec" && h.Name() != "shell" && h.Name() != "machine" {
			rt.ModelSource = "default"
		}
		if ck, ok := h.(harness.Checker); ok {
			if v := ck.Check(ctx).Version; v != "" && len(v) < 60 {
				rt.CLIVersion = v
			}
		}
		rep.mu.Lock()
		rep.base = &rt
		rep.mu.Unlock()
		rep.flush(ctx)
	}()
	return rep
}

// RunSettings records the model, effort and access the next run starts with.
func (r *runtimeReporter) RunSettings(model, effort, access string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	changed := r.model != model || r.effort != effort || r.access != access
	r.model, r.effort, r.access = model, effort, access
	r.mu.Unlock()
	if changed {
		r.flushLater()
	}
}

// Quota records the account's usage limits a run reported.
func (r *runtimeReporter) Quota(q harness.Quota) {
	if r == nil {
		return
	}
	out := &api.Quota{Source: q.Source, At: time.Now().UTC(), Windows: []api.QuotaWindow{}}
	for _, w := range q.Windows {
		aw := api.QuotaWindow{Name: w.Name, Label: w.Label, Used: w.Used, Status: w.Status}
		if !w.ResetsAt.IsZero() {
			t := w.ResetsAt
			aw.ResetsAt = &t
		}
		out.Windows = append(out.Windows, aw)
	}
	r.mu.Lock()
	r.quota = out
	r.mu.Unlock()
	r.flushLater()
}

// MaxAccess records the machine's spawn.max_access (machine.yaml) after it
// changed: the app reads from it whether agents may open a terminal here.
func (r *runtimeReporter) MaxAccess(a harness.Access) {
	if r == nil {
		return
	}
	r.mu.Lock()
	changed := r.maxAccess != string(a)
	r.maxAccess = string(a)
	r.mu.Unlock()
	if changed {
		r.flushLater()
	}
}

// watchMaxAccess reports the machine's spawn.max_access again whenever
// machine.yaml changes it (`myco machine shell-access`, or the app on this
// computer), until ctx ends. read is e.machineMaxAccess.
func (r *runtimeReporter) watchMaxAccess(ctx context.Context, every time.Duration, start harness.Access, read func() harness.Access) {
	if r == nil {
		return
	}
	last := start
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if a := read(); a != last {
			last = a
			r.MaxAccess(a)
		}
	}
}

func (r *runtimeReporter) flushLater() {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		r.flush(ctx)
	}()
}

// Provider records the provider of the next run; the hub hears of changes.
func (r *runtimeReporter) Provider(ch providerChoice) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.choice = ch
	r.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		r.flush(ctx)
	}()
}

func (r *runtimeReporter) flush(ctx context.Context) {
	r.mu.Lock()
	if r.base == nil || r.sending {
		r.mu.Unlock()
		return
	}
	rt := *r.base
	if r.models != nil {
		rt.Models = r.models()
	}
	ch := r.choice
	switch {
	case r.cfg != "":
		rt.Model, rt.ModelSource = r.cfg, "configured"
	case ch.model != "" && ch.settings != nil:
		rt.Model, rt.ModelSource = ch.model, "provider"
	}
	rt.Provider, rt.ProviderSource = ch.name, ch.source
	// Chosen in the app: wins over the machine's settings for every run.
	if r.model != "" {
		rt.Model, rt.ModelSource = r.model, "app"
	}
	if r.effort != "" {
		rt.Effort, rt.EffortSource = r.effort, "app"
	}
	if r.access != "" {
		rt.Access = r.access
	}
	rt.Quota = r.quota
	if r.maxAccess != "" {
		rt.MaxAccess = r.maxAccess
	}
	if reflect.DeepEqual(rt, r.last) {
		r.mu.Unlock()
		return
	}
	r.sending = true
	r.mu.Unlock()
	err := r.c.ReportRuntime(ctx, rt)
	r.mu.Lock()
	r.sending = false
	if err == nil {
		r.last = rt
	}
	again := err == nil && (r.choice.name != ch.name || r.choice.source != ch.source || r.choice.model != ch.model)
	r.mu.Unlock()
	if err != nil {
		r.log.Debug("runtime report failed (older hub?)", "err", err)
	}
	if again {
		r.flush(ctx)
	}
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
			Model string         `json:"model"`
			Env   map[string]any `json:"env"`
		}
		_ = json.Unmarshal(b, &s)
		if m := strings.TrimSpace(s.Model); m != "" {
			return m
		}
		// cc-switch and similar tools set the model through env.
		m, _ := s.Env["ANTHROPIC_MODEL"].(string)
		return strings.TrimSpace(m)
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

// cliModels reads the models a CLI offers on this computer: Codex keeps
// the list it got from its provider (models_cache.json); Claude Code's
// aliases are fixed, so it says nothing.
func cliModels(cli string) func() []api.RuntimeModel {
	if cli != "codex" {
		return nil
	}
	return func() []api.RuntimeModel {
		var out []api.RuntimeModel
		for _, m := range codex.CachedModels() {
			out = append(out, api.RuntimeModel{ID: m.ID, Name: m.Name, Efforts: m.Efforts})
		}
		return out
	}
}

// runnerFeatures are what this runner supports for harness h beyond the
// basics, reported with its runtime so apps can show the controls. Each
// capability adds its name here when it lands.
func runnerFeatures(h harness.Harness) []string {
	// Claude headless runs need an explicit allow rule for each MCP server.
	// Codex MCP calls are not controlled by its shell sandbox flag, so this
	// capability is advertised only for the harness whose flag we change.
	if h.Name() == "claude" {
		return []string{"mcp_tools"}
	}
	return nil
}

// Goodbye tells the hub this runner is stopping (best effort, a few
// seconds at most), so the app can say why the agent is offline instead of
// only that it is. The next start reports without it.
func (r *runtimeReporter) Goodbye() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.base == nil {
		r.mu.Unlock()
		return
	}
	rt := r.last
	if reflect.DeepEqual(rt, api.Runtime{}) {
		rt = *r.base
	}
	r.mu.Unlock()
	rt.Away = "stopped"
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := r.c.ReportRuntime(ctx, rt); err != nil {
		r.log.Debug("goodbye report failed", "err", err)
	}
}

// runnerEnv is where this runner runs when that explains going offline:
// "wsl" inside Windows' WSL, which Windows shuts down when it is idle.
func runnerEnv() string {
	if runtime.GOOS != "linux" {
		return ""
	}
	if b, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil && strings.Contains(strings.ToLower(string(b)), "microsoft") {
		return "wsl"
	}
	return ""
}
