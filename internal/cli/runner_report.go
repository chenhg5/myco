package cli

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/myco-ai/myco/internal/agenttools"
	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
	"github.com/myco-ai/myco/internal/config"
	"github.com/myco-ai/myco/internal/harness"
	"github.com/myco-ai/myco/internal/harness/codex"
	"github.com/myco-ai/myco/internal/machineid"
	"github.com/myco-ai/myco/internal/processwatch"
	"github.com/myco-ai/myco/internal/provider"
	"github.com/myco-ai/myco/internal/quota"
	"github.com/myco-ai/myco/internal/runner"
)

var runnerInstance = rand.Text()

// runtimeReporter tells the hub, in the background and best effort, which
// CLI, model and provider an agent runs with; it reports again whenever the
// provider changes (e.g. picked in the console, or switched in cc-switch).
type runtimeReporter struct {
	c           *client.Client
	log         *slog.Logger
	restartPath string
	cfg         string // runner.model, if set

	mu               sync.Mutex
	base             *api.Runtime // nil until the CLI version is known
	choice           providerChoice
	model            string // the agent's run settings from the hub ("" = none)
	effort           string
	access           string
	slots            *api.RunnerSlots
	quota            *api.Quota // the account's limits, as the CLI last reported them
	quotaStore       *quota.Store
	quotaCancel      context.CancelFunc
	quotaSource      quota.Source
	runQuotaRef      string
	runQuotaStarted  time.Time
	runQuotaIdentity quota.Source
	quotaInput       func(providerChoice) quota.Input
	// maxAccess is the machine's spawn.max_access as last read, when it has
	// changed since the start ("" = as at the start).
	maxAccess string
	last      api.Runtime
	sending   bool
	stopped   bool
	// models lists the models the CLI offers, read again for each report
	// (its list changes as the CLI updates); nil when it does not say.
	models  func() []api.RuntimeModel
	account func() string
	// provModels is the list of the provider the agent uses with an API
	// key (startModels), target where that provider is; refresh* follow
	// the App's requests to list again.
	provModels *provider.ModelList
	target     providerTarget
	hasTarget  bool
	refresh    chan struct{}
	// modelsDone is closed when the models loop has stopped (tests wait
	// for it before their temporary directories go).
	modelsDone  chan struct{}
	refreshSeen bool
	refreshLast string
}

// reportToHub reports this machine (once) and the agent's runtime, and
// returns the reporter for later provider changes.
func (e *env) reportToHub(c *client.Client, me *api.Agent, h harness.Harness, rc config.Runner, access, maxAccess harness.Access, choice providerChoice, workdir string, log *slog.Logger) *runtimeReporter {
	// Capacity reports must not wait for CLI version/tool discovery (which
	// may take seconds on a busy computer). Enrich the same base later.
	base := api.Runtime{RunnerInstance: runnerInstance, CLI: h.Name(), Access: string(access), MaxAccess: string(maxAccess), Features: runnerFeatures(h), Env: runnerEnv()}
	rep := &runtimeReporter{restartPath: runnerRestartPath(e.defaultPath), c: c, log: log, cfg: rc.Model, choice: choice, models: cliModels(h.Name()), base: &base}
	rep.account = func() string {
		vars, _ := config.ReadEnvFile(rc.EnvFile)
		env := map[string]string{}
		for _, kv := range vars {
			k, v, _ := strings.Cut(kv, "=")
			env[k] = v
		}
		get := func(key string) string {
			if value, ok := env[key]; ok {
				return value
			}
			return e.in.Getenv(key)
		}
		return runtimeAccount(h.Name(), accountHome(get), get)
	}
	rep.flushLater() // clear any prior restart lease before slow CLI/tool discovery
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
		rt := base
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
		// Health may arrive while CLI/tool discovery is running. Keep that
		// latest observation when publishing the enriched immutable base.
		rt.MachineHealth = rep.base.MachineHealth
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
	r.mu.Lock()
	store, input, ch, runRef, started, identity := r.quotaStore, r.quotaInput, r.choice, r.runQuotaRef, r.runQuotaStarted, r.runQuotaIdentity
	r.mu.Unlock()
	if store == nil || input == nil {
		return
	}
	src := store.Discover(input(ch))
	if src.Ref != runRef || !src.CanRecord(identity) || src.Status != "ok" || q.Source != src.Provider || len(q.Windows) == 0 || (q.Source == "codex" && (q.At.IsZero() || q.At.Before(started))) {
		return
	}
	at := q.At
	if at.IsZero() {
		at = time.Now().UTC()
	}
	out := &api.Quota{Source: q.Source, Provider: src.Provider, ProfileRef: src.Ref, Plan: src.Plan, SourceKind: "event", Status: "ok", At: at, CheckedAt: time.Now().UTC(), Windows: []api.QuotaWindow{}}
	for _, w := range q.Windows {
		if math.IsNaN(w.Used) || math.IsInf(w.Used, 0) || w.Used < 0 || w.Used > 1 {
			return
		}
		aw := api.QuotaWindow{Name: w.Name, Label: w.Label, Used: w.Used, Status: w.Status, DurationSec: w.DurationSec}
		if aw.Status == "allowed_warning" {
			aw.Status = "allowed"
		}
		if w.Name == "five_hour" {
			aw.DurationSec = 18000
		}
		if strings.HasPrefix(w.Name, "seven_day") {
			aw.DurationSec = 604800
		}
		if !w.ResetsAt.IsZero() {
			t := w.ResetsAt
			aw.ResetsAt = &t
		}
		out.Windows = append(out.Windows, aw)
	}
	if merged := store.Event(src, *out); merged != nil {
		r.setQuota(src.Ref, merged)
	}
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
	store, input := r.quotaStore, r.quotaInput
	r.mu.Unlock()
	if store != nil && input != nil {
		src := store.Discover(input(ch))
		r.bindQuota(src)
		r.mu.Lock()
		r.runQuotaRef = src.Ref
		r.runQuotaIdentity = src
		r.runQuotaStarted = time.Now().UTC()
		r.mu.Unlock()
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		r.flush(ctx)
	}()
}

