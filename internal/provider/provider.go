// Package provider lets a runner pin which model provider (endpoint, key,
// model) an agent's CLI uses, instead of whatever the machine is set to.
//
// Providers come from cc-switch (https://github.com/farion1231/cc-switch),
// read-only from ~/.cc-switch: an agent stores only a reference
// ("cc-switch:<id>") and the provider is read again before every run, so key
// changes made in cc-switch apply without copying secrets anywhere.
package provider

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// Provider is one provider configuration for a CLI.
type Provider struct {
	Ref     string `json:"ref"` // e.g. "cc-switch:<id>"
	ID      string `json:"id"`
	Name    string `json:"name"`
	App     string `json:"app"` // claude | codex | ...
	Current bool   `json:"current,omitempty"`
	// Host is the endpoint's host, for display ("" = the CLI's default).
	Host string `json:"host,omitempty"`
	// Env holds the provider's variables (secrets included); never print it.
	Env map[string]string `json:"-"`
	// Model is the settings-level model, if the provider sets one.
	Model string `json:"model,omitempty"`
	// CodexArgs are `codex -c` overrides (Codex providers); their key is in Env.
	CodexArgs []string `json:"-"`
}

// RefPrefix marks a cc-switch provider reference.
const RefPrefix = "cc-switch:"

// ErrNoCCSwitch: cc-switch's data was not found on this machine.
var ErrNoCCSwitch = errors.New("cc-switch is not installed (no ~/.cc-switch)")

// ErrNotFound: the referenced provider no longer exists.
var ErrNotFound = errors.New("provider not found in cc-switch")

// Dir returns cc-switch's data directory.
func Dir() string {
	home, _ := os.UserHomeDir()
	return DirIn(home)
}

// DirIn returns cc-switch's data directory for home.
func DirIn(home string) string {
	if d := os.Getenv("CC_SWITCH_DIR"); d != "" { // tests and unusual setups
		return d
	}
	return filepath.Join(home, ".cc-switch")
}

// List returns cc-switch's providers for app ("claude", "codex"; "" = all),
// in cc-switch's own order.
func List(dir, app string) ([]Provider, error) {
	if ps, err := listDB(filepath.Join(dir, "cc-switch.db"), app); !errors.Is(err, os.ErrNotExist) {
		return ps, err
	}
	if ps, err := listLegacy(filepath.Join(dir, "config.json"), app); !errors.Is(err, os.ErrNotExist) {
		return ps, err
	}
	return nil, ErrNoCCSwitch
}

// Resolve reads the provider a reference points to, for app.
func Resolve(dir, ref, app string) (Provider, error) {
	id, ok := strings.CutPrefix(ref, RefPrefix)
	if !ok || id == "" {
		return Provider{}, fmt.Errorf("unknown provider reference %q (want %s<id>)", ref, RefPrefix)
	}
	ps, err := List(dir, app)
	if err != nil {
		return Provider{}, err
	}
	for _, p := range ps {
		if p.ID == id {
			return p, nil
		}
	}
	return Provider{}, fmt.Errorf("%w: %s", ErrNotFound, id)
}

// Current returns cc-switch's active provider for app, if any.
func Current(dir, app string) (Provider, bool) {
	ps, err := List(dir, app)
	if err != nil {
		return Provider{}, false
	}
	for _, p := range ps {
		if p.Current {
			return p, true
		}
	}
	return Provider{}, false
}

func listDB(path, app string) ([]Provider, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro&_pragma=busy_timeout(3000)")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	q := `SELECT id, app_type, name, settings_config, is_current FROM providers`
	var args []any
	if app != "" {
		q += ` WHERE app_type = ?`
		args = append(args, app)
	}
	q += ` ORDER BY COALESCE(sort_index, 1e9), created_at, name`
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("read cc-switch providers: %w", err)
	}
	defer rows.Close()
	var out []Provider
	for rows.Next() {
		var p Provider
		var cfg string
		if err := rows.Scan(&p.ID, &p.App, &p.Name, &cfg, &p.Current); err != nil {
			return nil, err
		}
		fill(&p, []byte(cfg))
		out = append(out, p)
	}
	return out, rows.Err()
}

// listLegacy reads cc-switch's pre-database config.json:
// {"claude": {"providers": {"<id>": {"id", "name", "settingsConfig"}}, "current": "<id>"}, ...}
func listLegacy(path, app string) ([]Provider, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var out []Provider
	for _, a := range []string{"claude", "codex", "gemini"} {
		if app != "" && a != app {
			continue
		}
		var sec struct {
			Providers map[string]struct {
				ID             string          `json:"id"`
				Name           string          `json:"name"`
				SettingsConfig json.RawMessage `json:"settingsConfig"`
			} `json:"providers"`
			Current string `json:"current"`
		}
		if raw, ok := doc[a]; !ok || json.Unmarshal(raw, &sec) != nil {
			continue
		}
		var part []Provider
		for id, v := range sec.Providers {
			p := Provider{ID: id, Name: v.Name, App: a, Current: id == sec.Current}
			fill(&p, v.SettingsConfig)
			part = append(part, p)
		}
		sort.Slice(part, func(i, j int) bool { return part[i].Name < part[j].Name })
		out = append(out, part...)
	}
	return out, nil
}

// fill decodes a provider's settings: Claude-style {"env": {...}, "model"}
// or Codex-style {"auth": {"OPENAI_API_KEY"}, "config": "<config.toml>"}.
func fill(p *Provider, cfg []byte) {
	p.Ref = RefPrefix + p.ID
	var s struct {
		Env    map[string]any `json:"env"`
		Model  string         `json:"model"`
		Auth   map[string]any `json:"auth"`
		Config string         `json:"config"`
	}
	_ = json.Unmarshal(cfg, &s)
	if p.App == "codex" {
		key, _ := s.Auth["OPENAI_API_KEY"].(string)
		p.CodexArgs, p.Env, p.Host, p.Model = CodexOverrides(s.Config, key)
		return
	}
	p.Model = s.Model
	p.Env = map[string]string{}
	for k, v := range s.Env {
		switch t := v.(type) {
		case string:
			p.Env[k] = t
		case nil:
		default:
			b, _ := json.Marshal(t)
			p.Env[k] = string(b)
		}
	}
	if u := p.Env["ANTHROPIC_BASE_URL"]; u != "" {
		if pu, err := url.Parse(u); err == nil {
			p.Host = pu.Host
		}
	}
}
