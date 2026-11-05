// Package config loads and saves the client-side myco configuration
// (~/.myco/config.yaml by default).
package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
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
	Hub    string `yaml:"hub,omitempty"`
	Agent  string `yaml:"agent,omitempty"`
	Token  string `yaml:"token,omitempty"`
	Runner Runner `yaml:"runner,omitempty"`
}

// Runner configures `myco runner start`.
type Runner struct {
	Harness     string   `yaml:"harness,omitempty"`
	Binary      string   `yaml:"binary,omitempty"`
	Args        []string `yaml:"args,omitempty"`
	Command     string   `yaml:"command,omitempty"`
	Workdir     string   `yaml:"workdir,omitempty"`
	Timeout     string   `yaml:"timeout,omitempty"`
	Concurrency int      `yaml:"concurrency,omitempty"`
	PollWait    string   `yaml:"poll_wait,omitempty"`
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
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
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
