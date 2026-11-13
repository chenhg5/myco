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
	"sync"
	"time"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/config"
	"github.com/chenhg5/myco/internal/harness"
	"github.com/chenhg5/myco/internal/machineid"
)

// runtimeReporter tells the hub, in the background and best effort, which
// CLI, model and provider an agent runs with; it reports again whenever the
// provider changes (e.g. picked in the console, or switched in cc-switch).
type runtimeReporter struct {
	c   *client.Client
	log *slog.Logger
	cfg string // runner.model, if set

	mu      sync.Mutex
	base    *api.Runtime // nil until the CLI version is known
	choice  providerChoice
	model   string // the agent's run settings from the hub ("" = none)
	effort  string
	quota   *api.Quota // the account's limits, as the CLI last reported them
	last    api.Runtime
	sending bool
}

// reportToHub reports this machine (once) and the agent's runtime, and
// returns the reporter for later provider changes.
func (e *env) reportToHub(c *client.Client, me *api.Agent, h harness.Harness, rc config.Runner, access harness.Access, choice providerChoice, log *slog.Logger) *runtimeReporter {
	rep := &runtimeReporter{c: c, log: log, cfg: rc.Model, choice: choice}
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

// RunSettings records the model and effort the next run starts with.
func (r *runtimeReporter) RunSettings(model, effort string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	changed := r.model != model || r.effort != effort
	r.model, r.effort = model, effort
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
	rt.Quota = r.quota
	if rt == r.last {
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
