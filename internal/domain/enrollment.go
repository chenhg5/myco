package domain

import (
	"strings"
	"time"
	"unicode"
)

// HarnessExec is the harness of exec nodes (shell command execution).
const HarnessExec = "exec"

// Enrollment is a limited, shareable credential that lets a session register
// itself as an agent (or recover its agent after a restart) without the admin
// token. It is meant to live in an environment's secrets, so every session
// started there can join the network on its own.
//
// What an enrollment can do is deliberately small:
//   - register only the *calling* session (binding.session_id is required);
//   - only under NamePrefix, optionally only for one Harness;
//   - only with pull delivery (no URLs/secrets the hub would call);
//   - recover only agents that were created through the same enrollment.
type Enrollment struct {
	ID           string
	TokenHash    string
	NamePrefix   string
	Harness      string
	AllowFrom    []AgentName
	Capabilities []string
	// Labels are given to every agent enrolled with this token.
	Labels    []string
	Note      string
	MaxUses   int // 0 = unlimited
	Uses      int
	ExpiresAt time.Time // zero = never
	RevokedAt time.Time // zero = active
	CreatedAt time.Time
}

// EnrollmentSpec configures a new enrollment.
type EnrollmentSpec struct {
	NamePrefix   string
	Harness      string
	AllowFrom    []AgentName
	Capabilities []string
	Labels       []string
	Note         string
	MaxUses      int
	TTL          time.Duration
}

// NewEnrollment validates and builds an enrollment.
func NewEnrollment(id, tokenHash string, s EnrollmentSpec, now time.Time) (*Enrollment, error) {
	if s.NamePrefix == "" || !namePattern.MatchString(s.NamePrefix+"x") || len(s.NamePrefix) > 32 {
		return nil, Errf(CodeInvalidArgument, "invalid name prefix %q", s.NamePrefix).
			WithSuggestion("use a short lowercase prefix such as cloud-")
	}
	if s.Harness != "" && !namePattern.MatchString(s.Harness) {
		return nil, Errf(CodeInvalidArgument, "invalid harness %q", s.Harness)
	}
	if s.Harness == HarnessExec && len(s.AllowFrom) == 0 {
		return nil, Errf(CodeInvalidArgument, "an exec enrollment must name the agents allowed to send commands").
			WithSuggestion("add --allow-from <agent>[,<agent>...]; exec nodes run every task they receive as a shell command")
	}
	if s.MaxUses < 0 || s.TTL < 0 {
		return nil, Errf(CodeInvalidArgument, "max uses and TTL must not be negative")
	}
	if _, err := ParseLabels(s.Labels); err != nil {
		return nil, err
	}
	if len(s.Note) > 500 {
		return nil, Errf(CodeInvalidArgument, "note longer than 500 bytes")
	}
	e := &Enrollment{ID: id, TokenHash: tokenHash, NamePrefix: s.NamePrefix, Harness: s.Harness,
		AllowFrom: s.AllowFrom, Capabilities: s.Capabilities, Labels: s.Labels, Note: s.Note, MaxUses: s.MaxUses, CreatedAt: now}
	if s.TTL > 0 {
		e.ExpiresAt = now.Add(s.TTL)
	}
	return e, nil
}

// Usable reports why the enrollment cannot be used right now (nil if it can).
// Recovering an existing agent does not need a free use.
func (e *Enrollment) Usable(now time.Time, needUse bool) error {
	switch {
	case !e.RevokedAt.IsZero():
		return Errf(CodePermissionDenied, "enrollment %s was revoked", e.ID)
	case !e.ExpiresAt.IsZero() && now.After(e.ExpiresAt):
		return Errf(CodePermissionDenied, "enrollment %s expired at %s", e.ID, e.ExpiresAt.Format(time.RFC3339)).
			WithSuggestion("ask the hub admin for a new enrollment token")
	case needUse && e.MaxUses > 0 && e.Uses >= e.MaxUses:
		return Errf(CodePermissionDenied, "enrollment %s has no uses left (%d/%d)", e.ID, e.Uses, e.MaxUses)
	}
	return nil
}

// Revoke disables the enrollment.
func (e *Enrollment) Revoke(now time.Time) {
	if e.RevokedAt.IsZero() {
		e.RevokedAt = now
	}
}

// Admit checks that a binding may enroll under this enrollment.
func (e *Enrollment) Admit(b Binding) error {
	if err := b.Validate(); err != nil {
		return err
	}
	if b.SessionID == "" {
		return Errf(CodeInvalidArgument, "enrolling requires the calling session's id").
			WithSuggestion("run inside the session with --detect, or pass --session-id")
	}
	if e.Harness != "" && b.Harness != e.Harness {
		return Errf(CodePermissionDenied, "enrollment %s only admits harness %s, got %q", e.ID, e.Harness, b.Harness)
	}
	// Exec nodes execute commands, so only a token created for them (which
	// carries an allow_from list) may create one.
	if b.Harness == HarnessExec && e.Harness != HarnessExec {
		return Errf(CodePermissionDenied, "enrollment %s cannot create exec nodes", e.ID).
			WithSuggestion("the hub admin must create one for exec: myco enrollment create --harness exec --name-prefix <p> --allow-from <agent>")
	}
	return nil
}

// NameFor returns the agent name to use: the requested one (which must carry
// the prefix) or prefix + a short slug of the session id.
func (e *Enrollment) NameFor(requested, sessionID string) (AgentName, error) {
	if requested != "" {
		if !strings.HasPrefix(requested, e.NamePrefix) {
			return "", Errf(CodePermissionDenied, "name %q must start with %q under this enrollment", requested, e.NamePrefix)
		}
		return ParseAgentName(requested)
	}
	return ParseAgentName(e.NamePrefix + sessionSlug(sessionID))
}

// sessionSlug keeps the last 8 alphanumerics of a session id, lowercased.
func sessionSlug(id string) string {
	var r []rune
	for _, c := range id {
		if unicode.IsLetter(c) || unicode.IsDigit(c) {
			r = append(r, unicode.ToLower(c))
		}
	}
	if len(r) > 8 {
		r = r[len(r)-8:]
	}
	if len(r) == 0 {
		return "agent"
	}
	return string(r)
}
