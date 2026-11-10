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

// Binding ties an agent identity to one concrete session of one harness in
// one environment. In myco an agent *is* a session: a blank new session is a
// new agent, and waking an agent means resuming exactly this session.
type Binding struct {
	// Harness is the agent runtime, e.g. claude, codex, shell, claude-cloud.
	Harness string
	// SessionID identifies the harness session to resume (claude --resume,
	// a Claude Code cloud session_..., a codex thread). Empty until known.
	SessionID string
	// Environment names where the session lives (host name, cloud env id).
	Environment string
	// Workdir is the working directory of the session, if meaningful.
	Workdir string
}

const maxBindingField = 512

// Validate checks field formats.
func (b Binding) Validate() error {
	if b.Harness != "" && !namePattern.MatchString(b.Harness) {
		return Errf(CodeInvalidArgument, "invalid harness %q", b.Harness).
			WithSuggestion("use a short lowercase name such as claude, codex, shell, claude-cloud")
	}
	for k, v := range map[string]string{"session id": b.SessionID, "environment": b.Environment, "workdir": b.Workdir} {
		if len(v) > maxBindingField || strings.ContainsAny(v, "\n\r\t") {
			return Errf(CodeInvalidArgument, "invalid %s %q", k, v)
		}
	}
	if strings.ContainsAny(b.SessionID, " ") {
		return Errf(CodeInvalidArgument, "session id must not contain spaces")
	}
	return nil
}

// DeliveryPull is the built-in mode: the agent pulls its own inbox (usually
// via `myco runner`). Every other mode names a pluggable Waker on the hub.
const DeliveryPull = "pull"

// Delivery configures how the hub wakes an agent when messages arrive.
type Delivery struct {
	// Mode selects the waker: pull, webhook, claude_session, claude_routine,
	// github_dispatch, github_issue, ...
	Mode string
	// URL is the endpoint the waker calls (webhook URL, routine fire URL).
	URL string
	// Secret authenticates the wake call (bearer token / HMAC key). It is
	// write-only: never returned by the API.
	Secret string
	// Params holds mode-specific settings (e.g. repo=owner/name).
	Params map[string]string
}

// IsPush reports whether the hub must actively wake the agent.
func (d Delivery) IsPush() bool { return d.Mode != "" && d.Mode != DeliveryPull }

// Validate checks generic delivery fields; mode-specific rules are enforced
// by the matching waker.
func (d Delivery) Validate() error {
	if d.Mode != "" && !namePattern.MatchString(d.Mode) {
		return Errf(CodeInvalidArgument, "invalid delivery mode %q", d.Mode)
	}
	if len(d.URL) > 2048 || len(d.Secret) > 4096 || len(d.Params) > 32 {
		return Errf(CodeInvalidArgument, "delivery settings too large")
	}
	return nil
}

// Agent is a registered participant of the network.
type Agent struct {
	ID   string
	Name AgentName
	// StatusText is the agent's public progress line ("refactoring the
	// runner: 3/5 done"), visible to everyone who can list agents.
	StatusText string
	StatusAt   time.Time
	// DisplayName is cosmetic (shown in lists and the dashboard); Name is
	// the immutable address.
	DisplayName string
	Description string
	// Capabilities are the names of Skills (kept for clients and filters
	// that predate skills).
	Capabilities []string
	Skills       []Skill
	// Labels group agents (tags or key=value); see Selector.
	Labels   []string
	Binding  Binding
	Delivery Delivery
	// AllowFrom restricts who may send tasks/notes to this agent.
	// Empty means anyone. Replies are always accepted.
	AllowFrom []AgentName
	TokenHash string
	// EnrolledBy is the enrollment that created this agent ("" if the admin
	// registered it). Only that enrollment can recover the agent.
	EnrolledBy string
	CreatedAt  time.Time
	LastSeenAt time.Time
	// MachineID is the machine the agent runs on ("" until its runner
	// reports it); Runtime is its CLI and model as reported by the runner.
	// Both are set only through SetMachine / SetRuntime.
	MachineID string
	Runtime   Runtime
	// Owner is the member who added the agent ("" on hubs without members).
	Owner string
	// Visibility: team (everyone; "" before it existed) or private (only
	// the owner and the owner's agents). Set only by people, never by the
	// agent itself.
	Visibility Visibility
	// Coordinator lets this agent edit the card (display name, description,
	// labels, skills) of its owner's other agents. Set only by people.
	Coordinator bool
}

// AgentSpec is the mutable profile of an agent.
type AgentSpec struct {
	DisplayName  string
	Description  string
	Capabilities []string
	Labels       []string
	Binding      Binding
	Delivery     Delivery
	AllowFrom    []AgentName
}

const maxDescription = 2000

