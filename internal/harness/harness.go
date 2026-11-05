// Package harness abstracts the agent CLIs (Claude Code, Codex, any shell
// command) that a runner invokes to process a message. New harnesses plug in
// by implementing Harness and registering a Factory.
package harness

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// ErrSessionBusy means the session is in use by another process (typically a
// human in the interactive UI). Runners retry later instead of failing.
var ErrSessionBusy = errors.New("session is busy")

// BusyProber is implemented by harnesses that can tell when a session was
// last written by anyone (e.g. from its transcript file). The runner uses it
// to avoid writing into a session a human is using right now.
type BusyProber interface {
	// LastActivity returns the last time sessionID's transcript changed,
	// and false if the transcript cannot be found.
	LastActivity(sessionID string) (time.Time, bool)
}

// Request is one invocation of a harness.
type Request struct {
	// Prompt is the full text handed to the agent CLI.
	Prompt string
	// SessionID resumes a previous conversation when non-empty and the
	// harness supports sessions.
	SessionID string
	// Workdir is the working directory for the process.
	Workdir string
	// Env holds extra KEY=VALUE pairs (MYCO_* context) for the process.
	Env []string
}

// Result is the outcome of an invocation.
type Result struct {
	// Output is the agent's final answer.
	Output string
	// SessionID identifies the conversation for later resumption ("" if
	// unsupported).
	SessionID string
}

// Harness runs an agent CLI.
type Harness interface {
	Name() string
	Run(ctx context.Context, req Request) (Result, error)
}

// Options configures a harness instance.
type Options struct {
	// Binary overrides the executable (e.g. a full path to claude).
	Binary string
	// Args are extra arguments appended to the harness invocation.
	Args []string
	// Command is the shell command line for the "shell" harness.
	Command string
	// Model selects the model, in the CLI's own naming ("" = CLI default).
	Model string
	// Access is the permission level; each harness maps it to CLI flags.
	Access Access
	// Template describes a CLI declaratively (the "template" harness and
	// the built-in presets).
	Template *TemplateSpec
}

// Access is a harness-independent permission level.
type Access string

const (
	// AccessDefault leaves the CLI's own headless defaults untouched.
	AccessDefault Access = ""
	// AccessReadonly: read files, no edits, no shell side effects.
	AccessReadonly Access = "readonly"
	// AccessWorkspace: edit inside the working directory (sandboxed where
	// the CLI supports it).
	AccessWorkspace Access = "workspace"
	// AccessFull: no approvals and no sandbox. Only for isolated machines.
	AccessFull Access = "full"
)

// ParseAccess validates an access level.
func ParseAccess(s string) (Access, error) {
	switch a := Access(s); a {
	case AccessDefault, AccessReadonly, AccessWorkspace, AccessFull:
		return a, nil
	}
	return "", fmt.Errorf("invalid access %q (use readonly, workspace or full)", s)
}

// TemplateSpec declares how to drive a CLI without writing an adapter.
// Placeholders in Args/ResumeArgs: {prompt}, {session}, {workdir}, {model}.
// If no argument contains {prompt}, the prompt is written to stdin.
type TemplateSpec struct {
	Binary string `yaml:"binary,omitempty" json:"binary,omitempty"`
	// Args start a new session; ResumeArgs continue {session} (optional:
	// without it every message starts a fresh session).
	Args       []string `yaml:"args,omitempty" json:"args,omitempty"`
	ResumeArgs []string `yaml:"resume_args,omitempty" json:"resume_args,omitempty"`
	// ModelArgs are appended when a model is set, e.g. ["--model","{model}"].
	ModelArgs []string `yaml:"model_args,omitempty" json:"model_args,omitempty"`
	// AccessArgs maps an access level to extra arguments.
	AccessArgs map[Access][]string `yaml:"access_args,omitempty" json:"access_args,omitempty"`
	// Output is text (stdout is the answer), json (one object) or jsonl.
	Output string `yaml:"output,omitempty" json:"output,omitempty"`
	// Extraction rules for json/jsonl output.
	Text    *Extract `yaml:"text,omitempty" json:"text,omitempty"`
	Session *Extract `yaml:"session,omitempty" json:"session,omitempty"`
	Error   *Extract `yaml:"error,omitempty" json:"error,omitempty"`
	// SessionPattern extracts the session id from text output (regexp
	// with one group), e.g. "session: (\\S+)".
	SessionPattern string `yaml:"session_pattern,omitempty" json:"session_pattern,omitempty"`
	// VersionArgs prints the version for `myco doctor` (default --version).
	VersionArgs []string `yaml:"version_args,omitempty" json:"version_args,omitempty"`
	// Transcripts are glob patterns (with %s = session id, ~ = home) used
	// to detect a session being used elsewhere.
	Transcripts []string `yaml:"transcripts,omitempty" json:"transcripts,omitempty"`
}

// Extract picks a value from JSON events. Path is dotted (a.b.c); a value
// that is an array of content blocks is reduced to their text. When lists
// field=value conditions an event must satisfy. Append concatenates matches
// instead of keeping the last one.
type Extract struct {
	Path   string            `yaml:"path" json:"path"`
	When   map[string]string `yaml:"when,omitempty" json:"when,omitempty"`
	Append bool              `yaml:"append,omitempty" json:"append,omitempty"`
}

// Factory constructs a harness from options.
type Factory func(Options) (Harness, error)

// Registry maps harness names to factories.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{factories: map[string]Factory{}} }

// Register adds a factory under name.
func (r *Registry) Register(name string, f Factory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[name] = f
}

// New builds the named harness.
func (r *Registry) New(name string, opts Options) (Harness, error) {
	r.mu.RLock()
	f, ok := r.factories[name]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown harness %q (available: %v)", name, r.Names())
	}
	return f(opts)
}

// Names lists registered harness names, sorted.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.factories))
	for n := range r.factories {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
