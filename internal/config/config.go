// Package config loads and saves the client-side myco configuration
// (~/.myco/config.yaml by default).
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chenhg5/myco/internal/harness"
	"github.com/chenhg5/myco/internal/osutil"
)

// Env variable names.
const (
	EnvConfig     = "MYCO_CONFIG"
	EnvHub        = "MYCO_HUB"
	EnvToken      = "MYCO_TOKEN"
	EnvAdminToken = "MYCO_ADMIN_TOKEN"
	// EnvEnrollToken lets a session join the network by itself (see
	// `myco enrollment create`). Safe to put in an environment's secrets.
	EnvEnrollToken = "MYCO_ENROLL_TOKEN"
)

// Config is the on-disk client configuration.
type Config struct {
	Hub   string `yaml:"hub,omitempty" json:"hub,omitempty"`
	Agent string `yaml:"agent,omitempty" json:"agent,omitempty"`
	Token string `yaml:"token,omitempty" json:"token,omitempty"`
	// Harness and SessionID record which session this identity belongs to,
	// so the CLI can pick the right profile automatically inside a session.
	Harness   string `yaml:"harness,omitempty" json:"harness,omitempty"`
	SessionID string `yaml:"session_id,omitempty" json:"session_id,omitempty"`
	Runner    Runner `yaml:"runner,omitempty" json:"runner,omitempty"`
}

// Runner configures `myco runner start`.
type Runner struct {
	Harness     string   `yaml:"harness,omitempty" json:"harness,omitempty"`
	Binary      string   `yaml:"binary,omitempty" json:"binary,omitempty"`
	Args        []string `yaml:"args,omitempty" json:"args,omitempty"`
	Command     string   `yaml:"command,omitempty" json:"command,omitempty"`
	Workdir     string   `yaml:"workdir,omitempty" json:"workdir,omitempty"`
	Timeout     string   `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	Concurrency int      `yaml:"concurrency,omitempty" json:"concurrency,omitempty"`
	PollWait    string   `yaml:"poll_wait,omitempty" json:"poll_wait,omitempty"`
	// EnvFile holds KEY=VALUE lines passed only to this agent's CLI (e.g.
	// CLAUDE_CODE_OAUTH_TOKEN from `claude setup-token`). Keep it chmod 600.
	EnvFile string `yaml:"env_file,omitempty" json:"env_file,omitempty"`
	// Model and Access are mapped to CLI flags by the harness.
	Model  string `yaml:"model,omitempty" json:"model,omitempty"`
	Access string `yaml:"access,omitempty" json:"access,omitempty"`
	// Template configures the "template" harness, or overrides fields of a
	// preset (pi, kiro, cursor, gemini, opencode).
	Template *harness.TemplateSpec `yaml:"template,omitempty" json:"template,omitempty"`
	// Exec configures an exec node (harness "exec"): command allowlist,
	// timeout, output cap, shell and inherited environment.
	Exec *harness.ExecSpec `yaml:"exec,omitempty" json:"exec,omitempty"`
}

// DefaultPath returns ~/.myco/config.yaml.
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".myco/config.yaml"
	}
	return filepath.Join(home, ".myco", "config.yaml")
}

// Load reads path; a missing file yields an empty config.
func Load(path string) (*Config, error) {
	c := &Config{}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(b, c); err != nil {
		return nil, err
	}
	return c, nil
}

// Save writes c to path with owner-only permissions (it contains a token).
func Save(path string, c *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return writePrivate(path, b)
}

// ApplyEnv overrides hub/token from the environment (getenv is injected
// so callers and tests control the source).
func (c *Config) ApplyEnv(getenv func(string) string) {
	if v := getenv(EnvHub); v != "" {
		c.Hub = v
	}
	if v := getenv(EnvToken); v != "" {
		c.Token = v
	}
}

// Redacted returns a copy safe to print.
func (c Config) Redacted() Config {
	if c.Token != "" {
		c.Token = "***"
	}
	return c
}

// Dir is the directory holding the config file (runner state lives here).
func Dir(path string) string { return filepath.Dir(path) }

// Profiles: every agent identity bound to a session on this machine lives in
// <config dir>/agents/<agent>.yaml. The CLI picks the profile whose
// session_id matches the session it runs in, and `myco runner start --all`
// serves all of them from one process.

// ProfilesDir returns the profiles directory next to the default config.
func ProfilesDir(defaultPath string) string { return filepath.Join(Dir(defaultPath), "agents") }

// ProfilePath returns the profile file for agent.
func ProfilePath(defaultPath, agent string) string {
	return filepath.Join(ProfilesDir(defaultPath), agent+".yaml")
}

// Profile is a loaded profile file.
type Profile struct {
	Path   string
	Config *Config
}

// ListProfiles loads every *.yaml in dir (missing dir = none).
func ListProfiles(dir string) ([]Profile, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	var out []Profile
	for _, m := range matches {
		c, err := Load(m)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", m, err)
		}
		out = append(out, Profile{Path: m, Config: c})
	}
	return out, nil
}

// FindBySession returns the profile bound to sessionID, if any.
func FindBySession(dir, sessionID string) (Profile, bool) {
	if sessionID == "" {
		return Profile{}, false
	}
	ps, _ := ListProfiles(dir)
	for _, p := range ps {
		if p.Config.SessionID == sessionID {
			return p, true
		}
	}
	return Profile{}, false
}

// ReadEnvFile parses KEY=VALUE lines (blank lines, # comments and an
// optional "export " prefix allowed; values may be quoted).
func ReadEnvFile(path string) ([]string, error) {
	if _, ok := osutil.HomeRelative(path); ok {
		home, _ := os.UserHomeDir()
		path = osutil.ExpandHome(path, home)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []string
	for i, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		k = strings.TrimSpace(k)
		if !ok || k == "" || strings.ContainsAny(k, " \t") {
			return nil, fmt.Errorf("%s:%d: expected KEY=VALUE", path, i+1)
		}
		v = strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
			v = v[1 : len(v)-1]
		}
		out = append(out, k+"="+v)
	}
	return out, nil
}

// writePrivate atomically replaces path with b, readable only by the owner.
// The temporary file is always created fresh (os.CreateTemp, mode 0600):
// reusing a fixed name like path+".tmp" would keep the mode of a stale file
// left behind by an earlier crash, and the rename would publish the token
// with it.
func writePrivate(path string, b []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(b); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
