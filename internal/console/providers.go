package console

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/config"
	"github.com/chenhg5/myco/internal/provider"
)

// ProviderList answers GET /app/local/providers: cc-switch's Claude
// providers on this computer (names and hosts only, never keys).
type ProviderList struct {
	CCSwitch  bool                `json:"cc_switch"`
	Providers []provider.Provider `json:"providers"`
	Error     string              `json:"error,omitempty"`
}

// AgentProvider is a local agent's provider setting
// (GET/PUT /app/local/agents/{name}/provider).
type AgentProvider struct {
	// Local: the agent has a profile on this computer, so it can be set here.
	Local bool `json:"local"`
	// Provider: "" (follow this computer), "cc-switch:<id>", or "env_file"
	// (provider variables in the agent's env file, e.g. entered by hand).
	Provider string `json:"provider"`
	// Harness of the agent's runner; only claude can be pinned for now.
	Harness string `json:"harness,omitempty"`
	// EnvHost is the endpoint host in the env file, when set by hand.
	EnvHost string `json:"env_host,omitempty"`
}

// ProviderUpdate is the body of PUT /app/local/agents/{name}/provider:
// either Provider ("" or "cc-switch:<id>"), or Manual.
type ProviderUpdate struct {
	Provider string          `json:"provider"`
	Manual   *ManualProvider `json:"manual,omitempty"`
}

// ManualProvider is an Anthropic-compatible endpoint entered by hand.
type ManualProvider struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	Model   string `json:"model,omitempty"`
}

func (l *Local) defaultConfig() string { return filepath.Join(l.Home, ".myco", "config.yaml") }

func (b *Backend) listProviders(w http.ResponseWriter, r *http.Request) {
	ps, err := provider.List(provider.DirIn(b.Local.Home), "claude")
	switch {
	case errors.Is(err, provider.ErrNoCCSwitch):
		writeJSON(w, http.StatusOK, ProviderList{Providers: []provider.Provider{}})
	case err != nil:
		writeJSON(w, http.StatusOK, ProviderList{CCSwitch: true, Providers: []provider.Provider{}, Error: err.Error()})
	default:
		if ps == nil {
			ps = []provider.Provider{}
		}
		writeJSON(w, http.StatusOK, ProviderList{CCSwitch: true, Providers: ps})
	}
}

// localProfile finds this computer's profile of agent (preferring the one
// for hub, when several hubs have an agent of that name).
func (l *Local) localProfile(agent, hub string) (config.Profile, bool) {
	ps, _ := config.ListProfiles(config.ProfilesDir(l.defaultConfig()))
	var found []config.Profile
	for _, p := range ps {
		if p.Config.Agent == agent {
			found = append(found, p)
		}
	}
	sort.SliceStable(found, func(i, j int) bool {
		return sameHub(found[i].Config.Hub, hub) && !sameHub(found[j].Config.Hub, hub)
	})
	if len(found) == 0 {
		return config.Profile{}, false
	}
	return found[0], true
}

func sameHub(a, b string) bool {
	return b != "" && strings.TrimRight(a, "/") == strings.TrimRight(b, "/")
}

func (b *Backend) agentProvider(p config.Profile) AgentProvider {
	cfg := p.Config
	out := AgentProvider{Local: true, Provider: cfg.Runner.Provider, Harness: cfg.Runner.Harness}
	if out.Harness == "" {
		switch cfg.Harness {
		case "codex", "exec", "machine":
			out.Harness = cfg.Harness
		default:
			out.Harness = "claude"
		}
	}
	if out.Provider == "" && cfg.Runner.EnvFile != "" {
		if vals, err := config.ReadEnvFile(cfg.Runner.EnvFile); err == nil {
			for _, kv := range vals {
				if v, ok := strings.CutPrefix(kv, "ANTHROPIC_BASE_URL="); ok {
					out.Provider, out.EnvHost = "env_file", hostOf(v)
				}
			}
		}
	}
	return out
}

// hubURL is the URL of the hub given as ?hub_id= (or ?hub=, a URL).
func (b *Backend) hubURL(r *http.Request) string {
	if id := r.URL.Query().Get("hub_id"); id != "" && b.Store != nil {
		if hubs, err := b.Store.List(); err == nil {
			for _, h := range hubs {
				if h.ID == id {
					return h.URL
				}
			}
		}
	}
	return r.URL.Query().Get("hub")
}

