package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
)

// Model is a model a vendor offers: the id its API takes, and a name.
type Model struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// MaxModels bounds a vendor's list.
const MaxModels = 100

// Endpoint is where an agent's CLI sends its requests when it uses an API
// key with another vendor: the base URL, the key (never printed, logged or
// sent anywhere but Base's host) and whether the CLI speaks Anthropic
// Messages (Claude Code) or the OpenAI API (Codex).
type Endpoint struct {
	Base      string
	Key       string
	Anthropic bool
}

// Host is the endpoint's host, for messages.
func (e Endpoint) Host() string {
	if u, err := url.Parse(e.Base); err == nil {
		return u.Host
	}
	return ""
}

// ref names the endpoint and key in the cache without either: a new key
// is a new list (another account may offer other models).
func (e Endpoint) ref() string {
	k := sha256.Sum256([]byte(e.Key))
	h := sha256.Sum256([]byte(strings.TrimRight(e.Base, "/") + "\x00" + strconv.FormatBool(e.Anthropic) + "\x00" + hex.EncodeToString(k[:])))
	return hex.EncodeToString(h[:12])
}

// ClaudeVars are the variables Claude Code runs with: env (the run's
// environment), then its settings.json env, then overrides (a pinned
// provider's variables). Secrets included: never print them.
func ClaudeVars(home string, env, overrides map[string]string) map[string]string {
	vars := map[string]string{}
	for k, v := range env {
		vars[k] = v
	}
	dir := vars["CLAUDE_CONFIG_DIR"]
	if dir == "" {
		dir = filepath.Join(home, ".claude")
	}
	if b, err := os.ReadFile(filepath.Join(dir, "settings.json")); err == nil && len(b) < 4<<20 {
		var s struct {
			Env map[string]any `json:"env"`
		}
		if json.Unmarshal(b, &s) == nil {
			for k, v := range s.Env {
				if t, ok := v.(string); ok {
					vars[k] = t
				}
			}
		}
	}
	for k, v := range overrides {
		vars[k] = v
	}
	return vars
}

// ClaudeEndpoint is the endpoint Claude Code runs against (ClaudeVars);
// false for Anthropic itself, cloud platforms and logins, whose models
// Claude Code knows.
func ClaudeEndpoint(home string, env, overrides map[string]string) (Endpoint, bool) {
	return ClaudeEndpointOf(ClaudeVars(home, env, overrides))
}

// ClaudeEndpointOf is ClaudeEndpoint for variables already gathered.
func ClaudeEndpointOf(vars map[string]string) (Endpoint, bool) {
	for _, k := range []string{"CLAUDE_CODE_USE_BEDROCK", "CLAUDE_CODE_USE_VERTEX", "CLAUDE_CODE_USE_FOUNDRY"} {
		if v := strings.ToLower(strings.TrimSpace(vars[k])); v != "" && v != "0" && v != "false" {
			return Endpoint{}, false
		}
	}
	base := strings.TrimSpace(vars["ANTHROPIC_BASE_URL"])
	if !thirdParty(base, "api.anthropic.com") {
		return Endpoint{}, false
	}
	key := strings.TrimSpace(vars["ANTHROPIC_AUTH_TOKEN"])
	if key == "" {
		key = strings.TrimSpace(vars["ANTHROPIC_API_KEY"])
	}
	return Endpoint{Base: base, Key: key, Anthropic: true}, true
}

var (
	codexProviderArg = regexp.MustCompile(`^model_provider\s*=\s*"?([^"]*)"?$`)
	codexTableArg    = regexp.MustCompile(`^model_providers\.([A-Za-z0-9_-]+)\s*=\s*\{(.*)\}$`)
	codexField       = regexp.MustCompile(`(base_url|env_key)\s*=\s*"([^"]*)"`)
)

