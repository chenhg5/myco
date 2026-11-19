package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/chenhg5/myco/internal/api"
	. "github.com/chenhg5/myco/internal/cli/cmdkit"
	"github.com/chenhg5/myco/internal/client"
)

// workflowCommands: workflows the hub walks, handing each step to an agent,
// a computer or a person (docs/design/workflow.md). Agents use their own
// token (acting for their person); people their admin or member token.
func workflowCommands() *Command {
	adminFlag := Flag{Name: "admin-token-file", Kind: String, Help: "File containing the admin token (people; agents use their own)"}
	fileFlag := Flag{Name: "file", Kind: String, Required: true, Help: "The workflow's JSON ('-' = stdin)"}
	nameFlag := Flag{Name: "name", Kind: String, Required: true, Help: "The workflow's name (or id)"}
	runFlag := Flag{Name: "id", Kind: String, Required: true, Help: "The run's id (run_...)"}

	validate := &Command{
		Name:    "validate",
		Summary: "Check a workflow's JSON (and that its agents exist) without saving it",
		Long: `A workflow is JSON:

  {"name": "写-审-改", "project": "myco",
   "inputs": {"required": ["task"], "properties": {"task": {"type": "string", "title": "要做什么"}}},
   "steps": [
     {"id": "write", "agent": "coder", "prompt": "完成：{{inputs.task}}",
      "output": {"required": ["branch"], "properties": {"branch": {"type": "string"}}}, "timeout": "2h"},
     {"id": "review", "agent": "reviewer", "prompt": "审一下 {{steps.write.output.branch}}",
      "output": {"required": ["approved"], "properties": {"approved": {"type": "boolean"}, "comments": {"type": "array"}}}},
     {"id": "test", "computer": "studio-mac", "run": "make test"},
     {"id": "ok", "human": "$starter", "prompt": "合并 {{steps.write.output.branch}} 吗？", "options": ["合并", "不合并"]}],
   "edges": [
     {"from": "write", "to": "review"},
     {"from": "review", "to": "test", "when": "output.approved"},
     {"from": "review", "to": "write", "when": "!output.approved", "maxIterations": 3, "else": "$fail"},
     {"from": "test", "to": "ok"},
     {"from": "test", "to": "write", "onError": true, "maxIterations": 2},
     {"from": "ok", "to": "$end"}]}

Steps: agent (a task; output is the JSON shape its result must have),
computer (a command on an exec node; fails on a non-zero exit), human (a
decision in the App; options are the choices; unanswered by its timeout,
default 3d, it is a "no": choice is ""). Each step: title, timeout (30m, 2h,
1d), retry (0-5), dispatch "all" (take every matching edge, in parallel),
join "all" (start when every step leading here has arrived).

Edges: the first whose when holds is taken; when sees output (this step's
result), inputs, steps.<id>.output/.summary, status. onError edges are taken
only when the step failed. An edge back to an earlier step needs
maxIterations; past it, else is taken ($fail when unset). With no edges at
all, steps run in order. $end ends a branch, $fail fails the run.

{{expr}} in prompt, run and title puts a value in; {{#if expr}}…{{else}}…{{/if}}.
Expressions: paths, == != < <= > >= && || !, len(x), contains(x, y).`,
		Examples: []string{"myco workflow validate --file wf.json"},
		Flags:    []Flag{fileFlag, adminFlag},
		Run: handler(func(e *env) error {
			def, err := e.readWorkflowFile()
			if err != nil {
				return err
			}
			c, err := e.projectClient()
			if err != nil {
				return err
			}
			chk, err := c.ValidateWorkflow(e.in.Ctx, def)
			if err != nil {
				return err
			}
			if err := e.p.Print(chk, func(w io.Writer) {
				if chk.Valid {
					fmt.Fprintln(w, "valid")
				}
				for _, p := range chk.Problems {
					fmt.Fprintln(w, "problem: "+p)
				}
				for _, p := range chk.Warnings {
					fmt.Fprintln(w, "warning: "+p)
				}
			}); err != nil {
				return err
			}
			if !chk.Valid {
				return errorf("invalid_argument", ExitUsage, "fix the problems and validate again", "the workflow has %d problem(s)", len(chk.Problems))
			}
			return nil
		}),
	}
	apply := &Command{
		Name:     "apply",
		Summary:  "Save a workflow: create it, or give the one with the same name a new version (runs going on keep theirs)",
		Mutating: true,
		Examples: []string{"myco workflow apply --file wf.json", "myco workflow apply --file wf.json --dry-run"},
		Flags:    []Flag{fileFlag, adminFlag},
		Run: handler(func(e *env) error {
			def, err := e.readWorkflowFile()
			if err != nil {
				return err
			}
			c, err := e.projectClient()
			if err != nil {
				return err
			}
			dry := e.in.Bool("dry-run")
			out, err := c.ApplyWorkflow(e.in.Ctx, def, dry)
			if err != nil {
				return err
			}
			if dry {
				return e.dryRun(out)
			}
			return e.p.Print(out, func(w io.Writer) {
				switch {
				case out.Created:
					fmt.Fprintf(w, "created %s (%s)\n", out.Workflow.Name, out.Workflow.ID)
				case out.Changed:
					fmt.Fprintf(w, "updated %s to version %d\n", out.Workflow.Name, out.Workflow.Version)
				default:
					fmt.Fprintf(w, "%s is unchanged (version %d)\n", out.Workflow.Name, out.Workflow.Version)
				}
				for _, p := range out.Warnings {
					fmt.Fprintln(w, "warning: "+p)
				}
			})
		}),
	}
	list := &Command{
		Name:     "list",
		Summary:  "List workflows (yours; everyone's for those who manage the workspace)",
		Examples: []string{"myco workflow list"},
		Flags:    []Flag{adminFlag},
		Run: handler(func(e *env) error {
			c, err := e.projectClient()
			if err != nil {
				return err
			}
			ws, err := c.Workflows(e.in.Ctx)
			if err != nil {
				return err
			}
			return e.p.Print(api.WorkflowList{Workflows: ws}, func(w io.Writer) {
				if len(ws) == 0 {
					fmt.Fprintln(w, "no workflows yet: write one and save it with myco workflow apply --file wf.json")
					return
				}
				tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
				fmt.Fprintln(tw, "NAME\tVERSION\tSTEPS\tRUNNING\tLAST RUN\tPROJECT")
				for _, wf := range ws {
					last := "-"
					if r := wf.LastRun; r != nil {
						last = r.Status + " " + r.CreatedAt.Local().Format("01-02 15:04")
					}
					name := wf.Name
					if !wf.Enabled {
						name += " (off)"
					}
					fmt.Fprintf(tw, "%s\tv%d\t%d\t%d\t%s\t%s\n", name, wf.Version, wf.Steps, wf.Active, last, wf.Project)
				}
				tw.Flush()
			})
		}),
	}
	get := &Command{
		Name:     "get",
		Summary:  "Show a workflow and its definition (what it takes, its steps)",
		Examples: []string{`myco workflow get --name 写-审-改`, `myco workflow get --name 写-审-改 --format json`},
		Flags:    []Flag{nameFlag, adminFlag},
		Run: handler(func(e *env) error {
			c, err := e.projectClient()
			if err != nil {
				return err
			}
			wf, err := c.Workflow(e.in.Ctx, e.in.String("name"))
			if err != nil {
				return err
			}
			return e.p.Print(wf, func(w io.Writer) {
				fmt.Fprintf(w, "%s  v%d  %s%s\n", wf.Name, wf.Version, wf.ID, map[bool]string{false: "  (off)"}[wf.Enabled])
				if wf.Description != "" {
					fmt.Fprintln(w, wf.Description)
				}
				var pretty json.RawMessage
				if b, err := json.MarshalIndent(wf.Definition, "", "  "); err == nil {
					pretty = b
				}
				fmt.Fprintf(w, "\n%s\n", pretty)
			})
		}),
	}
	toggle := func(name string, on bool, summary string) *Command {
		return &Command{
			Name: name, Summary: summary, Mutating: true,
			Examples: []string{"myco workflow " + name + " --name 写-审-改"},
			Flags:    []Flag{nameFlag, adminFlag},
			Run: handler(func(e *env) error {
				c, err := e.projectClient()
				if err != nil {
					return err
				}
				if e.in.Bool("dry-run") {
					return e.dryRun(map[string]any{"action": "workflow_" + name, "name": e.in.String("name")})
				}
				wf, err := c.SetWorkflowEnabled(e.in.Ctx, e.in.String("name"), on)
				if err != nil {
					return err
				}
				return e.p.Print(wf, func(w io.Writer) {
					fmt.Fprintf(w, "%s is %s\n", wf.Name, map[bool]string{true: "on", false: "off"}[on])
				})
			}),
		}
	}
	remove := &Command{
		Name:     "remove",
		Summary:  "Remove a workflow and its runs (stopping the ones going; the tasks they sent stay)",
		Mutating: true,
		Examples: []string{"myco workflow remove --name 写-审-改"},
		Flags:    []Flag{nameFlag, adminFlag},
		Run: handler(func(e *env) error {
			c, err := e.projectClient()
			if err != nil {
				return err
			}
			dry := e.in.Bool("dry-run")
			if err := c.DeleteWorkflow(e.in.Ctx, e.in.String("name"), dry); err != nil {
				return err
			}
			if dry {
				return e.dryRun(map[string]any{"action": "workflow_remove", "name": e.in.String("name")})
			}
			return e.p.Print(map[string]any{"removed": e.in.String("name")}, func(w io.Writer) { fmt.Fprintf(w, "removed %s\n", e.in.String("name")) })
		}),
	}

	start := &Command{
		Name:     "start",
		Summary:  "Start a run of a workflow (with --wait: until it ends or waits for a decision)",
		Mutating: true,
		Examples: []string{
			`myco workflow run start --name 写-审-改 --input task="加导出按钮"`,
			`myco workflow run start --name 日报 --inputs '{"date": "2026-11-19"}' --wait`,
			`myco workflow run start --name 写-审-改 --input task=x --dry-run   # shows the first step's task`,
		},
		Flags: []Flag{nameFlag, adminFlag,
			{Name: "input", Kind: List, Help: "An input as key=value (repeatable; long text: use --inputs)"},
			{Name: "inputs", Kind: String, Help: "All inputs as a JSON object"},
			{Name: "inputs-file", Kind: String, Help: "Read the inputs (a JSON object) from a file"},
			{Name: "wait", Kind: Bool, Help: "Wait until the run ends or waits for a decision"},
			{Name: "timeout", Kind: Duration, Default: "30m", Help: "With --wait: how long to wait"}},
		Run: handler(func(e *env) error {
			inputs, err := e.workflowInputs()
			if err != nil {
				return err
			}
			c, err := e.projectClient()
			if err != nil {
				return err
			}
			run, pre, err := c.StartWorkflow(e.in.Ctx, e.in.String("name"), inputs, e.in.Bool("dry-run"))
			if err != nil {
				return err
			}
			if pre != nil {
				return e.dryRun(pre)
			}
			if e.in.Bool("wait") {
				if run, err = waitRun(e, c, run.ID, e.in.Duration("timeout")); err != nil {
					return err
				}
			}
			return e.p.Print(run, func(w io.Writer) { printRun(w, run) })
		}),
	}
	runs := &Command{
		Name:     "list",
		Summary:  "List runs, newest first",
		Examples: []string{"myco workflow run list", "myco workflow run list --name 写-审-改 --active"},
		Flags: []Flag{adminFlag,
			{Name: "name", Kind: String, Help: "Only this workflow's runs"},
			{Name: "active", Kind: Bool, Help: "Only runs still going (or waiting for a decision)"},
			{Name: "limit", Kind: Int, Default: "20", Help: "At most this many"}},
		Run: handler(func(e *env) error {
			c, err := e.projectClient()
			if err != nil {
				return err
			}
			rs, err := c.WorkflowRuns(e.in.Ctx, e.in.String("name"), e.in.Bool("active"), e.in.Int("limit"))
			if err != nil {
				return err
			}
			return e.p.Print(api.WorkflowRunList{Runs: rs}, func(w io.Writer) {
				tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
				fmt.Fprintln(tw, "ID\tWORKFLOW\tSTATUS\tSTARTED\tBY\tERROR")
				for _, r := range rs {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", r.ID, r.Name, r.Status, r.CreatedAt.Local().Format("01-02 15:04"), r.By, oneLine(r.Error, 60))
				}
				tw.Flush()
			})
		}),
	}
	show := &Command{
		Name:     "show",
		Summary:  "Show a run: each step, who did it, its summary; and the log",
		Examples: []string{"myco workflow run show --id run_abc123"},
		Flags:    []Flag{runFlag, adminFlag},
		Run: handler(func(e *env) error {
			c, err := e.projectClient()
			if err != nil {
				return err
			}
			r, err := c.WorkflowRun(e.in.Ctx, e.in.String("id"))
			if err != nil {
				return err
			}
			return e.p.Print(r, func(w io.Writer) { printRun(w, r) })
		}),
	}
	runAction := func(name, summary string, extra []Flag, examples []string, do func(e *env, c *client.Client) (*api.WorkflowRun, error)) *Command {
		return &Command{
			Name: name, Summary: summary, Mutating: true, Examples: examples,
			Flags: append([]Flag{runFlag, adminFlag}, extra...),
			Run: handler(func(e *env) error {
				c, err := e.projectClient()
				if err != nil {
					return err
				}
				if e.in.Bool("dry-run") {
					return e.dryRun(map[string]any{"action": "workflow_run_" + name, "id": e.in.String("id")})
				}
				r, err := do(e, c)
				if err != nil {
					return err
				}
				return e.p.Print(r, func(w io.Writer) { printRun(w, r) })
			}),
		}
	}
	cancel := runAction("cancel", "Stop a run: its open tasks are stopped, its decisions withdrawn", nil,
		[]string{"myco workflow run cancel --id run_abc123"},
		func(e *env, c *client.Client) (*api.WorkflowRun, error) {
			return c.CancelWorkflowRun(e.in.Ctx, e.in.String("id"))
		})
	retry := runAction("retry", "Start a failed or cancelled run again at a step (default: the one that failed), keeping earlier results",
		[]Flag{{Name: "from", Kind: String, Help: "The step to start from"}},
		[]string{"myco workflow run retry --id run_abc123", "myco workflow run retry --id run_abc123 --from review"},
		func(e *env, c *client.Client) (*api.WorkflowRun, error) {
			return c.RetryWorkflowRun(e.in.Ctx, e.in.String("id"), e.in.String("from"))
		})
	decide := runAction("decide", "Answer a decision step (people; also in the App)",
		[]Flag{{Name: "step", Kind: String, Required: true, Help: "The decision step's id"},
			{Name: "choice", Kind: String, Required: true, Help: "One of its options"},
			{Name: "note", Kind: String, Help: "A word to go with it"}},
		[]string{`myco workflow run decide --id run_abc123 --step ok --choice 合并`},
		func(e *env, c *client.Client) (*api.WorkflowRun, error) {
			if err := e.in.Require("step", "choice"); err != nil {
				return nil, err
			}
			return c.DecideWorkflowRun(e.in.Ctx, e.in.String("id"), api.WorkflowDecide{Step: e.in.String("step"), Choice: e.in.String("choice"), Note: e.in.String("note")})
		})
	decisions := &Command{
		Name:     "decisions",
		Summary:  "List the decisions waiting for you",
		Examples: []string{"myco workflow decisions"},
		Flags:    []Flag{adminFlag},
		Run: handler(func(e *env) error {
			c, err := e.projectClient()
			if err != nil {
				return err
			}
			ds, err := c.WorkflowDecisions(e.in.Ctx)
			if err != nil {
				return err
			}
			return e.p.Print(api.WorkflowDecisions{Decisions: ds}, func(w io.Writer) {
				if len(ds) == 0 {
					fmt.Fprintln(w, "nothing is waiting for a decision")
				}
				for _, d := range ds {
					fmt.Fprintf(w, "%s  %s · %s: %s  [%s]\n", d.Run.ID, d.Run.Name, d.Step.Step, oneLine(d.Step.Input, 80), strings.Join(d.Step.Options, " / "))
				}
			})
		}),
	}
	runCmd := &Command{Name: "run", Summary: "Start, follow, cancel and retry runs; answer decisions"}
	runCmd.Add(start, runs, show, cancel, retry, decide)
	root := &Command{Name: "workflow", Summary: "Workflows: steps the workspace hands to agents, computers and people, in order"}
	root.Add(validate, apply, list, get, toggle("enable", true, "Turn a workflow on (it can be started)"),
		toggle("disable", false, "Turn a workflow off (it cannot be started; runs going on go on)"), remove, runCmd, decisions)
	return root
}