func (s AgentSpec) validate() error {
	if _, err := ParseDisplayName(s.DisplayName); err != nil {
		return err
	}
	if _, err := ParseLabels(s.Labels); err != nil {
		return err
	}
	if len(s.Description) > maxDescription {
		return Errf(CodeInvalidArgument, "description longer than %d bytes", maxDescription)
	}
	if err := s.Binding.Validate(); err != nil {
		return err
	}
	return s.Delivery.Validate()
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

// Update merges spec into the agent's profile: fields left empty in spec
// keep their current value (re-registering to rotate a token must not wipe
// the session binding or delivery settings). A delivery secret left empty
// keeps the current one when the mode is unchanged.
func (a *Agent) Update(spec AgentSpec) error {
	if err := spec.validate(); err != nil {
		return err
	}
	if spec.Description == "" {
		spec.Description = a.Description
	}
	if spec.DisplayName == "" {
		spec.DisplayName = a.DisplayName
	}
	if len(spec.Capabilities) == 0 {
		spec.Capabilities = a.Capabilities
	}
	if len(spec.Labels) == 0 {
		spec.Labels = a.Labels
	}
	if len(spec.AllowFrom) == 0 {
		spec.AllowFrom = a.AllowFrom
	}
	cur := a.Binding
	if err := (&cur).mergeFrom(spec.Binding); err != nil {
		return err
	}
	spec.Binding = cur
	if spec.Delivery.Mode == "" {
		spec.Delivery = a.Delivery
	} else if spec.Delivery.Secret == "" && spec.Delivery.Mode == a.Delivery.Mode {
		spec.Delivery.Secret = a.Delivery.Secret
		if spec.Delivery.URL == "" {
			spec.Delivery.URL = a.Delivery.URL
		}
		if spec.Delivery.Params == nil {
			spec.Delivery.Params = a.Delivery.Params
		}
	}
	a.apply(spec)
	return nil
}

// mergeFrom overwrites b's fields with the non-empty fields of o.
func (b *Binding) mergeFrom(o Binding) error {
	if err := o.Validate(); err != nil {
		return err
	}
	if o.Harness != "" {
		b.Harness = o.Harness
	}
	if o.SessionID != "" {
		b.SessionID = o.SessionID
	}
	if o.Environment != "" {
		b.Environment = o.Environment
	}
	if o.Workdir != "" {
		b.Workdir = o.Workdir
	}
	return nil
}

func (a *Agent) apply(s AgentSpec) {
	a.DisplayName = strings.TrimSpace(s.DisplayName)
	a.Description = s.Description
	a.Capabilities = s.Capabilities
	a.Skills = skillsFor(s.Capabilities, a.Skills)
	a.Labels = s.Labels
	a.Binding = s.Binding
	if s.Delivery.Mode == "" {
		s.Delivery.Mode = DeliveryPull
	}
	a.Delivery = s.Delivery
	a.AllowFrom = s.AllowFrom
}

// Bind updates the session binding; empty fields keep their current value.
func (a *Agent) Bind(b Binding) error { return a.Binding.mergeFrom(b) }

// RotateToken replaces the agent's credential.
func (a *Agent) RotateToken(hash string) { a.TokenHash = hash }

// Accepts reports whether sender may send new tasks/notes to this agent.
func (a *Agent) Accepts(sender AgentName) bool {
	return len(a.AllowFrom) == 0 || slices.Contains(a.AllowFrom, sender) || slices.Contains(a.AllowFrom, AnySender)
}

// AnySender in allow_from explicitly admits every agent. Exec nodes need it
// (an empty list means "not configured" for them and is refused).
const AnySender AgentName = "*"

// IsOnline reports whether the agent was seen within window.
func (a *Agent) IsOnline(now time.Time, window time.Duration) bool {
	return !a.LastSeenAt.IsZero() && now.Sub(a.LastSeenAt) <= window
}

// HasCapability reports whether the agent advertises capability c.
func (a *Agent) HasCapability(c string) bool { return slices.Contains(a.Capabilities, c) }

// SetLabels applies add/remove to the agent's labels.
func (a *Agent) SetLabels(add, remove []string) error {
	next, err := MergeLabels(a.Labels, add, remove)
	if err != nil {
		return err
	}
	a.Labels = next
	return nil
}

// SetProfile changes cosmetic fields; nil leaves a field unchanged and an
// empty display name clears it.
// SetAllowFrom replaces who may send to the agent (empty = anyone). Exec
// nodes run what they receive as shell commands, so they must name senders
// unless acceptExecRisk explicitly opens them to every agent (stored as
// AnySender).
func (a *Agent) SetAllowFrom(names []AgentName, acceptExecRisk bool) error {
	if len(names) == 0 && a.Binding.Harness == HarnessExec {
		if !acceptExecRisk {
			return Errf(CodeInvalidArgument, "exec node %s must keep at least one allowed sender", a.Name).
				WithSuggestion("pass the agents that may run commands, e.g. --allow-from cloud-1; to let every agent run commands there, add --i-understand-exec-risk")
		}
		names = []AgentName{AnySender}
	}
	a.AllowFrom = names
	return nil
}

// MaxStatusText bounds the public status line.
const MaxStatusText = 280

// SetStatus publishes (or, with "", clears) the agent's status line.
func (a *Agent) SetStatus(text string, now time.Time) error {
	text = strings.TrimSpace(text)
	if len(text) > MaxStatusText {
		return Errf(CodeInvalidArgument, "status is %d bytes, limit is %d", len(text), MaxStatusText).
			WithSuggestion("keep it to one line; send details to the people who asked with `myco message progress`")
	}
	if strings.ContainsAny(text, "\r\n") {
		return Errf(CodeInvalidArgument, "status must be a single line")
	}
	a.StatusText = text
	a.StatusAt = now
	if text == "" {
		a.StatusAt = time.Time{}
	}
	return nil
}

func (a *Agent) SetProfile(displayName, description *string) error {
	if displayName != nil {
		d, err := ParseDisplayName(*displayName)
		if err != nil {
			return err
		}
		a.DisplayName = d
	}
	if description != nil {
		if len(*description) > maxDescription {
			return Errf(CodeInvalidArgument, "description longer than %d bytes", maxDescription)
		}
		a.Description = *description
	}
	return nil
}
