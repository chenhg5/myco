package domain

import (
	"regexp"
	"slices"
	"strings"
	"time"
)

var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`)

// AgentName is the unique, human-readable address of an agent.
type AgentName string

// ParseAgentName validates and normalizes an agent name.
func ParseAgentName(s string) (AgentName, error) {
	s = strings.TrimSpace(s)
	if !namePattern.MatchString(s) {
		return "", Errf(CodeInvalidArgument, "invalid agent name %q", s).
			WithSuggestion("use 1-63 chars of lowercase letters, digits, '.', '_' or '-', starting with a letter or digit")
	}
	return AgentName(s), nil
}

// ParseAgentNames validates a list of names, dropping blanks and duplicates.
func ParseAgentNames(in []string) ([]AgentName, error) {
	out := make([]AgentName, 0, len(in))
	for _, s := range in {
		if strings.TrimSpace(s) == "" {
			continue
		}
		n, err := ParseAgentName(s)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	return out, nil
}

// ParseCapabilities validates capability tags (same charset as names).
func ParseCapabilities(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" {
			continue
		}
		if !namePattern.MatchString(s) {
			return nil, Errf(CodeInvalidArgument, "invalid capability %q", s).
				WithSuggestion("capabilities are short lowercase tags such as code, github, shell")
		}
		if !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out, nil
}

// DeliveryMode describes how the hub gets messages to an agent.
type DeliveryMode string

const (
	// DeliveryPull: the agent (usually via `myco runner`) pulls its inbox.
	DeliveryPull DeliveryMode = "pull"
)

// ParseDeliveryMode validates a delivery mode; empty means pull.
func ParseDeliveryMode(s string) (DeliveryMode, error) {
	switch DeliveryMode(s) {
	case "", DeliveryPull:
		return DeliveryPull, nil
	}
	return "", Errf(CodeInvalidArgument, "unsupported delivery mode %q", s).
		WithSuggestion("supported modes: pull")
}

// Agent is a registered participant of the network.
type Agent struct {
	ID           string
	Name         AgentName
	Description  string
	Capabilities []string
	Delivery     DeliveryMode
	// AllowFrom restricts who may send tasks/notes to this agent.
	// Empty means anyone. Replies are always accepted.
	AllowFrom  []AgentName
	TokenHash  string
	CreatedAt  time.Time
	LastSeenAt time.Time
}

// AgentSpec is the mutable profile of an agent.
type AgentSpec struct {
	Description  string
	Capabilities []string
	Delivery     DeliveryMode
	AllowFrom    []AgentName
}

const maxDescription = 2000

func (s AgentSpec) validate() error {
	if len(s.Description) > maxDescription {
		return Errf(CodeInvalidArgument, "description longer than %d bytes", maxDescription)
	}
	return nil
}

// NewAgent creates a validated agent.
func NewAgent(id string, name AgentName, spec AgentSpec, tokenHash string, now time.Time) (*Agent, error) {
	if err := spec.validate(); err != nil {
		return nil, err
	}
	a := &Agent{ID: id, Name: name, TokenHash: tokenHash, CreatedAt: now}
	a.apply(spec)
	return a, nil
}

// Update replaces the agent's profile.
func (a *Agent) Update(spec AgentSpec) error {
	if err := spec.validate(); err != nil {
		return err
	}
	a.apply(spec)
	return nil
}

func (a *Agent) apply(s AgentSpec) {
	a.Description = s.Description
	a.Capabilities = s.Capabilities
	a.Delivery = s.Delivery
	a.AllowFrom = s.AllowFrom
}

// RotateToken replaces the agent's credential.
func (a *Agent) RotateToken(hash string) { a.TokenHash = hash }

// Accepts reports whether sender may send new tasks/notes to this agent.
func (a *Agent) Accepts(sender AgentName) bool {
	return len(a.AllowFrom) == 0 || slices.Contains(a.AllowFrom, sender)
}

// IsOnline reports whether the agent was seen within window.
func (a *Agent) IsOnline(now time.Time, window time.Duration) bool {
	return !a.LastSeenAt.IsZero() && now.Sub(a.LastSeenAt) <= window
}

// HasCapability reports whether the agent advertises capability c.
func (a *Agent) HasCapability(c string) bool { return slices.Contains(a.Capabilities, c) }