// CodexEndpoint is the endpoint Codex runs against: a pinned provider's
// -c arguments beat CODEX_HOME's config.toml; false for OpenAI itself and
// ChatGPT logins, whose models Codex lists on its own.
func CodexEndpoint(home string, env map[string]string, configArgs []string) (Endpoint, bool) {
	type table struct{ base, envKey string }
	name, tables := "", map[string]table{}
	for i := 0; i+1 < len(configArgs); i++ {
		if configArgs[i] != "-c" {
			continue
		}
		arg := strings.TrimSpace(configArgs[i+1])
		if m := codexProviderArg.FindStringSubmatch(arg); m != nil {
			name = m[1]
		} else if m := codexTableArg.FindStringSubmatch(arg); m != nil {
			var t table
			for _, f := range codexField.FindAllStringSubmatch(m[2], -1) {
				if f[1] == "base_url" {
					t.base = f[2]
				} else {
					t.envKey = f[2]
				}
			}
			tables[m[1]] = t
		}
	}
	dir := env["CODEX_HOME"]
	if dir == "" {
		dir = filepath.Join(home, ".codex")
	}
	var cfg struct {
		Provider  string `toml:"model_provider"`
		Base      string `toml:"openai_base_url"`
		Providers map[string]struct {
			Base string `toml:"base_url"`
			Key  string `toml:"env_key"`
		} `toml:"model_providers"`
	}
	_, _ = toml.DecodeFile(filepath.Join(dir, "config.toml"), &cfg)
	if name == "" {
		name = cfg.Provider
	}
	var base, key string
	if t, ok := tables[name]; ok {
		base, key = t.base, env[t.envKey]
	} else if p, ok := cfg.Providers[name]; ok && name != "" {
		base, key = p.Base, env[p.Key]
	} else if name == "" || name == "openai" {
		base = env["OPENAI_BASE_URL"]
		if base == "" {
			base = cfg.Base
		}
		key = env["OPENAI_API_KEY"]
	} else {
		return Endpoint{}, false // a provider Codex has built in (ollama, ...)
	}
	if key == "" {
		key = env[CodexKeyEnv]
	}
	if !thirdParty(base, "api.openai.com", "chatgpt.com") {
		return Endpoint{}, false
	}
	return Endpoint{Base: strings.TrimSpace(base), Key: strings.TrimSpace(key)}, true
}

// thirdParty: base is set, a URL, and not one of the CLI's own hosts.
func thirdParty(base string, own ...string) bool {
	u, err := url.Parse(base)
	if base == "" || err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, o := range own {
		if host == o || strings.HasSuffix(host, "."+o) {
			return false
		}
	}
	return true
}

var versionSegment = regexp.MustCompile(`/v\d+$`)

// modelURLs are where to ask for the endpoint's models: next to the base
// the CLI uses first, then the vendor's other interface on the same host
// (the same key often works on both, while only one lists models).
func modelURLs(e Endpoint) []string {
	base := strings.TrimRight(strings.TrimSpace(e.Base), "/")
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return nil
	}
	u.RawQuery, u.Fragment = "", ""
	base = u.String()
	var out []string
	add := func(s string) {
		for _, x := range out {
			if x == s {
				return
			}
		}
		out = append(out, s)
	}
	if versionSegment.MatchString(u.Path) {
		add(base + "/models")
	} else if e.Anthropic {
		add(base + "/v1/models")
	} else {
		add(base + "/models")
		add(base + "/v1/models")
	}
	if v, ok := VendorOf(base); ok {
		origin := u.Scheme + "://" + u.Host
		anthropic, openai := origin+v.Anthropic+"/v1/models", origin+v.OpenAI+"/models"
		if e.Anthropic {
			add(anthropic)
			add(openai)
		} else {
			add(openai)
			add(anthropic)
		}
	}
	return out
}

// errNoModels: the vendor answered with no models.
var errNoModels = errors.New("no models")