func (r *runtimeReporter) flush(ctx context.Context) {
	for {
		r.mu.Lock()
		if r.base == nil || r.sending || r.stopped {
			r.mu.Unlock()
			return
		}
		base := *r.base
		rt := base
		if supportsRemoteProvider(rt.CLI) {
			options := localProviderOptions(rt.CLI)
			rt.Providers = &options
		}
		if ms, from, at, ok := r.providerModelsLocked(); ok {
			rt.Models, rt.ModelsFrom, rt.ModelsAt = ms, from, at
		} else if r.models != nil {
			if rt.Models = r.models(); len(rt.Models) > 0 {
				rt.ModelsFrom = "cli"
			}
		}
		ch := r.choice
		switch {
		case r.cfg != "":
			rt.Model, rt.ModelSource = r.cfg, "configured"
		case ch.model != "" && ch.settings != nil:
			rt.Model, rt.ModelSource = ch.model, "provider"
		}
		rt.Provider, rt.ProviderSource, rt.ProviderHost = ch.name, ch.source, ch.host
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
		rt.RunnerSlots = r.slots
		if r.maxAccess != "" {
			rt.MaxAccess = r.maxAccess
		}
		if r.account != nil {
			rt.Account = r.account()
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
		again := err == nil && (!reflect.DeepEqual(*r.base, base) || r.choice.name != ch.name || r.choice.host != ch.host || r.choice.source != ch.source || r.choice.model != ch.model || !reflect.DeepEqual(r.slots, rt.RunnerSlots) || !reflect.DeepEqual(r.quota, rt.Quota))
		r.mu.Unlock()
		if err != nil {
			r.log.Debug("runtime report failed (older hub?)", "err", err)
		}
		if !again {
			return
		}
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
		return []string{"mcp_tools", "remote_provider", "usage", "command_watch", "models_refresh", "hub_provider"}
	}
	if h.Name() == "codex" {
		return []string{"remote_provider", "usage", "command_watch", "models_refresh", "hub_provider"}
	}
	if h.Name() != "exec" && h.Name() != "machine" {
		return []string{"usage", "command_watch"}
	}
	if h.Name() == "machine" {
		// It opens a new agent's provider sealed for it (i115), or fetches
		// one the hub keeps (i119).
		features := []string{"usage", "sealed_spawn", "hub_provider"}
		if managed, ok := h.(interface{ CanManageForks() bool }); ok && managed.CanManageForks() {
			features = append(features, "fork_retire_v1")
		}
		return features
	}
	return []string{"usage"}
}

// Goodbye tells the hub this runner is stopping (best effort, a few
// seconds at most), so the app can say why the agent is offline instead of
// only that it is. The next start reports without it.
func (r *runtimeReporter) Goodbye() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.stopped = true
	if r.quotaCancel != nil {
		r.quotaCancel()
	}
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
	if until := runnerRestartUntil(r.restartPath); !until.IsZero() {
		rt.Away, rt.RestartingUntil = "updating", until
	}
	rt.RunnerSlots = nil
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

// Slots always receives a fresh immutable snapshot. No other workspace's
// message IDs or names are included in a runtime report.
func (r *runtimeReporter) Slots(s *api.RunnerSlots) {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.slots != nil && s.At.Before(r.slots.At) {
		r.mu.Unlock()
		return
	}
	r.slots = s
	r.mu.Unlock()
	r.flushLater()
}
func (r *runtimeReporter) watchSlots(ctx context.Context, p *runner.SlotPool, owner string) {
	changes, stop := p.Subscribe()
	defer stop()
	tick := time.NewTicker(20 * time.Second)
	defer tick.Stop()
	r.Slots(p.Snapshot(owner))
	for {
		select {
		case <-ctx.Done():
			return
		case <-changes:
			// Coalesce machine-wide admissions instead of making one HTTP request
			// per release per agent during a burst.
			select {
			case <-ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
			}
		case <-tick.C:
		}
		r.Slots(p.Snapshot(owner))
	}
}

// watchHealth reads the shared sampler; there is no per-profile OS sampling.
func (r *runtimeReporter) watchHealth(ctx context.Context, m *processwatch.Manager, workspace, owner string) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		h := m.Snapshot(workspace, owner)
		r.mu.Lock()
		if r.base != nil {
			// The discovery goroutine still holds the initial base value.
			// Publish a copy instead of mutating that shared value in place.
			next := *r.base
			next.MachineHealth = h
			r.base = &next
		}
		r.mu.Unlock()
		r.flushLater()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
