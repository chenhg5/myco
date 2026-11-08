package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// EnvJoinToken carries a join link's token to `myco machine join` (set by
// the script a join link serves).
const EnvJoinToken = "MYCO_JOIN_TOKEN"

// EnvInviteCode carries an invite's one-time code to `myco member accept`
// (set by the invite link's script).
const EnvInviteCode = "MYCO_INVITE_CODE"

// MachineHub is this machine's membership of one hub: the hub and the
// machine's own enrollment token there, which sessions use to join.
type MachineHub struct {
	Hub         string   `yaml:"hub" json:"hub"`
	Machine     string   `yaml:"machine" json:"machine"`
	EnrollToken string   `yaml:"enroll_token" json:"-"`
	NamePrefix  string   `yaml:"name_prefix" json:"name_prefix"`
	Harness     string   `yaml:"harness,omitempty" json:"harness,omitempty"`
	AllowFrom   []string `yaml:"allow_from,omitempty" json:"allow_from,omitempty"`
	// MemberToken is the person's own credential on this hub when they
	// joined with an invite on the command line (`myco member accept`): a
	// device token like the app's, used by commands that act as them.
	MemberToken string `yaml:"member_token,omitempty" json:"-"`
}

// Machine is this machine's memberships, written by `myco machine join`:
// the first hub it joined (the primary one, used by default) and any other
// hubs it joined since (Others). Kept in <config dir>/machine.yaml, mode 600.
type Machine struct {
	MachineHub `yaml:",inline"`
	// Others are further hubs this machine joined, each with its own
	// credentials and machine node.
	Others []MachineHub `yaml:"others,omitempty" json:"others,omitempty"`
	// Spawn limits what `myco agent spawn` may start on this machine. It is
	// local policy: only someone with access to this machine can change it,
	// and it applies whichever hub asks.
	Spawn SpawnPolicy `yaml:"spawn,omitempty" json:"spawn,omitempty"`
}

// Hubs lists every membership, the primary one first.
func (m *Machine) Hubs() []MachineHub {
	if m == nil {
		return nil
	}
	var out []MachineHub
	if m.Hub != "" {
		out = append(out, m.MachineHub)
	}
	return append(out, m.Others...)
}

// For returns the membership of hub.
func (m *Machine) For(hub string) (MachineHub, bool) {
	for _, h := range m.Hubs() {
		if SameHub(h.Hub, hub) {
			return h, true
		}
	}
	return MachineHub{}, false
}

// Put adds or replaces the membership of h.Hub (the first one becomes the
// primary).
func (m *Machine) Put(h MachineHub) {
	switch {
	case m.Hub == "" || SameHub(m.Hub, h.Hub):
		h.MemberToken = orKeep(h.MemberToken, m.MemberToken, SameHub(m.Hub, h.Hub))
		m.MachineHub = h
		return
	}
	for i := range m.Others {
		if SameHub(m.Others[i].Hub, h.Hub) {
			h.MemberToken = orKeep(h.MemberToken, m.Others[i].MemberToken, true)
			m.Others[i] = h
			return
		}
	}
	m.Others = append(m.Others, h)
}

// orKeep is v, or old when v is empty and old belongs to the same hub (a
// re-join keeps the person's credential).
func orKeep(v, old string, same bool) string {
	if v == "" && same {
		return old
	}
	return v
}

// SameHub compares hub URLs, ignoring a trailing slash and letter case.
func SameHub(a, b string) bool {
	return a != "" && strings.EqualFold(strings.TrimRight(a, "/"), strings.TrimRight(b, "/"))
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
