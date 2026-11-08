package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// EnvJoinToken carries a join link's token to `myco machine join` (set by
// the script a join link serves).
const EnvJoinToken = "MYCO_JOIN_TOKEN"

// Machine is this machine's membership, written by `myco machine join`:
// the hub and the machine's own enrollment token, which sessions here use
// to join (`myco join`). Kept in <config dir>/machine.yaml, mode 600.
type Machine struct {
	Hub         string   `yaml:"hub" json:"hub"`
	Machine     string   `yaml:"machine" json:"machine"`
	EnrollToken string   `yaml:"enroll_token" json:"-"`
	NamePrefix  string   `yaml:"name_prefix" json:"name_prefix"`
	Harness     string   `yaml:"harness,omitempty" json:"harness,omitempty"`
	AllowFrom   []string `yaml:"allow_from,omitempty" json:"allow_from,omitempty"`
	// Spawn limits what `myco agent spawn` may start on this machine. It is
	// local policy: only someone with access to this machine can change it.
	Spawn SpawnPolicy `yaml:"spawn,omitempty" json:"spawn,omitempty"`
}

// SpawnPolicy is a machine's local rules for starting new agent sessions.
type SpawnPolicy struct {
	// Disabled turns the machine node off (no `agent spawn` here).
	Disabled bool `yaml:"disabled,omitempty" json:"disabled,omitempty"`
	// Dirs are the directories new sessions may run in, with subdirectories
	// ("~" allowed). Default: the home directory.
	Dirs []string `yaml:"dirs,omitempty" json:"dirs,omitempty"`
	// MaxAccess caps the permission level of spawned sessions: readonly,
	// workspace (default) or full. full is only possible when set here.
	MaxAccess string `yaml:"max_access,omitempty" json:"max_access,omitempty"`
}

// MachinePath returns machine.yaml next to the default config.
func MachinePath(defaultPath string) string { return filepath.Join(Dir(defaultPath), "machine.yaml") }

// LoadMachine reads machine.yaml (nil, nil if this machine never joined).
func LoadMachine(defaultPath string) (*Machine, error) {
	b, err := os.ReadFile(MachinePath(defaultPath))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	m := &Machine{}
	if err := yaml.Unmarshal(b, m); err != nil {
		return nil, err
	}
	return m, nil
}

// SaveMachine writes machine.yaml with mode 600.
func SaveMachine(defaultPath string, m *Machine) error {
	path := MachinePath(defaultPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	return writePrivate(path, b)
}
