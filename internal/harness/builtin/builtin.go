// Package builtin assembles the registry of harnesses shipped with myco.
package builtin

import (
	"github.com/chenhg5/myco/internal/harness"
	"github.com/chenhg5/myco/internal/harness/acp"
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
	r.Register(acp.Name, acp.New)
	for name, spec := range Presets {
		r.Register(name, template.Preset(name, spec))
	}
	for name, spec := range ACPPresets {
		r.Register(name, acpOrTemplate(name, spec))
	}
	return r
}

// acpOrTemplate drives the CLI over ACP, unless the agent's config describes
// it with runner.template (how these CLIs were configured before ACP).
func acpOrTemplate(name string, spec acp.Spec) harness.Factory {
	viaACP := acp.Preset(name, spec)
	viaTemplate := template.Preset(name, legacyTemplates[name])
	return func(opts harness.Options) (harness.Harness, error) {
		if opts.Template != nil {
			return viaTemplate(opts)
		}
		return viaACP(opts)
	}
}

// Experimental lists presets whose CLI formats have not been verified.
var Experimental = map[string]bool{"cursor": true, "gemini": true, "opencode": true}
