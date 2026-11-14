// Package template drives any agent CLI from a declarative TemplateSpec:
// argv templates, text/json/jsonl output and field extraction rules. The
// built-in presets (pi, kiro, cursor, gemini, opencode) are TemplateSpecs.
package template

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/chenhg5/myco/internal/harness"
	"github.com/chenhg5/myco/internal/osutil"
)

// Name is the registry key of the user-configurable template harness.
const Name = "template"

type tmpl struct {
	name string
	spec harness.TemplateSpec
	opts harness.Options
	*harness.TranscriptFinder
	sessionRe *regexp.Regexp
}

// New builds the user-configured template harness (Options.Template).
func New(opts harness.Options) (harness.Harness, error) {
	if opts.Template == nil {
		return nil, errors.New("template harness needs runner.template in the agent config")
	}
	return Build(Name, *opts.Template, opts)
}

// Preset returns a Factory for a named built-in spec; a user template in
// Options overrides individual fields.
func Preset(name string, spec harness.TemplateSpec) harness.Factory {
	return func(opts harness.Options) (harness.Harness, error) {
		if opts.Template != nil {
			spec = merge(spec, *opts.Template)
		}
		return Build(name, spec, opts)
	}
}

// Build validates spec and returns a harness.
func Build(name string, spec harness.TemplateSpec, opts harness.Options) (harness.Harness, error) {
	if opts.Binary != "" {
		spec.Binary = opts.Binary
	}
	if spec.Binary == "" || len(spec.Args) == 0 {
		return nil, fmt.Errorf("%s: template needs binary and args", name)
	}
	switch spec.Output {
	case "", "text", "json", "jsonl":
	default:
		return nil, fmt.Errorf("%s: output must be text, json or jsonl", name)
	}
	t := &tmpl{name: name, spec: spec, opts: opts}
	if spec.SessionPattern != "" {
		re, err := regexp.Compile(spec.SessionPattern)
		if err != nil || re.NumSubexp() != 1 {
			return nil, fmt.Errorf("%s: session_pattern must be a regexp with one group", name)
		}
		t.sessionRe = re
	}
	if len(spec.Transcripts) > 0 {
		t.TranscriptFinder = &harness.TranscriptFinder{Patterns: func(id string) []string {
			home, _ := os.UserHomeDir()
			var out []string
			for _, p := range spec.Transcripts {
				p = strings.ReplaceAll(p, "%s", id)
				p = osutil.ExpandHome(p, home)
				out = append(out, p)
			}
			return out
		}}
	}
	return t, nil
}

func merge(base, over harness.TemplateSpec) harness.TemplateSpec {
	b, _ := json.Marshal(over)
	_ = json.Unmarshal(b, &base) // non-empty fields of over win
	return base
}

func (t *tmpl) Name() string { return t.name }

// LastActivity makes the harness a BusyProber only when transcripts are set.
func (t *tmpl) LastActivity(id string) (ts time.Time, ok bool) {
	if t.TranscriptFinder == nil {
		return ts, false
	}
	return t.TranscriptFinder.LastActivity(id)
}

func (t *tmpl) Check(ctx context.Context) harness.Check {
	return harness.CheckBinary(ctx, t.spec.Binary, t.spec.VersionArgs...)
}