// FetchModels asks the endpoint's vendor for its models, with its key, on
// this computer. Errors name the host and status only.
func FetchModels(ctx context.Context, client *http.Client, e Endpoint) ([]Model, error) {
	if client == nil {
		client = &http.Client{}
	}
	c := *client
	if c.Timeout == 0 {
		c.Timeout = 8 * time.Second
	}
	// The key goes to the endpoint's host only: no redirect elsewhere.
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 || req.URL.Host != via[0].URL.Host || req.URL.Scheme != via[0].URL.Scheme {
			return http.ErrUseLastResponse
		}
		return nil
	}
	var last error = errNoModels
	for _, u := range modelURLs(e) {
		ms, err := fetchList(ctx, &c, u, e.Key)
		if err == nil && len(ms) > 0 {
			return ms, nil
		}
		if err != nil {
			last = err
		}
		if ctx.Err() != nil {
			break
		}
	}
	return nil, last
}

// authStyles: both headers first (most vendors take either); a vendor that
// refuses two is asked with each alone.
var authStyles = []string{"both", "bearer", "x-api-key"}

func fetchList(ctx context.Context, c *http.Client, list, key string) ([]Model, error) {
	var out []Model
	seen := map[string]bool{}
	next := list
	for page := 0; page < 5 && next != ""; page++ {
		var body []byte
		var err error
		for i, style := range authStyles {
			var status int
			body, status, err = get(ctx, c, next, key, style)
			if err == nil || key == "" || (status != 400 && status != 401 && status != 403) || i == len(authStyles)-1 {
				break
			}
		}
		if err != nil {
			return nil, err
		}
		var p struct {
			Data    []entry `json:"data"`
			Models  []entry `json:"models"`
			HasMore bool    `json:"has_more"`
			LastID  string  `json:"last_id"`
		}
		if json.Unmarshal(body, &p) != nil {
			return nil, fmt.Errorf("models from %s: not a model list", hostOf(list))
		}
		for _, x := range append(p.Data, p.Models...) {
			if m, ok := x.model(); ok && !seen[m.ID] && len(out) < MaxModels {
				seen[m.ID] = true
				out = append(out, m)
			}
		}
		next = ""
		if p.HasMore && p.LastID != "" && len(out) < MaxModels {
			u, _ := url.Parse(list)
			q := u.Query()
			q.Set("after_id", p.LastID)
			u.RawQuery = q.Encode()
			next = u.String()
		}
	}
	return out, nil
}

func get(ctx context.Context, c *http.Client, u, key, style string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("models: bad endpoint")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "myco")
	req.Header.Set("anthropic-version", "2023-06-01")
	if key != "" {
		if style != "x-api-key" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		if style != "bearer" {
			req.Header.Set("x-api-key", key)
		}
	}
	res, err := c.Do(req)
	if err != nil {
		// The error text of a failed request can hold the URL; say only where.
		if ctx.Err() != nil {
			return nil, 0, ctx.Err()
		}
		return nil, 0, fmt.Errorf("models from %s: unreachable", hostOf(u))
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))
		return nil, res.StatusCode, fmt.Errorf("models from %s: HTTP %d", hostOf(u), res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 1<<20+1))
	if err != nil {
		return nil, res.StatusCode, fmt.Errorf("models from %s: cut off", hostOf(u))
	}
	if len(b) > 1<<20 {
		return nil, res.StatusCode, fmt.Errorf("models from %s: list too large", hostOf(u))
	}
	return b, res.StatusCode, nil
}

type entry struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Type        string `json:"type"`
}

// model: chat models only (not embeddings, images, speech...).
func (x entry) model() (Model, bool) {
	id := strings.TrimSpace(x.ID)
	if id == "" || len(id) > 80 || strings.ContainsAny(id, " \t\r\n\"'`") {
		return Model{}, false
	}
	switch strings.ToLower(x.Type) {
	case "", "chat", "model", "text", "language", "llm":
	default:
		return Model{}, false
	}
	if l := strings.ToLower(id); strings.Contains(l, "embedding") || strings.Contains(l, "rerank") {
		return Model{}, false
	}
	name := strings.TrimSpace(x.DisplayName)
	if name == "" {
		name = strings.TrimSpace(x.Name)
	}
	if len(name) > 80 || name == id {
		name = ""
	}
	return Model{ID: id, Name: name}, true
}