func (b *Backend) getAgentProvider(w http.ResponseWriter, r *http.Request) {
	p, ok := b.Local.localProfile(r.PathValue("name"), b.hubURL(r))
	if !ok {
		writeJSON(w, http.StatusOK, AgentProvider{})
		return
	}
	writeJSON(w, http.StatusOK, b.agentProvider(p))
}

func (b *Backend) putAgentProvider(w http.ResponseWriter, r *http.Request) {
	var in ProviderUpdate
	if !decode(w, r, &in) {
		return
	}
	p, ok := b.Local.localProfile(r.PathValue("name"), b.hubURL(r))
	if !ok {
		writeErr(w, &client.Error{Status: http.StatusNotFound, Code: "not_found", Message: "this agent does not run on this computer"})
		return
	}
	cfg := p.Config
	if cur := b.agentProvider(p); cur.Harness != "claude" {
		writeErr(w, &client.Error{Status: http.StatusBadRequest, Code: "invalid_argument", Message: "only Claude Code agents can pick a provider for now"})
		return
	}
	switch {
	case in.Manual != nil:
		m := in.Manual
		m.BaseURL, m.APIKey, m.Model = strings.TrimSpace(m.BaseURL), strings.TrimSpace(m.APIKey), strings.TrimSpace(m.Model)
		if !strings.HasPrefix(m.BaseURL, "https://") && !strings.HasPrefix(m.BaseURL, "http://") || m.APIKey == "" {
			writeErr(w, &client.Error{Status: http.StatusBadRequest, Code: "invalid_argument", Message: "enter the endpoint URL (https://…) and the key"})
			return
		}
		file := cfg.Runner.EnvFile
		if file == "" {
			file = filepath.Join(b.Local.Home, ".myco", "env", cfg.Agent+".env")
		}
		vars := map[string]string{"ANTHROPIC_BASE_URL": m.BaseURL, "ANTHROPIC_AUTH_TOKEN": m.APIKey}
		if m.Model != "" {
			vars["ANTHROPIC_MODEL"] = m.Model
		}
		if err := writeProviderEnv(file, cfg.Agent, vars); err != nil {
			writeErr(w, err)
			return
		}
		cfg.Runner.EnvFile, cfg.Runner.Provider = file, ""
	case in.Provider == "":
		cfg.Runner.Provider = ""
		// "Follow this computer" also drops hand-entered provider variables.
		if cfg.Runner.EnvFile != "" {
			if err := writeProviderEnv(cfg.Runner.EnvFile, cfg.Agent, nil); err != nil {
				writeErr(w, err)
				return
			}
		}
	default:
		pr, err := provider.Resolve(provider.DirIn(b.Local.Home), in.Provider, "claude")
		if err != nil {
			writeErr(w, &client.Error{Status: http.StatusBadRequest, Code: "invalid_argument", Message: err.Error()})
			return
		}
		cfg.Runner.Provider = pr.Ref
	}
	if err := config.Save(p.Path, cfg); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, b.agentProvider(config.Profile{Path: p.Path, Config: cfg}))
}

// writeProviderEnv replaces the provider variables in an agent's env file
// with vars (nil = remove them), keeping its other lines; mode 600.
func writeProviderEnv(path, agent string, vars map[string]string) error {
	var keep []string
	if b, err := os.ReadFile(path); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			k, _, _ := strings.Cut(strings.TrimPrefix(strings.TrimSpace(line), "export "), "=")
			if strings.TrimSpace(line) == "" || isProviderVar(strings.TrimSpace(k)) {
				continue
			}
			keep = append(keep, line)
		}
	} else if len(vars) == 0 {
		return nil
	}
	if len(keep) == 0 {
		keep = []string{"# passed only to " + agent + "'s CLI (set in the myco app)"}
	}
	names := make([]string, 0, len(vars))
	for k := range vars {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		keep = append(keep, k+"="+quoteEnvValue(vars[k]))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(strings.Join(keep, "\n")+"\n"), 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func isProviderVar(k string) bool {
	return strings.HasPrefix(k, "ANTHROPIC_") || strings.HasPrefix(k, "CLAUDE_CODE_USE_") || k == "CLAUDE_CODE_OAUTH_TOKEN"
}

func quoteEnvValue(v string) string {
	if strings.ContainsAny(v, " \t#\"'") {
		return `"` + strings.ReplaceAll(v, `"`, `\"`) + `"`
	}
	return v
}
