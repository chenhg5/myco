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
)

// ErrSessionBusy means the session is in use by another process (typically a
// human in the interactive UI). Runners retry later instead of failing.
var ErrSessionBusy = errors.New("session is busy")

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
	// Args are extra arguments appended to the harness invocation
	// (e.g. permission flags).
	Args []string
	// Command is the shell command line for the "shell" harness.
	Command string
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
