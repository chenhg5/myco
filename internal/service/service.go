// Package service installs `myco runner start --all` as a per-user background
// service: launchd on macOS, systemd --user on Linux.
package service

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Spec describes the service to install.
type Spec struct {
	// Binary is the absolute path of the myco executable.
	Binary string
	// Args follow the binary, e.g. runner start --all.
	Args []string
	// Env is written into the service definition (no secrets).
	Env map[string]string
	// LogPath receives stdout and stderr.
	LogPath string
}

// Status reports the service state.
type Status struct {
	Manager   string `json:"manager"`
	File      string `json:"file"`
	Installed bool   `json:"installed"`
	Running   bool   `json:"running"`
	PID       int    `json:"pid,omitempty"`
	LogPath   string `json:"log_path,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

// Plan is what Install/Uninstall will do (also used for --dry-run).
type Plan struct {
	Manager  string     `json:"manager"`
	File     string     `json:"file"`
	Content  string     `json:"content,omitempty"`
	Commands [][]string `json:"commands"`
	Notes    []string   `json:"notes,omitempty"`
}

// Manager installs and controls the runner service.
type Manager interface {
	Name() string
	PlanInstall(spec Spec) (Plan, error)
	PlanUninstall() Plan
	Status() (Status, error)
	Restart() error
}

// Runner executes system commands (swappable in tests).
type Runner interface {
	Run(name string, args ...string) (string, error)
}

// ExecRunner runs real commands.
type ExecRunner struct{}

func (ExecRunner) Run(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s %s: %v: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// Label is the runner service's launchd label.
const Label = "ai.myco.runner"

// ID names a service: its launchd label, systemd unit and description.
type ID struct {
	Label       string
	Unit        string
	Description string
}

// RunnerID is `myco runner start --all`; HubID is a hub on a personal
// machine (`myco hub up`).
var (
	RunnerID = ID{Label: Label, Unit: "myco-runner.service", Description: "myco runner (serves every local agent)"}
	HubID    = ID{Label: "ai.myco.hub", Unit: "myco-hub.service", Description: "myco hub"}
)

func (id ID) orDefault() ID {
	if id.Label == "" {
		return RunnerID
	}
	return id
}

// ErrUnsupported is returned on platforms without a supported manager.
var ErrUnsupported = errors.New("no supported service manager on this platform (launchd on macOS, systemd on Linux)")

// Detect returns the runner service's manager for this OS.
func Detect(home string, r Runner) (Manager, error) { return DetectID(home, r, RunnerID) }

// DetectID returns the manager of service id for this OS.
func DetectID(home string, r Runner, id ID) (Manager, error) {
	switch runtime.GOOS {
	case "darwin":
		return &Launchd{Home: home, UID: os.Getuid(), R: r, ID: id}, nil
	case "linux":
		if _, err := exec.LookPath("systemctl"); err != nil {
			return nil, fmt.Errorf("%w: systemctl not found", ErrUnsupported)
		}
		return &Systemd{Home: home, R: r, ID: id}, nil
	}
	return nil, ErrUnsupported
}

// Apply writes the plan's file (if any) and runs its commands in order.
// Commands prefixed with "-" may fail (e.g. stopping a service that is
// not loaded yet).
func Apply(p Plan, r Runner) error {
	if p.Content != "" {
		if err := os.MkdirAll(filepath.Dir(p.File), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p.File, []byte(p.Content), 0o644); err != nil {
			return err
		}
	}
	for _, c := range p.Commands {
		name, optional := c[0], false
		if strings.HasPrefix(name, "-") {
			name, optional = name[1:], true
		}
		if _, err := r.Run(name, c[1:]...); err != nil && !optional {
			return err
		}
	}
	return nil
}

// RemoveFile deletes the service definition after uninstalling.
func RemoveFile(p Plan) error {
	if err := os.Remove(p.File); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

func exists(path string) bool { _, err := os.Stat(path); return err == nil }
