package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// DefaultMaxConcurrent is how many agent CLIs a runner (runner start --all)
// runs at once when neither its flag nor machine.yaml says otherwise.
const DefaultMaxConcurrent = 10

// MaxConcurrentCeiling is the highest limit the setting accepts.
const MaxConcurrentCeiling = 64

// RunnerPolicy is machine.yaml's runner section.
type RunnerPolicy struct {
	// MaxConcurrent: agent CLIs running at once on this machine, shared
	// between its workspaces (0: not set here).
	MaxConcurrent int `yaml:"max_concurrent,omitempty" json:"max_concurrent,omitempty"`
	// SetAt is when it was last set (from the App or `myco machine
	// concurrency`): it wins over a runner flag chosen before then.
	SetAt time.Time `yaml:"set_at,omitempty" json:"set_at,omitempty"`
}

// Concurrency sources, as EffectiveConcurrency reports them.
const (
	ConcurrencyFromSetting = "setting"
	ConcurrencyFromFlag    = "flag"
	ConcurrencyFromDefault = "default"
)

// EffectiveConcurrency is the rule for how many agent CLIs run at once: of
// the runner's explicit --max-concurrent (flag, chosen at flagAt) and
// machine.yaml's runner.max_concurrent, the one chosen last; the default
// when neither is there.
func EffectiveConcurrency(flag int, flagAt time.Time, set RunnerPolicy) (int, string) {
	if set.MaxConcurrent > 0 && (flag <= 0 || set.SetAt.After(flagAt)) {
		return min(set.MaxConcurrent, MaxConcurrentCeiling), ConcurrencyFromSetting
	}
	if flag > 0 {
		return flag, ConcurrencyFromFlag
	}
	return DefaultMaxConcurrent, ConcurrencyFromDefault
}

// LoadRunnerPolicy reads machine.yaml's runner section (zero when the file
// or the section is not there).
func LoadRunnerPolicy(defaultPath string) (RunnerPolicy, error) {
	m, err := LoadMachine(defaultPath)
	if err != nil || m == nil {
		return RunnerPolicy{}, err
	}
	return m.Runner, nil
}

// SetRunnerConcurrency sets runner.max_concurrent in machine.yaml, stamped
// with now so it wins over a runner flag chosen before. Every other key is
// kept as it is. ErrNotJoined when there is no machine.yaml.
func SetRunnerConcurrency(defaultPath string, n int, now time.Time) error {
	if n < 1 || n > MaxConcurrentCeiling {
		return fmt.Errorf("the limit must be between 1 and %d", MaxConcurrentCeiling)
	}
	path := MachinePath(defaultPath)
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return ErrNotJoined
	}
	if err != nil {
		return err
	}
	doc := map[string]any{}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return err
	}
	runner, _ := doc["runner"].(map[string]any)
	if runner == nil {
		runner = map[string]any{}
	}
	runner["max_concurrent"] = n
	runner["set_at"] = now.UTC().Format(time.RFC3339Nano)
	doc["runner"] = runner
	out, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}
	return writePrivate(path, out)
}

// RunnerSlotsState is what a runner (runner start --all) writes about its
// slots next to machine.yaml, for the App on this computer and for the
// flag's place in EffectiveConcurrency. It holds no secrets.
type RunnerSlotsState struct {
	PID     int    `json:"pid"`
	Version string `json:"version,omitempty"`
	// Limit is the limit in force and Source where it comes from.
	Limit  int    `json:"limit"`
	Source string `json:"source"`
	// Flag is the runner's explicit --max-concurrent (0: none) and FlagAt
	// when this value was first seen (a restart with the same flag keeps it).
	Flag    int       `json:"flag,omitempty"`
	FlagAt  time.Time `json:"flag_at,omitempty"`
	InUse   int       `json:"in_use"`
	Waiting int       `json:"waiting"`
	At      time.Time `json:"at"`
}

// RunnerSlotsPath is that file.
func RunnerSlotsPath(defaultPath string) string {
	return filepath.Join(Dir(defaultPath), "runner-slots.json")
}

// LoadRunnerSlots reads it (nil, nil when no runner wrote one).
func LoadRunnerSlots(defaultPath string) (*RunnerSlotsState, error) {
	b, err := os.ReadFile(RunnerSlotsPath(defaultPath))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s := &RunnerSlotsState{}
	if err := json.Unmarshal(b, s); err != nil {
		return nil, err
	}
	return s, nil
}

// SaveRunnerSlots writes it.
func SaveRunnerSlots(defaultPath string, s *RunnerSlotsState) error {
	path := RunnerSlotsPath(defaultPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return writePrivate(path, b)
}

// FlagSince is when the runner's flag value was chosen: the time it was
// first seen, kept from an earlier run with the same value.
func FlagSince(prev *RunnerSlotsState, flag int, now time.Time) time.Time {
	if flag <= 0 {
		return time.Time{}
	}
	if prev != nil && prev.Flag == flag && !prev.FlagAt.IsZero() {
		return prev.FlagAt
	}
	return now
}

// RunnerSlotsFresh: a running runner rewrites runner-slots.json at least
// every 30 seconds, so one older than this is from a runner that stopped.
const RunnerSlotsFresh = 90 * time.Second

// Fresh reports whether a runner that is still running wrote s.
func (s *RunnerSlotsState) Fresh(now time.Time) bool {
	return s != nil && now.Sub(s.At) <= RunnerSlotsFresh && !s.At.After(now.Add(time.Minute))
}
