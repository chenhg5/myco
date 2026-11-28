// Package quota collects account limits without making model requests.
package quota

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Input contains only the current agent's effective provider. Secrets remain
// local and are never serialized, logged, or sent to a different provider.
type Input struct {
	CLI, Binary, Home, Model, Host, Workdir string
	Env                                     map[string]string `json:"-"`
	Overrides                               map[string]string `json:"-"`
	Pinned                                  bool
	UnmodeledLaunch                         bool
}

type Source struct {
	Provider, Plan, Ref, Kind, Status string
	dir, key, endpoint, binary        string
	env                               map[string]string
	account, revision                 string
	valid                             func() bool
}

func ref(salt []byte, parts ...string) string {
	h := hmac.New(sha256.New, salt)
	for _, p := range parts {
		h.Write([]byte{0})
		h.Write([]byte(p))
	}
	return "qp_" + hex.EncodeToString(h.Sum(nil))
}

func readJSON(path string, v any) bool {
	b, e := os.ReadFile(path)
	return e == nil && len(b) < 4<<20 && json.Unmarshal(b, v) == nil
}

// Discover follows the effective connection, not the mere presence of keys.
// Salt is a machine-local random secret, never an account credential.
func Discover(in Input, salt []byte) Source {
	if in.UnmodeledLaunch || projectAccountOverride(in) {
		return unsupported(in, salt)
	}
	env := map[string]string{}
	for k, v := range in.Env {
		env[k] = v
	}
	src := Source{Status: "unsupported", binary: in.Binary, env: env}
	dir := ""
	base, key := in.Host, ""
	goSelected := false
	if in.CLI == "claude" {
		dir = env["CLAUDE_CONFIG_DIR"]
		if dir == "" {
			dir = filepath.Join(in.Home, ".claude")
		}
		var cfg struct {
			Env map[string]string `json:"env"`
		}
		if readJSON(filepath.Join(dir, "settings.json"), &cfg) {
			for k, v := range cfg.Env {
				env[k] = v
			}
		}
		for k, v := range in.Overrides {
			env[k] = v
		}
		if env["ANTHROPIC_BASE_URL"] != "" {
			base = env["ANTHROPIC_BASE_URL"]
		}
		key = first(env["ANTHROPIC_AUTH_TOKEN"], env["ANTHROPIC_API_KEY"])
		if base == "" || base == "api.anthropic.com" || host(base) == "api.anthropic.com" {
			src.Provider, src.Plan, src.Kind, src.dir = "claude", "claude_subscription", "cache", dir
			profile := filepath.Join(filepath.Dir(dir), ".claude.json")
			if in.Env["CLAUDE_CONFIG_DIR"] != "" {
				profile = filepath.Join(dir, ".claude.json")
			}
			var data struct {
				OAuthAccount struct {
					ID string `json:"accountUuid"`
				} `json:"oauthAccount"`
			}
			readJSON(profile, &data)
			src.Ref = ref(salt, src.Provider, canonical(dir), data.OAuthAccount.ID)
			src.endpoint = profile
			src.account = data.OAuthAccount.ID
			paths := append([]string{filepath.Join(dir, ".credentials.json")}, keychainPaths(in.Home)...)
			src.revision = credentialRevision(paths...)
			src.Status = "auth_required"
			if key != "" || env["CLAUDE_CODE_OAUTH_TOKEN"] != "" || enabled(env["CLAUDE_CODE_USE_BEDROCK"]) || enabled(env["CLAUDE_CODE_USE_VERTEX"]) || enabled(env["CLAUDE_CODE_USE_FOUNDRY"]) {
				src.Status = "unsupported"
				src.Ref = ref(salt, "claude_api", dir, key, env["CLAUDE_CODE_OAUTH_TOKEN"])
				return src
			}
			if data.OAuthAccount.ID != "" {
				src.Status = "ok"
			}
			return src
		}
	} else if in.CLI == "codex" {
		dir = env["CODEX_HOME"]
		if dir == "" {
			dir = filepath.Join(in.Home, ".codex")
		}
		var cfg struct {
			Provider  string `toml:"model_provider"`
			AuthStore string `toml:"cli_auth_credentials_store"`
			Base      string `toml:"openai_base_url"`
			Providers map[string]struct {
				Base string `toml:"base_url"`
				Key  string `toml:"env_key"`
			} `toml:"model_providers"`
		}
		_, cfgErr := toml.DecodeFile(filepath.Join(dir, "config.toml"), &cfg)
		if !in.Pinned {
			if cfgErr == nil {
				if p, ok := cfg.Providers[cfg.Provider]; ok {
					base = p.Base
					key = env[p.Key]
				} else if cfg.Provider == "" || cfg.Provider == "openai" {
					base = first(env["OPENAI_BASE_URL"], cfg.Base)
				} else {
					base = "unknown"
				}
			}
		}
		if key == "" {
			key = first(env["MYCO_CODEX_API_KEY"], env["OPENAI_API_KEY"])
		}
		if base == "" || base == "api.openai.com" || host(base) == "api.openai.com" {
			src.Provider, src.Plan, src.Kind, src.dir = "codex", "chatgpt_subscription", "native", dir
			var auth struct {
				Mode   string `json:"auth_mode"`
				Key    string `json:"OPENAI_API_KEY"`
				Tokens struct {
					ID     string `json:"account_id"`
					Access string `json:"access_token"`
				} `json:"tokens"`
			}
			raw, revision := codexAuth(dir, in.Home, cfg.AuthStore)
			_ = json.Unmarshal(raw, &auth)
			src.account, src.revision = auth.Tokens.ID, revision
			expected := revision
			src.valid = func() bool {
				_, current := codexAuth(dir, in.Home, cfg.AuthStore)
				return expected != "" && expected == current
			}

			src.Ref = ref(salt, src.Provider, canonical(dir), auth.Tokens.ID)
			src.Status = "ok"
			if auth.Tokens.ID == "" {
				src.Status = "auth_required"
			}
			if cfg.AuthStore != "" && cfg.AuthStore != "file" && cfg.AuthStore != "auto" && cfg.AuthStore != "keyring" {
				src.Status = "unsupported"
			}
			if cfg.AuthStore == "keyring" && revision == "" {
				src.Status = "unsupported"
			}
			if cfg.AuthStore == "keyring" && revision != "" && auth.Tokens.ID != "" {
				src.Status = "ok"
			}
			if key != "" || auth.Mode == "apikey" || auth.Key != "" && auth.Tokens.Access == "" {
				src.Status = "unsupported"
				src.Ref = ref(salt, "codex_api", dir, key, auth.Key)
			}
			return src
		}
	} else if in.CLI == "opencode" {
		key = env["OPENCODE_API_KEY"]
		base = first(env["OPENCODE_BASE_URL"], base)
		model := in.Model
		var cfg struct {
			Model    string `json:"model"`
			Provider map[string]struct {
				Options struct {
					Key  string `json:"apiKey"`
					Base string `json:"baseURL"`
				} `json:"options"`
			} `json:"provider"`
		}
		configDir := env["XDG_CONFIG_HOME"]
		if configDir == "" {
			configDir = filepath.Join(in.Home, ".config")
		}
		configPath := env["OPENCODE_CONFIG"]
		if configPath != "" {
			readJSONC(configPath, &cfg)
		} else if !readJSONC(filepath.Join(configDir, "opencode", "opencode.jsonc"), &cfg) {
			readJSONC(filepath.Join(configDir, "opencode", "opencode.json"), &cfg)
		}
		if model == "" {
			model = cfg.Model
		}
		id, _, _ := strings.Cut(model, "/")
		goSelected = id == "opencode-go"
		if id != "" {
			if p, ok := cfg.Provider[id]; ok {
				base = first(p.Options.Base, base)
				key = first(p.Options.Key, key)
			}
		}
		if id == "opencode-go" && base == "" {
			base = "https://opencode.ai/inference/go"
		}
		if id == "" && base == "" {
			return unsupported(in, salt)
		}
		data := env["XDG_DATA_HOME"]
		if data == "" {
			data = filepath.Join(in.Home, ".local", "share")
		}
		var auth map[string]struct {
			Type string `json:"type"`
			Key  string `json:"key"`
		}
		if key == "" && readJSON(filepath.Join(data, "opencode", "auth.json"), &auth) {
			if a := auth[id]; a.Type == "api" {
				key = a.Key
			}
		}
	}
	if strings.HasPrefix(key, "{env:") && strings.HasSuffix(key, "}") {
		key = env[strings.TrimSuffix(strings.TrimPrefix(key, "{env:"), "}")]
	}
	// Match fixed provider origins only. Never forward a gateway key elsewhere.
	switch host(base) {
	case "opencode.ai":
		u, _ := url.Parse(base)
		if !goSelected && (u == nil || (u.Path != "/inference/go" && !strings.HasPrefix(u.Path, "/inference/go/"))) {
			return unsupported(in, salt)
		}
		src.Provider, src.Plan, src.endpoint = "opencode_go", "go", "https://opencode.ai/inference/go/v1/usage"
	case "open.bigmodel.cn", "api.z.ai", "api.kimi.com", "openrouter.ai":
		provider, endpoint := planEndpoint(base)
		if provider == "" {
			return unsupported(in, salt)
		}
		src.Provider, src.Plan, src.endpoint = provider, "coding_plan", endpoint
		if provider == "openrouter" {
			src.Plan = "api_balance"
		}
	case "api.minimax.io", "www.minimax.io":
		src.Provider, src.Plan, src.endpoint = "minimax", "coding_plan", "https://www.minimax.io/v1/token_plan/remains"
	case "api.minimaxi.com", "api.minimax.cn":
		src.Provider, src.Plan, src.endpoint = "minimax", "coding_plan", "https://api.minimax.cn/v1/token_plan/remains"
	default:
		return unsupported(in, salt)
	}
	src.Kind, src.Status, src.key = "official_api", "ok", key
	if src.Provider == "minimax" && strings.HasPrefix(key, "sk-api-") {
		src.Status = "unsupported"
		src.key = ""
	}
	if key == "" || strings.HasPrefix(key, "{env:") || strings.HasPrefix(key, "{file:") {
		src.Status = "no_credentials"
		src.key = ""
	}
	src.Ref = ref(salt, src.Provider, host(src.endpoint), key)
	return src
}
func unsupported(in Input, salt []byte) Source {
	p := in.CLI
	if p != "claude" && p != "codex" {
		p = "unknown"
	}
	return Source{Provider: p, Status: "unsupported", Ref: ref(salt, in.CLI, in.Host)}
}
func first(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}
func host(s string) string {
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, e := url.Parse(s)
	if e != nil || u.User != nil || u.Port() != "" {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

func enabled(s string) bool { return s != "" && s != "0" && !strings.EqualFold(s, "false") }

// String keeps accidental diagnostic formatting from exposing local secrets.
func (s Source) String() string   { return s.Provider + ":" + s.Ref + ":" + s.Status }
func (s Source) GoString() string { return s.String() }

func canonical(path string) string {
	if p, e := filepath.EvalSymlinks(path); e == nil {
		path = p
	}
	if p, e := filepath.Abs(path); e == nil {
		return p
	}
	return filepath.Clean(path)
}

// Only recognized HTTPS plan paths opt in. A gateway or a general API key
// never gets probed against another provider or another region.
func planEndpoint(base string) (string, string) {
	u, e := url.Parse(base)
	if e != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" {
		return "", ""
	}
	path := strings.TrimSuffix(u.Path, "/")
	switch strings.ToLower(u.Host) {
	case "open.bigmodel.cn", "api.z.ai":
		if path == "/api/anthropic" || path == "/api/anthropic/v1" || path == "/api/coding/paas/v4" {
			return "glm", "https://" + strings.ToLower(u.Host) + "/api/monitor/usage/quota/limit"
		}
	case "api.kimi.com":
		if strings.HasPrefix(path, "/coding/") {
			return "kimi", "https://api.kimi.com/coding/v1/usages"
		}
	case "openrouter.ai":
		if path == "/api/v1" {
			return "openrouter", "https://openrouter.ai/api/v1/key"
		}
	}
	return "", ""
}

func planQuotaEndpoint(provider, endpoint string) bool {
	switch provider {
	case "glm":
		return endpoint == "https://open.bigmodel.cn/api/monitor/usage/quota/limit" || endpoint == "https://api.z.ai/api/monitor/usage/quota/limit"
	case "kimi":
		return endpoint == "https://api.kimi.com/coding/v1/usages"
	case "openrouter":
		return endpoint == "https://openrouter.ai/api/v1/key"
	}
	return false
}