// argv expands the templates for one call.
func (t *tmpl) argv(req harness.Request) (args []string, stdin string, resumed bool) {
	base := t.spec.Args
	if req.SessionID != "" && len(t.spec.ResumeArgs) > 0 {
		base, resumed = t.spec.ResumeArgs, true
	}
	vars := map[string]string{"{session}": req.SessionID, "{workdir}": req.Workdir, "{model}": t.opts.Model}
	usesPrompt := false
	expand := func(list []string) {
		for _, a := range list {
			if strings.Contains(a, "{prompt}") {
				usesPrompt = true
				a = strings.ReplaceAll(a, "{prompt}", req.Prompt)
			}
			for k, v := range vars {
				a = strings.ReplaceAll(a, k, v)
			}
			args = append(args, a)
		}
	}
	// Options first so a trailing positional {prompt} stays last.
	var pre []string
	if t.opts.Model != "" {
		pre = append(pre, t.spec.ModelArgs...)
	}
	pre = append(pre, t.spec.AccessArgs[req.AccessOr(t.opts.Access)]...)
	pre = append(pre, t.opts.Args...)
	if n := len(base); n > 0 && strings.Contains(base[n-1], "{prompt}") {
		expand(base[:n-1])
		expand(pre)
		expand(base[n-1:])
	} else {
		expand(base)
		expand(pre)
	}
	if !usesPrompt {
		stdin = req.Prompt
	}
	return args, stdin, resumed
}

func (t *tmpl) Run(ctx context.Context, req harness.Request) (harness.Result, error) {
	args, stdin, resumed := t.argv(req)
	out, runErr := harness.Exec(ctx, harness.Process{
		Binary: t.spec.Binary, Args: args, Stdin: stdin, Workdir: req.Workdir, Env: req.Env,
	})
	res, failure := t.parse(out.Stdout)
	if res.SessionID == "" && resumed {
		res.SessionID = req.SessionID
	}
	switch {
	case failure != "" && strings.TrimSpace(res.Output) == "":
		return res, fmt.Errorf("%s: %s", t.name, failure)
	case runErr != nil && strings.TrimSpace(res.Output) == "":
		return res, runErr
	}
	return res, nil
}

// parse applies the extraction rules to stdout.
func (t *tmpl) parse(stdout string) (harness.Result, string) {
	var res harness.Result
	switch t.spec.Output {
	case "", "text":
		res.Output = strings.TrimSpace(stdout)
		if t.sessionRe != nil {
			if m := t.sessionRe.FindStringSubmatch(stdout); m != nil {
				res.SessionID = m[1]
			}
		}
		return res, ""
	}
	var events []map[string]any
	if t.spec.Output == "json" {
		var v map[string]any
		if json.Unmarshal([]byte(strings.TrimSpace(stdout)), &v) == nil {
			events = append(events, v)
		}
	} else {
		sc := bufio.NewScanner(strings.NewReader(stdout))
		sc.Buffer(make([]byte, 1<<20), 32<<20)
		for sc.Scan() {
			var v map[string]any
			if json.Unmarshal(sc.Bytes(), &v) == nil {
				events = append(events, v)
			}
		}
	}
	var failure string
	for _, ev := range events {
		if s := pick(ev, t.spec.Session); s != "" {
			res.SessionID = s
		}
		if s := pick(ev, t.spec.Text); s != "" {
			if t.spec.Text.Append && res.Output != "" {
				res.Output += "\n" + s
			} else {
				res.Output = s
			}
		}
		if s := pick(ev, t.spec.Error); s != "" {
			failure = s
		}
	}
	return res, failure
}

// pick returns the extracted string if ev satisfies the rule.
func pick(ev map[string]any, x *harness.Extract) string {
	if x == nil || x.Path == "" {
		return ""
	}
	for k, want := range x.When {
		if fmt.Sprint(lookup(ev, k)) != want {
			return ""
		}
	}
	return strings.TrimSpace(textOf(lookup(ev, x.Path)))
}

func lookup(v any, path string) any {
	for _, p := range strings.Split(path, ".") {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = m[p]
	}
	return v
}

// textOf turns strings, {"text":...} blocks and arrays of blocks into text.
func textOf(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case map[string]any:
		if t, ok := x["text"].(string); ok && (x["type"] == nil || x["type"] == "text") {
			return t
		}
		return ""
	case []any:
		var parts []string
		for _, e := range x {
			if s := textOf(e); strings.TrimSpace(s) != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, "\n")
	case float64, bool:
		return fmt.Sprint(x)
	}
	return ""
}
