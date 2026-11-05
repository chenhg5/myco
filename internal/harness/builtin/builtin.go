// Package builtin assembles the registry of harnesses shipped with myco.
package builtin

import (
	"github.com/chenhg5/myco/internal/harness"
	"github.com/chenhg5/myco/internal/harness/claude"
	"github.com/chenhg5/myco/internal/harness/codex"
	"github.com/chenhg5/myco/internal/harness/execnode"
	"github.com/chenhg5/myco/internal/harness/shell"
	"github.com/chenhg5/myco/internal/harness/template"
)

// Registry returns every harness shipped with myco: native adapters
// (claude, codex), generic ones (shell, template) and template presets.
func Registry() *harness.Registry {
	r := harness.NewRegistry()
	r.Register(claude.Name, claude.New)
	r.Register(codex.Name, codex.New)
	r.Register(shell.Name, shell.New)
	r.Register(execnode.Name, execnode.New)
	r.Register(template.Name, template.New)
	for name, spec := range Presets {
		r.Register(name, template.Preset(name, spec))
	}
	return r
}

// Experimental lists presets whose CLI formats have not been verified.
var Experimental = map[string]bool{"cursor": true, "gemini": true, "opencode": true}
