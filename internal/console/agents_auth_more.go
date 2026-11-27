package console

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

// All results are configuration evidence, never remote credential validation.
// In particular, absent or inaccessible secure stores remain unknown.
func (l *Local) moreCLISignedIn(ctx context.Context, cli string) bool {
	switch cli {
	case "atomcode":
		return l.atomcodeSignedIn()
	case "kimi":
		return l.kimiSignedIn()
	case "copilot":
		return l.copilotSignedIn()
	case "auggie":
		return l.auggieSignedIn()
	case "kilo":
		return l.kiloSignedIn(ctx)
	case "cline":
		return l.clineSignedIn()
	case "grok":
		return l.grokSignedIn()
	case "antigravity":
		if l.hasAuthEnv("AGY_PATH") {
			return false
		}
		settings := authObject(filepath.Join(l.Home, ".gemini", "antigravity-cli", "settings.json"))
		return authText(settings["modelProvider"]) == "gemini" && l.hasAuthEnv("GEMINI_API_KEY")
	case "goose":
		return l.gooseSignedIn()
	}
	return false
}

func authText(raw json.RawMessage) string {
	var s string
	_ = json.Unmarshal(raw, &s)
	return strings.TrimSpace(s)
}
func (l *Local) hasAuthEnv(key string) bool { return strings.TrimSpace(l.userGetenv(key)) != "" }

// Unlike credentialPresent, these formats require literal strings, not an
// arbitrary object that happens to contain metadata.
func authStrings(record map[string]json.RawMessage, keys ...string) bool {
	for _, key := range keys {
		if authText(record[key]) != "" {
			return true
		}
	}
	return false
}

func authConfig(path, format string) map[string]any {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil || len(b) > 1<<20 {
		return nil
	}
	var v map[string]any
	switch format {
	case "toml":
		err = toml.Unmarshal(b, &v)
	case "yaml":
		err = yaml.Unmarshal(b, &v)
	}
	if err != nil {
		return nil
	}
	return v
}
func configText(v any) string        { s, _ := v.(string); return strings.TrimSpace(s) }
func configMap(v any) map[string]any { m, _ := v.(map[string]any); return m }

// AtomCode's npm launcher delegates to the Rust CLI. Its OAuth store is
// auth.toml under ATOMCODE_HOME (default ~/.atomcode), not auth.json. Only
// literal token fields are evidence; account metadata alone is insufficient.
// This never runs login, refreshes credentials, or validates them remotely.
func (l *Local) atomcodeSignedIn() bool {
	auth := authConfig(filepath.Join(l.authDir("ATOMCODE_HOME", ".atomcode"), "auth.toml"), "toml")
	return configText(auth["access_token"]) != "" || configText(auth["refresh_token"]) != ""
}

func (l *Local) kimiSignedIn() bool {
	home := l.authDir("KIMI_CODE_HOME", ".kimi-code")
	dir := filepath.Join(home, "credentials")
	files, _ := os.ReadDir(dir)
	for _, file := range files {
		if !strings.HasPrefix(file.Name(), "kimi-code") || !strings.HasSuffix(file.Name(), ".json") || file.IsDir() {
			continue
		}
		if authStrings(authObject(filepath.Join(dir, file.Name())), "access_token", "refresh_token") {
			return true
		}
	}
	cfg := authConfig(filepath.Join(home, "config.toml"), "toml")
	for _, raw := range configMap(cfg["providers"]) {
		provider := configMap(raw)
		if provider["oauth"] != nil {
			continue
		}
		if configText(provider["api_key"]) != "" {
			return true
		}
		if key := configText(provider["api_key_env"]); key != "" && l.hasAuthEnv(key) {
			return true
		}
	}
	return false
}

