// Package builtin assembles the registry of harnesses shipped with myco.
package builtin

import (
	"github.com/chenhg5/myco/internal/harness"
	"github.com/chenhg5/myco/internal/harness/claude"
	"github.com/chenhg5/myco/internal/harness/codex"
	"github.com/chenhg5/myco/internal/harness/shell"
)

// Registry returns a registry with claude, codex and shell harnesses.
func Registry() *harness.Registry {
	r := harness.NewRegistry()
	r.Register(claude.Name, claude.New)
	r.Register(codex.Name, codex.New)
	r.Register(shell.Name, shell.New)
	return r
}
