package cmdkit

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// WriteHelp renders concise help: examples first, then flags or subcommands.
func WriteHelp(w io.Writer, c *Command, footer string) {
	fmt.Fprintf(w, "%s — %s\n", c.Path(), c.Summary)
	if c.Long != "" {
		fmt.Fprintf(w, "\n%s\n", strings.TrimSpace(c.Long))
	}
	if len(c.Examples) > 0 {
		fmt.Fprintln(w, "\nExamples:")
		for _, e := range c.Examples {
			fmt.Fprintf(w, "  %s\n", e)
		}
	}
	if len(c.Subs) > 0 {
		fmt.Fprintln(w, "\nCommands:")
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, s := range c.Subs {
			fmt.Fprintf(tw, "  %s\t%s\n", s.Name, s.Summary)
		}
		tw.Flush()
		fmt.Fprintf(w, "\nRun '%s <command> --help' for details.\n", c.Path())
	}
	if c.Run != nil {
		if c.Args != "" {
			fmt.Fprintf(w, "\nArguments:\n  %s\n", c.Args)
		}
		own := c.Flags
		if c.Mutating {
			own = append(append([]Flag{}, own...), Flag{Name: "dry-run", Kind: Bool, Help: "Validate and preview without side effects (exit 10)"})
		}
		if len(own) > 0 {
			fmt.Fprintln(w, "\nFlags:")
			writeFlags(w, own)
		}
	}
	var global []Flag
	for p := c; p != nil; p = p.parent {
		for _, f := range p.Flags {
			if f.Persistent {
				global = append(global, f)
			}
		}
	}
	if len(global) > 0 {
		fmt.Fprintln(w, "\nGlobal flags:")
		writeFlags(w, global)
	}
	if footer != "" {
		fmt.Fprintf(w, "\n%s\n", strings.TrimSpace(footer))
	}
}

func writeFlags(w io.Writer, flags []Flag) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, f := range flags {
		var meta []string
		if f.Kind != Bool {
			meta = append(meta, valueHint(f))
		}
		if f.Required {
			meta = append(meta, "required")
		}
		if f.Default != "" {
			meta = append(meta, "default: "+f.Default)
		}
		if f.Env != "" {
			meta = append(meta, "env: "+f.Env)
		}
		m := ""
		if len(meta) > 0 {
			m = " <" + strings.Join(meta, ", ") + ">"
		}
		fmt.Fprintf(tw, "  --%s%s\t%s\n", f.Name, m, f.Help)
	}
	tw.Flush()
}

// Schema is the machine-readable description of a command.
type Schema struct {
	Command     string   `json:"command"`
	Summary     string   `json:"summary"`
	Description string   `json:"description,omitempty"`
	Mutating    bool     `json:"mutating"`
	Args        string   `json:"args,omitempty"`
	Flags       []Flag   `json:"flags"`
	Examples    []string `json:"examples,omitempty"`
	Subcommands []string `json:"subcommands,omitempty"`
}

// SchemaOf describes c.
func SchemaOf(c *Command) Schema {
	s := Schema{Command: c.Path(), Summary: c.Summary, Description: strings.TrimSpace(c.Long),
		Mutating: c.Mutating, Args: c.Args, Examples: c.Examples, Flags: []Flag{}}
	if c.Run != nil {
		s.Flags = c.AllFlags()
	}
	for _, sub := range c.Subs {
		s.Subcommands = append(s.Subcommands, sub.Name)
	}
	return s
}

// Walk visits c and all descendants depth-first.
func Walk(c *Command, fn func(*Command)) {
	fn(c)
	for _, s := range c.Subs {
		Walk(s, fn)
	}
}