func (l *Local) copilotSignedIn() bool {
	if l.hasAuthEnv("COPILOT_PROVIDER_BASE_URL") && l.hasAuthEnv("COPILOT_PROVIDER_API_KEY") {
		return true
	}
	// Respect the CLI's environment precedence; classic PATs are unsupported.
	for _, key := range []string{"COPILOT_GITHUB_TOKEN", "GH_TOKEN", "GITHUB_TOKEN"} {
		if token := strings.TrimSpace(l.userGetenv(key)); token != "" {
			return strings.HasPrefix(token, "github_pat_") || strings.HasPrefix(token, "gho_") || strings.HasPrefix(token, "ghu_")
		}
	}
	// OAuth accounts in config.json can be metadata for a keyring entry. There
	// is no verified public fallback schema, so do not infer a token from them.
	return false
}

func (l *Local) auggieSignedIn() bool {
	valid := func(r map[string]json.RawMessage) bool {
		return authStrings(r, "accessToken") && authStrings(r, "tenantURL")
	}
	if raw := l.userGetenv("AUGMENT_SESSION_AUTH"); raw != "" {
		var r map[string]json.RawMessage
		if json.Unmarshal([]byte(raw), &r) == nil && valid(r) {
			return true
		}
	}
	// AUGMENT_CODING_AGENT_DIR relocates agent settings, not this session store.
	dir := filepath.Join(l.Home, ".augment")
	if override := strings.TrimSpace(l.userGetenv("AUGMENT_CACHE_DIR")); override != "" {
		// Expand the CLI's literal ~/ syntax before cleaning native separators.
		dir = filepath.Clean(expandAuthHome(override, l.Home))
	}
	if valid(authObject(filepath.Join(dir, "session.json"))) {
		return true
	}
	return l.hasAuthEnv("AUGMENT_API_TOKEN") && l.hasAuthEnv("AUGMENT_API_URL")
}
func expandAuthHome(path, home string) string {
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}

func (l *Local) clineSignedIn() bool {
	root := l.authDir("CLINE_DIR", ".cline")
	data := filepath.Join(root, "data")
	if override := strings.TrimSpace(l.userGetenv("CLINE_DATA_DIR")); override != "" {
		data = override
	}
	path := filepath.Join(data, "settings", "providers.json")
	if override := strings.TrimSpace(l.userGetenv("CLINE_PROVIDER_SETTINGS_PATH")); override != "" {
		path = override
	}
	record := authObject(path)
	var version int
	if json.Unmarshal(record["version"], &version) != nil || version != 1 {
		return false
	}
	for _, raw := range authFields(record["providers"]) {
		settings := authFields(authFields(raw)["settings"])
		if authStrings(settings, "apiKey") || authStrings(authFields(settings["auth"]), "apiKey", "accessToken", "refreshToken") {
			return true
		}
	}
	// Legacy secrets and cloud helper/profile metadata are deliberately opaque.
	return false
}

func (l *Local) grokSignedIn() bool {
	// An administrator can disable API-key auth. We do not execute external
	// auth-provider commands or attempt the CLI's refresh/login flows.
	cfg := authConfig(filepath.Join(l.authDir("GROK_HOME", ".grok"), "config.toml"), "toml")
	for _, name := range []string{"grok_com_config", "auth"} {
		auth := configMap(cfg[name])
		disabled, _ := auth["disable_api_key_auth"].(bool)
		if disabled || auth["force_login_team_uuid"] != nil || configText(auth["preferred_method"]) == "oidc" {
			return false
		}
	}
	if l.hasAuthEnv("GROK_FORCE_LOGIN_TEAM_ID") || strings.EqualFold(strings.TrimSpace(l.userGetenv("GROK_DISABLE_API_KEY_AUTH")), "true") || l.userGetenv("GROK_DISABLE_API_KEY_AUTH") == "1" {
		return false
	}
	if l.hasAuthEnv("XAI_API_KEY") {
		return true
	}
	path := filepath.Join(l.authDir("GROK_HOME", ".grok"), "auth.json")
	if override := l.userGetenv("GROK_AUTH_PATH"); override != "" {
		path = override
	}
	// The API-key scope is stable. OIDC scopes depend on the configured issuer
	// and team policy; finding an arbitrary cached session is insufficient.
	key := authFields(authObject(path)["xai::api_key"])
	return authText(key["auth_mode"]) == "api_key" && authStrings(key, "key")
}

