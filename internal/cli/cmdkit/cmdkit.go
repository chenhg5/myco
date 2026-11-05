// Package cmdkit is a small, dependency-free command framework designed for
// agent-friendly CLIs: noun-verb trees, long flags only, strict validation
// with "did you mean" hints, and help/schema generated from one definition.
package cmdkit

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Kind is a flag's value type.
type Kind string

const (
	String   Kind = "string"
	Bool     Kind = "bool"
	Int      Kind = "int"
	Duration Kind = "duration"
	List     Kind = "list" // repeatable and/or comma-separated
	Enum     Kind = "enum"
)

// Flag declares a long flag.
type Flag struct {
	Name       string   `json:"name"`
	Kind       Kind     `json:"type"`
	Help       string   `json:"description"`
	Default    string   `json:"default,omitempty"`
	Required   bool     `json:"required"`
	Values     []string `json:"enum,omitempty"`
	Env        string   `json:"env,omitempty"`
	Persistent bool     `json:"-"` // inherited by subcommands
}

// Command is a node in the command tree.
type Command struct {
	Name     string
	Summary  string
	Long     string
	Examples []string
	Flags    []Flag
	// Mutating commands get --dry-run automatically.
	Mutating bool
	// Args documents accepted positional arguments ("" = none allowed).
	Args string
	Subs []*Command
	Run  func(*Invocation) error

	parent *Command
}

// Add attaches subcommands and returns c.
func (c *Command) Add(subs ...*Command) *Command {
	for _, s := range subs {
		s.parent = c
		c.Subs = append(c.Subs, s)
	}
	return c
}

// Path is the full command path, e.g. "myco message send".
func (c *Command) Path() string {
	if c.parent == nil {
		return c.Name
	}
	return c.parent.Path() + " " + c.Name
}

// Find returns the subcommand named name.
func (c *Command) Find(name string) *Command {
	for _, s := range c.Subs {
		if s.Name == name {
			return s
		}
	}
	return nil
}

// AllFlags returns own flags, the implicit --dry-run, and inherited persistent flags.
func (c *Command) AllFlags() []Flag {
	var out []Flag
	out = append(out, c.Flags...)
	if c.Mutating {
		out = append(out, Flag{Name: "dry-run", Kind: Bool, Help: "Validate and show what would happen without doing it (exit 10)"})
	}
	for p := c.parent; p != nil; p = p.parent {
		for _, f := range p.Flags {
			if f.Persistent {
				out = append(out, f)
			}
		}
	}
	return out
}

func (c *Command) flag(name string) (Flag, bool) {
	for _, f := range c.AllFlags() {
		if f.Name == name {
			return f, true
		}
	}
	return Flag{}, false
}

// UsageError is an invalid invocation (maps to exit code 2).
type UsageError struct {
	Message    string
	Suggestion string
	Command    string
}

func (e *UsageError) Error() string { return e.Message }

// HelpRequest signals that help was printed.
type HelpRequest struct{ Command *Command }

func (HelpRequest) Error() string { return "help requested" }

// Invocation is a parsed command call.
type Invocation struct {
	Ctx    context.Context
	Cmd    *Command
	Args   []string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Getenv func(string) string

	values map[string][]string
}

// Resolve walks args to the target command and parses its flags.
func Resolve(root *Command, args []string) (*Command, map[string][]string, []string, error) {
	cmd := root
	i := 0
	for i < len(args) {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			break
		}
		if a == "help" && cmd == root {
			// "myco help message send" == "myco message send --help"
			target := root
			for _, n := range args[i+1:] {
				if s := target.Find(n); s != nil {
					target = s
				}
			}
			return target, nil, nil, HelpRequest{Command: target}
		}
		sub := cmd.Find(a)
		if sub == nil {
			if len(cmd.Subs) > 0 {
				return cmd, nil, nil, &UsageError{
					Message:    fmt.Sprintf("unknown command %q for %s", a, cmd.Path()),
					Suggestion: suggest(a, subNames(cmd), "run: "+cmd.Path()+" --help"),
					Command:    cmd.Path(),
				}
			}
			break // positional args for a leaf
		}
		cmd = sub
		i++
	}
	values := map[string][]string{}
	var positional []string
	rest := args[i:]
	for j := 0; j < len(rest); j++ {
		a := rest[j]
		if a == "--" {
			positional = append(positional, rest[j+1:]...)
			break
		}
		if a == "-h" || a == "--help" {
			return cmd, nil, nil, HelpRequest{Command: cmd}
		}
		if !strings.HasPrefix(a, "--") {
			if strings.HasPrefix(a, "-") && a != "-" {
				return cmd, nil, nil, &UsageError{Message: fmt.Sprintf("short flag %q is not supported", a),
					Suggestion: "use long flags; see " + cmd.Path() + " --help", Command: cmd.Path()}
			}
			positional = append(positional, a)
			continue
		}
		name, val, hasVal := strings.Cut(strings.TrimPrefix(a, "--"), "=")
		f, ok := cmd.flag(name)
		if !ok {
			if strings.HasPrefix(name, "no-") {
				if bf, ok := cmd.flag(strings.TrimPrefix(name, "no-")); ok && bf.Kind == Bool && !hasVal {
					values[bf.Name] = []string{"false"}
					continue
				}
			}
			return cmd, nil, nil, &UsageError{
				Message:    fmt.Sprintf("unknown flag --%s for %s", name, cmd.Path()),
				Suggestion: suggest(name, flagNames(cmd), "run: "+cmd.Path()+" --help"),
				Command:    cmd.Path(),
			}
		}
		if f.Kind == Bool && !hasVal {
			val = "true"
		} else if !hasVal {
			if j+1 >= len(rest) {
				return cmd, nil, nil, &UsageError{Message: fmt.Sprintf("flag --%s needs a value", name),
					Suggestion: fmt.Sprintf("pass --%s <%s>", name, valueHint(f)), Command: cmd.Path()}
			}
			j++
			val = rest[j]
		}
		if err := validate(f, val); err != nil {
			return cmd, nil, nil, &UsageError{Message: err.Error(), Suggestion: fmt.Sprintf("--%s expects %s", name, valueHint(f)), Command: cmd.Path()}
		}
		if f.Kind == List {
			for _, part := range strings.Split(val, ",") {
				if p := strings.TrimSpace(part); p != "" {
					values[name] = append(values[name], p)
				}
			}
		} else {
			values[name] = []string{val}
		}
	}
	if cmd.Run == nil {
		return cmd, nil, nil, HelpRequest{Command: cmd}
	}
	if len(positional) > 0 && cmd.Args == "" {
		return cmd, nil, nil, &UsageError{
			Message:    fmt.Sprintf("unexpected argument %q for %s", positional[0], cmd.Path()),
			Suggestion: "values are passed as long flags; see " + cmd.Path() + " --help",
			Command:    cmd.Path(),
		}
	}
	return cmd, values, positional, nil
}

