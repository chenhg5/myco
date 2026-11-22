package console

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
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
	// Harness of the agent's runner; Claude Code and Codex support pins.
	Harness string `json:"harness,omitempty"`
	// EnvHost is the endpoint host in the env file, when set by hand.
	EnvHost string              `json:"env_host,omitempty"`
	Manual  *ManualProviderInfo `json:"manual,omitempty"`
}

// ProviderUpdate is the body of PUT /app/local/agents/{name}/provider:
// either Provider ("" or "cc-switch:<id>"), or Manual.
type ProviderUpdate struct {
	Provider string          `json:"provider"`
	Manual   *ManualProvider `json:"manual,omitempty"`
}

// ManualProvider is accepted only by the local App backend.
type ManualProvider = provider.Manual

type ManualProviderInfo struct {
	BaseURL string `json:"base_url"`
	Model   string `json:"model,omitempty"`
	WireAPI string `json:"wire_api,omitempty"`
	HasKey  bool   `json:"has_key"`
}

func (l *Local) defaultConfig() string { return filepath.Join(l.Home, ".myco", "config.yaml") }

func (b *Backend) listProviders(w http.ResponseWriter, r *http.Request) {
	app := r.URL.Query().Get("app")
	if app != "codex" {
		app = "claude"
	}
	ps, err := provider.List(provider.DirIn(b.Local.Home), app)
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
			vars := map[string]string{}
			for _, kv := range vals {
				k, v, _ := strings.Cut(kv, "=")
				vars[k] = v
			}
			base, key, model := vars["ANTHROPIC_BASE_URL"], vars["ANTHROPIC_AUTH_TOKEN"], vars["ANTHROPIC_MODEL"]
			if key == "" {
				key = vars["ANTHROPIC_API_KEY"]
			}
			wire := ""
			if out.Harness == "codex" {
				base, key, model, wire = vars["OPENAI_BASE_URL"], vars["OPENAI_API_KEY"], vars["OPENAI_MODEL"], vars["OPENAI_WIRE_API"]
				if wire == "" {
					wire = "responses"
				}
			}
			if base != "" {
				out.Provider, out.EnvHost = "env_file", hostOf(base)
				// Never echo URL credentials or query values from older env captures.
				m := provider.Manual{BaseURL: base, APIKey: "placeholder", Model: model, WireAPI: wire}
				if m.Validate(out.Harness) == nil {
					out.Manual = &ManualProviderInfo{BaseURL: base, Model: model, WireAPI: wire, HasKey: key != ""}
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
	cur := b.agentProvider(p)
	if cur.Harness != "claude" && cur.Harness != "codex" {
		writeErr(w, &client.Error{Status: http.StatusBadRequest, Code: "invalid_argument", Message: "only Claude Code and Codex agents can pick a provider"})
		return
	}
	switch {
	case in.Manual != nil:
		m := in.Manual
		if err := m.Validate(cur.Harness); err != nil {
			writeErr(w, &client.Error{Status: http.StatusBadRequest, Code: "invalid_argument", Message: err.Error()})
			return
		}
		file := cfg.Runner.EnvFile
		if file == "" {
			file = filepath.Join(b.Local.Home, ".myco", "env", fmt.Sprintf("provider-%x.env", sha256.Sum256([]byte(p.Path))))
		}
		vars := m.Variables(cur.Harness)
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
		pr, err := provider.Resolve(provider.DirIn(b.Local.Home), in.Provider, cur.Harness)
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
	return provider.WriteEnv(path, agent, vars)
}

func isProviderVar(k string) bool { return provider.IsProviderVar(k) }