func (l *Local) gooseSignedIn() bool {
	if l.hasAuthEnv("GOOSE_ADDITIONAL_CONFIG_FILES") {
		return false
	}
	dir := gooseAuthDir(l.Home, runtime.GOOS, l.userGetenv)
	if dir == "" {
		return false
	}
	cfg := authConfig(filepath.Join(dir, "config.yaml"), "yaml")
	active := strings.TrimSpace(l.userGetenv("GOOSE_PROVIDER"))
	if active == "" {
		active = configText(cfg["active_provider"])
	}
	entry := configMap(configMap(cfg["providers"])[active])
	enabled, _ := entry["enabled"].(bool)
	configured, _ := entry["configured"].(bool)
	// Goose persists this explicit state after provider setup, including local
	// providers that require no API key. Merely naming a model is insufficient.
	return active != "" && enabled && configured && configText(entry["model"]) != ""
}
func gooseAuthDir(home, platform string, getenv func(string) string) string {
	if root := getenv("GOOSE_PATH_ROOT"); filepath.IsAbs(root) {
		return filepath.Join(root, "config")
	}
	if platform == "windows" {
		base := getenv("APPDATA")
		if base == "" {
			return ""
		}
		return filepath.Join(base, "Block", "goose", "config")
	}
	base := getenv("XDG_CONFIG_HOME")
	if !filepath.IsAbs(base) {
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "goose")
}

func (l *Local) kiloSignedIn(ctx context.Context) bool {
	base := l.userGetenv("XDG_DATA_HOME")
	if base == "" {
		base = filepath.Join(l.Home, ".local", "share")
	}
	data := filepath.Join(strings.ReplaceAll(strings.ReplaceAll(base, "\n", ""), "\r", ""), "kilo")
	path := filepath.Join(data, "kilo.db")
	if override := l.userGetenv("KILO_DB"); override != "" {
		if override == ":memory:" {
			return false
		}
		path = override
		if !filepath.IsAbs(path) {
			path = filepath.Join(data, path)
		}
	}
	if !fileExists(path) {
		return false
	}
	// Open the existing database read-only. Return only EXISTS booleans, never
	// select tokens into Go, migrate its schema, or invoke Kilo itself.
	uriPath := filepath.ToSlash(path)
	// A drive letter without the leading slash becomes a URI authority.
	// SQLite expects file:///C:/... rather than file://C:/... on Windows.
	if len(uriPath) >= 3 && uriPath[1] == ':' && uriPath[2] == '/' {
		uriPath = "/" + uriPath
	}
	u := &url.URL{Scheme: "file", Path: uriPath}
	q := u.Query()
	q.Set("mode", "ro")
	q.Set("_pragma", "query_only(1)")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return false
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	for _, query := range []string{
		`SELECT EXISTS(SELECT 1 FROM account a JOIN account_state s ON a.id=s.active_account_id WHERE length(trim(a.access_token))>0 OR length(trim(a.refresh_token))>0)`,
		`SELECT EXISTS(SELECT 1 FROM credential WHERE active=1 AND length(trim(integration_id))>0 AND connector_id IS NULL AND json_valid(value) AND ((json_extract(value,'$.type')='key' AND length(trim(json_extract(value,'$.key')))>0) OR (json_extract(value,'$.type')='oauth' AND (length(trim(json_extract(value,'$.access')))>0 OR length(trim(json_extract(value,'$.refresh')))>0))))`,
	} {
		var present bool
		if db.QueryRowContext(ctx, query).Scan(&present) == nil && present {
			return true
		}
	}
	return false
}