func validate(f Flag, v string) error {
	switch f.Kind {
	case Bool:
		if _, err := strconv.ParseBool(v); err != nil {
			return fmt.Errorf("invalid boolean %q for --%s", v, f.Name)
		}
	case Int:
		if _, err := strconv.Atoi(v); err != nil {
			return fmt.Errorf("invalid integer %q for --%s", v, f.Name)
		}
	case Duration:
		if _, err := time.ParseDuration(v); err != nil {
			return fmt.Errorf("invalid duration %q for --%s", v, f.Name)
		}
	case Enum:
		for _, e := range f.Values {
			if e == v {
				return nil
			}
		}
		return fmt.Errorf("invalid value %q for --%s", v, f.Name)
	}
	return nil
}

func valueHint(f Flag) string {
	switch f.Kind {
	case Enum:
		return strings.Join(f.Values, "|")
	case Duration:
		return "duration, e.g. 30s, 5m"
	case List:
		return "value[,value...]"
	}
	return string(f.Kind)
}

// NewInvocation builds an invocation from parsed values.
func NewInvocation(ctx context.Context, cmd *Command, values map[string][]string, args []string, stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string) *Invocation {
	return &Invocation{Ctx: ctx, Cmd: cmd, Args: args, Stdin: stdin, Stdout: stdout, Stderr: stderr, Getenv: getenv, values: values}
}

// IsSet reports whether the flag was given explicitly.
func (in *Invocation) IsSet(name string) bool { _, ok := in.values[name]; return ok }

func (in *Invocation) raw(name string) string {
	if v, ok := in.values[name]; ok && len(v) > 0 {
		return v[len(v)-1]
	}
	f, _ := in.Cmd.flag(name)
	if f.Env != "" && in.Getenv != nil {
		if v := in.Getenv(f.Env); v != "" {
			return v
		}
	}
	return f.Default
}

// String returns a flag value (flag, then env, then default).
func (in *Invocation) String(name string) string { return in.raw(name) }

// Bool returns a boolean flag.
func (in *Invocation) Bool(name string) bool { b, _ := strconv.ParseBool(in.raw(name)); return b }

// Int returns an integer flag.
func (in *Invocation) Int(name string) int { n, _ := strconv.Atoi(in.raw(name)); return n }

// Duration returns a duration flag.
func (in *Invocation) Duration(name string) time.Duration {
	d, _ := time.ParseDuration(in.raw(name))
	return d
}

// List returns a list flag.
func (in *Invocation) List(name string) []string {
	if v, ok := in.values[name]; ok {
		return v
	}
	if d := in.raw(name); d != "" {
		return strings.Split(d, ",")
	}
	return nil
}

// Require returns a UsageError if any named flag is empty.
func (in *Invocation) Require(names ...string) error {
	for _, n := range names {
		if strings.TrimSpace(in.String(n)) == "" {
			f, _ := in.Cmd.flag(n)
			hint := fmt.Sprintf("pass --%s <%s>", n, valueHint(f))
			if f.Env != "" {
				hint += " or set " + f.Env
			}
			return &UsageError{Message: fmt.Sprintf("missing required flag --%s", n), Suggestion: hint, Command: in.Cmd.Path()}
		}
	}
	return nil
}

func subNames(c *Command) []string {
	var out []string
	for _, s := range c.Subs {
		out = append(out, s.Name)
	}
	return out
}

func flagNames(c *Command) []string {
	var out []string
	for _, f := range c.AllFlags() {
		out = append(out, f.Name)
	}
	return out
}

// suggest returns a "did you mean" hint or fallback.
func suggest(got string, options []string, fallback string) string {
	best, bestD := "", 1<<30
	for _, o := range options {
		if d := levenshtein(got, o); d < bestD {
			best, bestD = o, d
		}
	}
	if best != "" && bestD <= max(2, len(got)/3) {
		return fmt.Sprintf("did you mean %q? (%s)", best, fallback)
	}
	sort.Strings(options)
	return fmt.Sprintf("valid options: %s (%s)", strings.Join(options, ", "), fallback)
}

func levenshtein(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}
