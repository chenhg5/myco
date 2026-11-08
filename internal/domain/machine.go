package domain

import (
	"context"
	"strings"
	"time"
)

// Machine is a computer, server or cloud environment that agents run on.
// It is identified by a fingerprint the machine computes from its own
// stable ID (only a hash reaches the hub), so adding the same device again,
// or changing its host name, never creates a second machine.
type Machine struct {
	ID              string
	FingerprintHash string
	Name            string
	Hostname        string
	OS              string
	Arch            string
	MycoVersion     string
	CreatedAt       time.Time
	LastSeenAt      time.Time
	// Owner is the member whose machine this is (from the link it joined with).
	Owner string
}

// MachineReport is what a runner says about its machine.
type MachineReport struct {
	Fingerprint string // already hashed by the machine
	Name        string // suggested display name (the name it joined with)
	Hostname    string
	OS          string
	Arch        string
	MycoVersion string
}

// Validate checks a report's sizes and the fingerprint's shape.
func (r MachineReport) Validate() error {
	if len(r.Fingerprint) < 16 || len(r.Fingerprint) > 128 {
		return Errf(CodeInvalidArgument, "machine fingerprint must be 16-128 characters")
	}
	for _, s := range []string{r.Name, r.Hostname, r.OS, r.Arch, r.MycoVersion} {
		if len(s) > 200 || strings.ContainsAny(s, "\x00\n\r") {
			return Errf(CodeInvalidArgument, "machine fields must be single lines under 200 characters")
		}
	}
	return nil
}

// MachineRepository persists machines.
type MachineRepository interface {
	Create(ctx context.Context, m *Machine) error
	Save(ctx context.Context, m *Machine) error
	Get(ctx context.Context, id string) (*Machine, error)
	GetByFingerprint(ctx context.Context, hash string) (*Machine, error)
	List(ctx context.Context) ([]*Machine, error)
}

// Runtime is what an agent runs on: its CLI, the CLI's version and model,
// and the permission level, as reported by its runner.
type Runtime struct {
	CLI         string `json:"cli,omitempty"`
	CLIVersion  string `json:"cli_version,omitempty"`
	Model       string `json:"model,omitempty"`
	ModelSource string `json:"model_source,omitempty"` // configured | cli-config | observed | default
	Access      string `json:"access,omitempty"`
	// Provider is the model provider's display name; ProviderSource is
	// pinned | machine | env_file | missing.
	Provider       string `json:"provider,omitempty"`
	ProviderSource string `json:"provider_source,omitempty"`
}

// Validate keeps runtime fields short single lines.
func (r Runtime) Validate() error {
	for _, s := range []string{r.CLI, r.CLIVersion, r.Model, r.ModelSource, r.Access, r.Provider, r.ProviderSource} {
		if len(s) > 120 || strings.ContainsAny(s, "\x00\n\r") {
			return Errf(CodeInvalidArgument, "runtime fields must be single lines under 120 characters")
		}
	}
	return nil
}

// Agent kinds, derived from the harness.
const (
	KindAgent   = "agent"   // an AI agent session (claude, codex, ...)
	KindShell   = "shell"   // exec / shell nodes
	KindHuman   = "human"   // a person (no harness)
	KindMachine = "machine" // a machine's node for starting sessions: not a node to people
)

// Kind classifies the agent by its harness.
func (a *Agent) Kind() string {
	switch a.Binding.Harness {
	case "machine":
		return KindMachine
	case "exec", "shell":
		return KindShell
	case "":
		return KindHuman
	}
	return KindAgent
}

// InCloud reports whether the agent is a hosted cloud session (claude.ai,
// Codex cloud, ...): it runs in a temporary sandbox, not on a machine the
// network knows, and nothing else can be started there.
func (a *Agent) InCloud() bool {
	return strings.HasSuffix(a.Binding.Harness, "-cloud") || strings.HasPrefix(a.Binding.Environment, "claude-cloud")
}