func (e *env) readWorkflowFile() (json.RawMessage, error) {
	if err := e.in.Require("file"); err != nil {
		return nil, err
	}
	path := e.in.String("file")
	var b []byte
	var err error
	if path == "-" {
		b, err = io.ReadAll(e.in.Stdin)
	} else {
		b, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, errorf("invalid_argument", ExitUsage, "", "cannot read %s: %v", path, err)
	}
	if !json.Valid(b) {
		return nil, errorf("invalid_argument", ExitUsage, "check it with a JSON linter (python3 -m json.tool)", "%s is not valid JSON", path)
	}
	return b, nil
}

var inputKeyRe = regexp.MustCompile(`^[\p{L}_][\p{L}\p{N}_-]*=`)

// workflowInputs gathers --inputs-file, --inputs and --input k=v (in that
// order, later ones winning). --input is split at commas by the flag
// parser: a piece without "key=" belongs to the value before it.
func (e *env) workflowInputs() (map[string]any, error) {
	out := map[string]any{}
	readJSON := func(what string, b []byte) error {
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			return errorf("invalid_argument", ExitUsage, `write it like {"task": "..."}`, "%s must be a JSON object: %v", what, err)
		}
		for k, v := range m {
			out[k] = v
		}
		return nil
	}
	if f := e.in.String("inputs-file"); f != "" {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, errorf("invalid_argument", ExitUsage, "", "cannot read %s: %v", f, err)
		}
		if err := readJSON("--inputs-file", b); err != nil {
			return nil, err
		}
	}
	if s := e.in.String("inputs"); s != "" {
		if err := readJSON("--inputs", []byte(s)); err != nil {
			return nil, err
		}
	}
	key := ""
	for _, part := range e.in.List("input") {
		if inputKeyRe.MatchString(part) {
			k, v, _ := strings.Cut(part, "=")
			key, out[k] = k, v
			continue
		}
		if key == "" {
			return nil, errorf("invalid_argument", ExitUsage, `pass --input key=value`, "--input %q has no key", part)
		}
		out[key] = fmt.Sprint(out[key]) + "," + part
	}
	return out, nil
}

