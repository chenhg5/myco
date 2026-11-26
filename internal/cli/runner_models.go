package cli

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/provider"
)

// providerTarget is where an agent's CLI sends its requests with an API
// key of another vendor, and the tier variables the person set themselves.
type providerTarget struct {
	endpoint provider.Endpoint
	claude   bool
	set      map[string]bool // model variables already set (their values win)
}

// modelTiers are Claude Code's model variables myco may set for a run.
var modelTiers = []string{"ANTHROPIC_DEFAULT_OPUS_MODEL", "ANTHROPIC_DEFAULT_SONNET_MODEL", "ANTHROPIC_DEFAULT_HAIKU_MODEL", "ANTHROPIC_SMALL_FAST_MODEL"}

// providerTargetOf finds the target of a Claude Code or Codex agent from
// its run's environment and provider choice; false when it uses its
// vendor's own login or API (the CLI knows those models).
func providerTargetOf(cli, home string, env map[string]string, ch providerChoice) (providerTarget, bool) {
	var overrides map[string]string
	var args []string
	if ch.settings != nil {
		overrides, args = ch.settings.Env, ch.settings.ConfigArgs
	}
	switch cli {
	case "claude":
		vars := provider.ClaudeVars(home, env, overrides)
		ep, ok := provider.ClaudeEndpointOf(vars)
		if !ok {
			return providerTarget{}, false
		}
		set := map[string]bool{}
		for _, k := range modelTiers {
			if strings.TrimSpace(vars[k]) != "" {
				set[k] = true
			}
		}
		return providerTarget{endpoint: ep, claude: true, set: set}, true
	case "codex":
		merged := map[string]string{}
		for k, v := range env {
			merged[k] = v
		}
		for k, v := range overrides {
			merged[k] = v
		}
		ep, ok := provider.CodexEndpoint(home, merged, args)
		return providerTarget{endpoint: ep}, ok
	}
	return providerTarget{}, false
}

// modelsEvery is how often the provider's target is looked at again (the
// list itself is asked for at most every 45 minutes, or when asked).
var modelsEvery = time.Minute

// startModels lists the models of the agent's provider on this computer,
// with its key, and reports them with the runtime; dir holds the cache.
func (r *runtimeReporter) startModels(ctx context.Context, dir string, target func(providerChoice) (providerTarget, bool), current func() providerChoice) {
	if r == nil {
		return
	}
	cache := &provider.ModelCache{Dir: filepath.Join(dir, "state", "models")}
	r.mu.Lock()
	r.refresh = make(chan struct{}, 1)
	refresh := r.refresh
	r.mu.Unlock()
	go func() {
		tick := time.NewTicker(modelsEvery)
		defer tick.Stop()
		force := false
		for {
			t, ok := target(current())
			var list *provider.ModelList
			if ok {
				if l, found := cache.Lookup(ctx, t.endpoint, force); found {
					list = &l
				}
				r.pollRefresh(ctx)
			}
			if ctx.Err() != nil {
				return
			}
			force = false
			r.mu.Lock()
			changed := !reflect.DeepEqual(r.provModels, list) || !reflect.DeepEqual(r.target, t)
			r.provModels, r.target, r.hasTarget = list, t, ok
			r.mu.Unlock()
			if changed {
				r.flushLater()
			}
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			case <-refresh:
				force = true
			}
		}
	}()
}

// pollRefresh reads the App's request to list models again (the hub has
// no way to call a runner: it is read once a minute, for agents on another
// vendor only).
func (r *runtimeReporter) pollRefresh(ctx context.Context) {
	if r.c == nil {
		return
	}
	c, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if me, err := r.c.Me(c); err == nil && me.RunSettings != nil {
		r.ModelsRefresh(me.RunSettings.ModelsRefresh)
	}
}

// ModelsRefresh hears the run settings' models_refresh: a new value asks
// for the list at once. The first value seen is where it stands.
func (r *runtimeReporter) ModelsRefresh(v string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.refreshSeen {
		r.refreshSeen, r.refreshLast = true, v
		return
	}
	if v == r.refreshLast {
		return
	}
	r.refreshLast = v
	if r.refresh != nil {
		select {
		case r.refresh <- struct{}{}:
		default:
		}
	}
}

// providerModelsLocked is the provider's list for the runtime report
// (r.mu held); false when there is none.
func (r *runtimeReporter) providerModelsLocked() ([]api.RuntimeModel, string, *time.Time, bool) {
	if r.provModels == nil || len(r.provModels.Models) == 0 {
		return nil, "", nil, false
	}
	out := make([]api.RuntimeModel, 0, len(r.provModels.Models))
	for _, m := range r.provModels.Models {
		out = append(out, api.RuntimeModel{ID: m.ID, Name: m.Name})
	}
	var at *time.Time
	if !r.provModels.At.IsZero() {
		t := r.provModels.At
		at = &t
	}
	return out, r.provModels.From, at, true
}

// claudeAliases are Claude Code's own model names: on another vendor its
// tier variables say what they mean, so they are never set to them.
func claudeAlias(m string) bool {
	m = strings.ToLower(strings.TrimSpace(m))
	switch m {
	case "default", "opus", "sonnet", "haiku", "opusplan", "best":
		return true
	}
	return strings.HasPrefix(m, "claude-") || strings.HasPrefix(m, "opus[") || strings.HasPrefix(m, "sonnet[")
}

// ModelEnv is what a Claude Code run on another vendor adds: its opus and
// sonnet tiers are the model it runs (subagents ask for them by name), its
// haiku tier and background work the vendor's small model (or the same).
// Variables the person set stay theirs. Nothing for Codex or Anthropic.
func (r *runtimeReporter) ModelEnv(model string) map[string]string {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	t := r.target
	if !r.hasTarget || !t.claude {
		return nil
	}
	if model == "" {
		model = orStr(r.cfg, r.choice.model)
	}
	var list []provider.Model
	if r.provModels != nil {
		list = r.provModels.Models
	}
	vars := map[string]string{}
	if model != "" && !claudeAlias(model) {
		vars["ANTHROPIC_DEFAULT_OPUS_MODEL"], vars["ANTHROPIC_DEFAULT_SONNET_MODEL"] = model, model
	}
	if model == "" || claudeAlias(model) {
		model = ""
	}
	if small := provider.SmallModel(t.endpoint, list, model); small != "" && !claudeAlias(small) {
		vars["ANTHROPIC_DEFAULT_HAIKU_MODEL"], vars["ANTHROPIC_SMALL_FAST_MODEL"] = small, small
	}
	for k := range t.set {
		delete(vars, k)
	}
	return vars
}
