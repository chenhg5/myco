package domain

import (
	"context"
	"slices"
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
	Delete(ctx context.Context, id string) error
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
	// Effort is the reasoning effort the CLI runs with; EffortSource is
	// app (the agent's run settings), self (the agent reported it) or "".
	Effort       string `json:"effort,omitempty"`
	EffortSource string `json:"effort_source,omitempty"`
	// MaxAccess is the highest access its machine lets the hub set.
	MaxAccess string `json:"max_access,omitempty"`
	// Quota is the account's usage limits as the CLI last reported them
	// (nil until a run reported any).
	Quota *Quota `json:"quota,omitempty"`
}

// Quota is a CLI account's usage limits at one moment.
type Quota struct {
	// Source is the CLI that reported it: claude or codex.
	Source  string        `json:"source"`
	At      time.Time     `json:"at"`
	Windows []QuotaWindow `json:"windows"`
}

// QuotaWindow is one limit: how much of it is used (0..1) and when it resets.
type QuotaWindow struct {
	// Name: five_hour, seven_day (Claude Code), primary, secondary (Codex).
	Name string `json:"name"`
	// Label describes the window when its name does not, e.g. "5h" or "7d".
	Label    string     `json:"label,omitempty"`
	Used     float64    `json:"used"`
	ResetsAt *time.Time `json:"resets_at,omitempty"`
	// Status: allowed, or why work is refused (e.g. rejected, limit reached).
	Status string `json:"status,omitempty"`
}

// RunSettings choose how an agent's runner starts its CLI; "" follows the
// machine's own settings.
type RunSettings struct {
	Model  string `json:"model,omitempty"`
	Effort string `json:"effort,omitempty"`
	// Access is the permission level (readonly, workspace or full). Only the
	// agent's owner sets it, and its machine caps it (spawn.max_access).
	Access string `json:"access,omitempty"`
}

// Accesses are the permission levels, lowest first.
var Accesses = []string{"readonly", "workspace", "full"}

// Efforts are the reasoning efforts each CLI takes.
var Efforts = map[string][]string{
	"claude": {"low", "medium", "high", "xhigh", "max"},
	"codex":  {"minimal", "low", "medium", "high", "xhigh"},
}

// Validate checks the settings for an agent on harness h.
func (rs RunSettings) Validate(h string) error {
	for _, s := range []string{rs.Model, rs.Effort} {
		if len(s) > 80 || strings.ContainsAny(s, "\x00\n\r \t") {
			return Errf(CodeInvalidArgument, "model and effort must be single words under 80 characters")
		}
	}
	if rs.Access != "" && !slices.Contains(Accesses, rs.Access) {
		return Errf(CodeInvalidArgument, "access %q is not one of %s", rs.Access, strings.Join(Accesses, ", "))
	}
	if rs.Effort == "" {
		return nil
	}
	allowed, ok := Efforts[h]
	if !ok {
		return Errf(CodeInvalidArgument, "%s agents have no reasoning effort setting", orNone(h)).
			WithSuggestion("set only the model, or leave the effort empty")
	}
	if !slices.Contains(allowed, rs.Effort) {
		return Errf(CodeInvalidArgument, "effort %q is not one of %s for %s", rs.Effort, strings.Join(allowed, ", "), h)
	}
	return nil
}

func orNone(h string) string {
	if h == "" {
		return "these"
	}
	return h
}

// Validate keeps runtime fields short single lines.
func (r Runtime) Validate() error {
	for _, s := range []string{r.CLI, r.CLIVersion, r.Model, r.ModelSource, r.Access, r.Provider, r.ProviderSource, r.Effort, r.EffortSource, r.MaxAccess} {
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