// waitRun follows a run until it ends or waits for a decision.
func waitRun(e *env, c *client.Client, id string, timeout time.Duration) (*api.WorkflowRun, error) {
	deadline := time.Now().Add(timeout)
	for {
		r, err := c.WorkflowRun(e.in.Ctx, id)
		if err != nil {
			return nil, err
		}
		if r.Status != "running" {
			return r, nil
		}
		if time.Now().After(deadline) {
			return r, errorf("timeout", ExitTimeout, "see where it is: myco workflow run show --id "+id, "run %s is still going after %s", id, timeout)
		}
		select {
		case <-e.in.Ctx.Done():
			return r, e.in.Ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
}

func printRun(w io.Writer, r *api.WorkflowRun) {
	fmt.Fprintf(w, "%s  %s v%d  %s  (by %s, %s)\n", r.ID, r.Name, r.Version, r.Status, r.By, r.CreatedAt.Local().Format("2006-01-02 15:04"))
	if r.Error != "" {
		fmt.Fprintln(w, "  "+r.Error)
	}
	for _, s := range r.Steps {
		line := fmt.Sprintf("  %-9s %s", s.Status, s.Step)
		if s.Attempt > 1 {
			line += fmt.Sprintf(" (#%d)", s.Attempt)
		}
		if s.Target != "" {
			line += " → " + s.Target
		}
		switch {
		case s.Error != "" && s.Status == "failed":
			line += ": " + oneLine(s.Error, 80)
		case s.Summary != "":
			line += ": " + oneLine(s.Summary, 80)
		case s.Status == "waiting":
			line += fmt.Sprintf(": %s [%s]", oneLine(s.Input, 60), strings.Join(s.Options, " / "))
		}
		if s.MessageID != "" {
			line += "  (" + s.MessageID + ")"
		}
		fmt.Fprintln(w, line)
	}
}