// ModelList is an endpoint's models and where they came from: "provider" (its
// vendor said so, at At) or "catalog" (the built-in table: it could not be
// asked yet).
type ModelList struct {
	Models []Model
	From   string
	At     time.Time
}

// ModelCache keeps each endpoint's list on this computer (in Dir, without
// keys), asks the vendor again after TTL, and keeps the last list when
// asking fails.
type ModelCache struct {
	Dir    string
	TTL    time.Duration
	Client *http.Client
	Now    func() time.Time

	mu     sync.Mutex
	failed map[string]time.Time
}

// retryFailed: after a failure the vendor is not asked again for this
// long, unless the person asks.
const retryFailed = 5 * time.Minute

type cached struct {
	At     time.Time `json:"at"`
	Models []Model   `json:"models"`
}

func (c *ModelCache) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// Lookup is the endpoint's list: from the cache while fresh, else asked
// again (force asks at once). Falls back to the last list, then to the
// catalogue; false when there is nothing to offer.
func (c *ModelCache) Lookup(ctx context.Context, e Endpoint, force bool) (ModelList, bool) {
	ref := e.ref()
	path := filepath.Join(c.Dir, ref+".json")
	var old cached
	if b, err := os.ReadFile(path); err == nil && len(b) < 1<<20 {
		_ = json.Unmarshal(b, &old)
	}
	now := c.now()
	ttl := c.TTL
	if ttl == 0 {
		ttl = 45 * time.Minute
	}
	c.mu.Lock()
	failedAt := c.failed[ref]
	c.mu.Unlock()
	fresh := len(old.Models) > 0 && now.Sub(old.At) < ttl && !old.At.After(now)
	if !force && (fresh || now.Sub(failedAt) < retryFailed) {
		return c.fallback(e, old)
	}
	ms, err := FetchModels(ctx, c.Client, e)
	if err != nil {
		c.mu.Lock()
		if c.failed == nil {
			c.failed = map[string]time.Time{}
		}
		c.failed[ref] = now
		c.mu.Unlock()
		return c.fallback(e, old)
	}
	c.mu.Lock()
	delete(c.failed, ref)
	c.mu.Unlock()
	keep := cached{At: now.UTC(), Models: ms}
	if b, err := json.Marshal(keep); err == nil && os.MkdirAll(c.Dir, 0o700) == nil {
		tmp := path + ".tmp"
		if os.WriteFile(tmp, b, 0o600) == nil {
			_ = os.Rename(tmp, path)
		}
	}
	return ModelList{Models: ms, From: "provider", At: keep.At}, true
}

func (c *ModelCache) fallback(e Endpoint, old cached) (ModelList, bool) {
	if len(old.Models) > 0 {
		return ModelList{Models: old.Models, From: "provider", At: old.At}, true
	}
	if v, ok := VendorOf(e.Base); ok {
		var ms []Model
		for _, id := range v.Models {
			ms = append(ms, Model{ID: id})
		}
		return ModelList{Models: ms, From: "catalog"}, true
	}
	return ModelList{}, false
}

// SmallModel is the model for Claude Code's background work on e when the
// agent runs main: the vendor's small model when its list has it (or
// nothing could be listed), else main itself.
func SmallModel(e Endpoint, list []Model, main string) string {
	v, ok := VendorOf(e.Base)
	if !ok || v.Small == "" {
		return main
	}
	if len(list) == 0 {
		return v.Small
	}
	for _, m := range list {
		if m.ID == v.Small {
			return v.Small
		}
	}
	return main
}
