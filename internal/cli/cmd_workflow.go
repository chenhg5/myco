package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/chenhg5/myco/internal/api"
	. "github.com/chenhg5/myco/internal/cli/cmdkit"
	"github.com/chenhg5/myco/internal/client"
	"github.com/chenhg5/myco/internal/domain"
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

  {"name": "写-审-改", "title": "写、审、改", "tags": ["dev"], "project": "myco",
   "inputs": {"type": "object", "required": ["task"], "properties": {"task": {"type": "string", "title": "要做什么", "minLength": 4}}},
   "steps": [
     {"id": "write", "agent": "coder", "prompt": "完成：{{input.task}}{{#if steps.review}}\n意见：{{steps.review.output.comments}}{{/if}}",
      "outputSchema": {"type": "object", "required": ["branch"], "properties": {"branch": {"type": "string", "pattern": "^feat/"}}}, "timeout": "2h"},
     {"id": "review", "role": "审查", "prompt": "审一下 {{input.branch}}",
      "inputSchema": {"type": "object", "required": ["branch"]},
      "outputSchema": {"type": "object", "required": ["approved"], "properties": {"approved": {"type": "boolean"}, "comments": {"type": "array", "items": {"type": "string"}}}}},
     {"id": "test", "computer": "studio-mac", "run": "git checkout {{steps.write.output.branch}} && make test"},
     {"id": "ok", "human": "$starter", "prompt": "合并 {{steps.write.output.branch}} 吗？", "options": ["合并", "不合并"]}],
   "edges": [
     {"from": "write", "to": "review", "map": {"branch": "{{output.branch}}"}},
     {"from": "review", "to": "test", "when": "output.approved"},
     {"from": "review", "to": "write", "when": "!output.approved", "maxIterations": 3, "else": "$fail", "map": {"task": "{{inputs.task}}"}},
     {"from": "test", "to": "ok"},
     {"from": "test", "to": "write", "onError": true, "maxIterations": 2, "map": {"task": "{{inputs.task}}"}},
     {"from": "ok", "to": "$end"}]}

Steps: agent (a task) or role (a task to the least busy agent whose role
matches: a label role=<x>, a skill, the word in its description), computer
(a command on an exec node; fails on a non-zero exit), human (a decision in
the App; options are the choices; unanswered by its timeout, default 3d,
it is a "no": choice is ""). Each step: title, timeout (30m, 2h, 1d), retry
(0-5), dispatch "first" (default) or "all" (every matching edge, in
parallel), join "all" or {"from": [ids]} (start when those have arrived).

Inputs and outputs: a run's inputs are the entry step's input (entry:
the first step unless set); a step's result goes to the next step as its
input, or what an edge's map builds ("{{output.x}}" alone keeps x's type);
a join's input is an object by step id. inputSchema and outputSchema
(output for short) are full JSON Schema draft 2020-12 (oneOf, $ref,
pattern, minimum, …): an input that does not fit fails the step; a result
that does not fit is sent back once, then fails it.

Edges: the first whose when holds is taken; onError edges only when the
step failed. An edge back to an earlier step needs maxIterations (past it,
else; $fail when unset) unless allowUnboundedCycles is true. With no edges
at all, steps run in order. $end ends a branch, $fail fails the run.

{{expr}} in prompt, run, title, agent, role and computer puts a value in;
{{#if expr}}…{{else}}…{{/if}}. Names: inputs, input, steps.<id> (or nodes.<id>)
.output/.result/.input/.summary/.status/.error/.attempt, run; on an edge also
output, status, error, attempt. Operators: == != < <= > >= && || ! and or
not in + - * / %; functions len contains startsWith endsWith matches lower
upper trim. Names that are not there are refused by validate.`,
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
			{Name: "status", Kind: Enum, Values: []string{"", "running", "waiting", "succeeded", "failed", "cancelled"}, Help: "Only runs in this state"},
			{Name: "limit", Kind: Int, Default: "20", Help: "At most this many"}},
		Run: handler(func(e *env) error {
			c, err := e.projectClient()
			if err != nil {
				return err
			}
			rs, err := c.WorkflowRunsWhere(e.in.Ctx, client.RunQuery{Workflow: e.in.String("name"), Active: e.in.Bool("active"),
				Status: e.in.String("status"), Limit: e.in.Int("limit")})
			if err != nil {
				return err
			}
			return e.p.Print(api.WorkflowRunList{Runs: rs}, func(w io.Writer) {
				tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
				fmt.Fprintln(tw, "ID\tWORKFLOW\tSTATUS\tSTARTED\tTOOK\tBY\tERROR")
				for _, r := range rs {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", r.ID, r.Name, r.Status, r.CreatedAt.Local().Format("01-02 15:04"),
						took(r.DurationMS), r.By, oneLine(r.Error, 60))
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
	logs := &Command{
		Name:    "logs",
		Summary: "Show a step's log in a run: the task sent, progress, the answer, what the agent did, a computer's output",
		Examples: []string{"myco workflow run logs --id run_abc123 --step review",
			"myco workflow run logs --id run_abc123 --step review --no-trace"},
		Flags: []Flag{runFlag, adminFlag,
			{Name: "step", Kind: String, Required: true, Help: "The step's id (its latest start), or a step start's id (rs_...)"},
			{Name: "no-trace", Kind: Bool, Help: "Leave out what the agent did (its trace)"}},
		Run: handler(func(e *env) error {
			if err := e.in.Require("step"); err != nil {
				return err
			}
			c, err := e.projectClient()
			if err != nil {
				return err
			}
			l, err := c.WorkflowStepLog(e.in.Ctx, e.in.String("id"), e.in.String("step"), !e.in.Bool("no-trace"))
			if err != nil {
				return err
			}
			return e.p.Print(l, func(w io.Writer) { printStepLog(w, l) })
		}),
	}
	evaluate := &Command{
		Name:     "evaluate",
		Summary:  "Give a run (or one of its steps) a score: by hand, from a test script, or by an agent that checked the work",
		Mutating: true,
		Examples: []string{"myco workflow run evaluate --id run_abc123 --name quality --value 4 --unit 分 --direction higher",
			`myco workflow run evaluate --id run_abc123 --name tests_failed --value 0 --direction lower --note "全部通过"`},
		Flags: []Flag{runFlag, adminFlag,
			{Name: "name", Kind: String, Required: true, Help: "What is measured (letters, digits, _ . -)"},
			{Name: "value", Kind: String, Required: true, Help: "A number"},
			{Name: "unit", Kind: String, Help: "Its unit, shown after it (%, 分, 个 …)"},
			{Name: "direction", Kind: Enum, Values: []string{"", "higher", "lower"}, Help: "Whether more is better (higher) or less (lower)"},
			{Name: "step", Kind: String, Help: "The step it is about (default: the whole run)"},
			{Name: "note", Kind: String, Help: "Why"}},
		Run: handler(func(e *env) error {
			if err := e.in.Require("name", "value"); err != nil {
				return err
			}
			v, err := strconv.ParseFloat(strings.TrimSpace(e.in.String("value")), 64)
			if err != nil {
				return &UsageError{Message: fmt.Sprintf("--value %q is not a number", e.in.String("value")),
					Suggestion: "pass a number, e.g. --value 4.5", Command: e.in.Cmd.Path()}
			}
			c, err := e.projectClient()
			if err != nil {
				return err
			}
			in := api.RunEvaluate{Step: e.in.String("step"), Name: e.in.String("name"), Value: v, Unit: e.in.String("unit"),
				Direction: e.in.String("direction"), Note: e.in.String("note")}
			ev, err := c.EvaluateWorkflowRun(e.in.Ctx, e.in.String("id"), in, e.in.Bool("dry-run"))
			if err != nil {
				return err
			}
			if e.in.Bool("dry-run") {
				return e.dryRun(map[string]any{"action": "workflow_run_evaluate", "id": e.in.String("id"), "evaluation": ev})
			}
			return e.p.Print(ev, func(w io.Writer) { fmt.Fprintf(w, "%s: %s = %s\n", ev.Run, ev.Name, scoreText(ev.Value, ev.Unit)) })
		}),
	}
	compare := &Command{
		Name:     "compare",
		Summary:  "Set two runs side by side: time, attempts and cost per step, and their scores",
		Examples: []string{"myco workflow run compare --a run_abc123 --b run_def456"},
		Flags: []Flag{adminFlag,
			{Name: "a", Kind: String, Required: true, Help: "One run's id"},
			{Name: "b", Kind: String, Required: true, Help: "The other run's id"}},
		Run: handler(func(e *env) error {
			if err := e.in.Require("a", "b"); err != nil {
				return err
			}
			c, err := e.projectClient()
			if err != nil {
				return err
			}
			cmp, err := c.CompareWorkflowRuns(e.in.Ctx, e.in.String("a"), e.in.String("b"))
			if err != nil {
				return err
			}
			return e.p.Print(cmp, func(w io.Writer) { printCompare(w, cmp) })
		}),
	}
	runCmd := &Command{Name: "run", Summary: "Start, follow, cancel and retry runs; answer decisions; logs, scores, comparisons"}
	runCmd.Add(start, runs, show, logs, cancel, retry, decide, evaluate, compare)
	stats := &Command{
		Name:     "stats",
		Summary:  "A workflow's figures: runs, success rate, time (p50/p95), each step's time, failures, retries and cost; scores",
		Examples: []string{"myco workflow stats --name 写-审-改", "myco workflow stats --name 写-审-改 --limit 50 --version 3"},
		Flags: []Flag{nameFlag, adminFlag,
			{Name: "limit", Kind: Int, Default: "200", Help: "Work out times over the last this many runs"},
			{Name: "version", Kind: Int, Help: "Only runs of this version"}},
		Run: handler(func(e *env) error {
			c, err := e.projectClient()
			if err != nil {
				return err
			}
			st, err := c.WorkflowStats(e.in.Ctx, e.in.String("name"), e.in.Int("limit"), e.in.Int("version"))
			if err != nil {
				return err
			}
			return e.p.Print(st, func(w io.Writer) { printStats(w, st) })
		}),
	}
	root := &Command{Name: "workflow", Summary: "Workflows: steps the workspace hands to agents, computers and people, in order"}
	root.Add(validate, apply, list, get, toggle("enable", true, "Turn a workflow on (it can be started)"),
		toggle("disable", false, "Turn a workflow off (it cannot be started; runs going on go on)"), remove, stats, runCmd, decisions)
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
	fmt.Fprintf(w, "%s  %s v%d  %s  (by %s, %s, took %s)\n", r.ID, r.Name, r.Version, r.Status, r.By, r.CreatedAt.Local().Format("2006-01-02 15:04"),
		took(r.DurationMS))
	if r.Error != "" {
		fmt.Fprintln(w, "  "+r.Error)
	}
	if m := r.Metrics; m != nil {
		line := fmt.Sprintf("  at work %s · waiting for decisions %s · %d steps, %d attempts", took(m.ActiveMS), took(m.WaitingMS), m.Steps, m.Attempts)
		if m.Retries > 0 {
			line += fmt.Sprintf(", %d retries", m.Retries)
		}
		if m.Bounces > 0 {
			line += fmt.Sprintf(", %d sent back", m.Bounces)
		}
		if u := usageText(m.Usage); u != "" {
			line += " · " + u
		}
		fmt.Fprintln(w, line)
	}
	for _, s := range r.Steps {
		line := fmt.Sprintf("  %-9s %-8s %s", s.Status, took(s.DurationMS), s.Step)
		if s.Attempt > 1 {
			line += fmt.Sprintf(" (#%d)", s.Attempt)
		}
		if s.Attempts > 1 {
			line += fmt.Sprintf(" ×%d", s.Attempts)
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
	if len(r.Artifacts) > 0 {
		fmt.Fprintln(w, "files handed back:")
		for _, a := range r.Artifacts {
			fmt.Fprintf(w, "  %s  %s (%s)  from %s · %s  (myco file get --id %s)\n", a.Step, a.Name, sizeText(a.Size), a.From, a.Kind, a.FileID)
		}
	}
	if len(r.Evaluations) > 0 {
		fmt.Fprintln(w, "scores:")
		for _, e := range r.Evaluations {
			line := fmt.Sprintf("  %s = %s", e.Name, scoreText(e.Value, e.Unit))
			if e.Step != "" {
				line += " (" + e.Step + ")"
			}
			line += "  by " + e.By
			if e.Note != "" {
				line += ": " + oneLine(e.Note, 60)
			}
			fmt.Fprintln(w, line)
		}
	}
}

// took writes a duration in ms for people ("1m05s"; "-" for none).
func took(ms int64) string {
	if ms <= 0 {
		return "-"
	}
	return domain.HumanMS(ms)
}

func usageText(u *api.Usage) string {
	if u == nil {
		return ""
	}
	var parts []string
	if u.InputTokens+u.OutputTokens > 0 {
		parts = append(parts, fmt.Sprintf("%s in / %s out tokens", countText(u.InputTokens), countText(u.OutputTokens)))
	}
	if u.CostUSD > 0 {
		parts = append(parts, fmt.Sprintf("$%.2f", u.CostUSD))
	}
	return strings.Join(parts, ", ")
}

func countText(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 1000:
		return fmt.Sprintf("%.1fk", float64(n)/1e3)
	}
	return strconv.FormatInt(n, 10)
}

func sizeText(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

func scoreText(v float64, unit string) string {
	return strconv.FormatFloat(v, 'f', -1, 64) + unit
}

func printStepLog(w io.Writer, l *api.StepLog) {
	s := l.Step
	fmt.Fprintf(w, "%s · %s  %s  took %s", l.Run, s.Step, s.Status, took(s.DurationMS))
	if s.Target != "" {
		fmt.Fprintf(w, "  → %s", s.Target)
	}
	if u := usageText(s.Usage); u != "" {
		fmt.Fprintf(w, "  (%s)", u)
	}
	fmt.Fprintln(w)
	if l.Clipped {
		fmt.Fprintf(w, "(only the last %d lines)\n", len(l.Entries))
	}
	for _, e := range l.Entries {
		head := e.Kind
		if e.Sub != "" {
			head += ":" + e.Sub
		}
		if e.Try > 1 {
			head += fmt.Sprintf(" #%d", e.Try)
		}
		fmt.Fprintf(w, "%s  %-18s", e.At.Local().Format("15:04:05"), head)
		text := strings.TrimRight(e.Text, "\n")
		switch e.Kind {
		case "stdout", "stderr", "task", "reply", "progress":
			fmt.Fprintln(w)
			for _, line := range strings.Split(text, "\n") {
				fmt.Fprintln(w, "    "+line)
			}
		default:
			fmt.Fprintln(w, oneLine(text, 160))
		}
		for _, a := range e.Attachments {
			fmt.Fprintf(w, "    📎 %s (%s)  myco file get --id %s\n", a.Name, sizeText(a.Size), a.FileID)
		}
	}
}

func distText(d api.Dist) string {
	if d.N == 0 {
		return "-"
	}
	return fmt.Sprintf("p50 %s · p95 %s · avg %s", took(int64(d.P50)), took(int64(d.P95)), took(int64(d.Avg)))
}

func printStats(w io.Writer, st *api.WorkflowStats) {
	fmt.Fprintf(w, "%s: %d runs", st.Name, st.Total)
	for _, k := range []string{"succeeded", "failed", "cancelled", "running", "waiting"} {
		if n := st.ByStatus[k]; n > 0 {
			fmt.Fprintf(w, " · %s %d", k, n)
		}
	}
	if st.Total > 0 {
		fmt.Fprintf(w, " · success %.0f%%", st.SuccessRate*100)
	}
	fmt.Fprintln(w)
	if st.Sample == 0 {
		return
	}
	scope := fmt.Sprintf("last %d runs", st.Sample)
	if st.Version > 0 {
		scope += fmt.Sprintf(" of version %d", st.Version)
	}
	fmt.Fprintf(w, "time (%s): %s\n", scope, distText(st.Duration))
	fmt.Fprintf(w, "  at work %s\n  waiting for decisions %s\n", distText(st.Active), distText(st.Waiting))
	if u := usageText(st.Usage); u != "" {
		fmt.Fprintf(w, "cost: %s (about $%.2f a run)\n", u, st.CostPerRun)
	}
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "STEP\tRUNS\tFAILED\tRETRIES\tSENT BACK\tP50\tP95\tCOST\tLAST ERROR")
	for _, x := range st.Steps {
		cost := "-"
		if x.Usage != nil && x.Usage.CostUSD > 0 {
			cost = fmt.Sprintf("$%.2f", x.Usage.CostUSD)
		}
		fmt.Fprintf(tw, "%s\t%d\t%d\t%d\t%d\t%s\t%s\t%s\t%s\n", x.Step, x.Runs, x.Failed, x.Retries, x.Bounces,
			took(int64(x.Duration.P50)), took(int64(x.Duration.P95)), cost, oneLine(x.LastError, 50))
	}
	tw.Flush()
	if len(st.Versions) > 1 {
		fmt.Fprintln(w, "by version:")
		for _, v := range st.Versions {
			fmt.Fprintf(w, "  v%d  %d runs, %d succeeded, %d failed  %s\n", v.Version, v.Runs, v.Succeeded, v.Failed, distText(v.Duration))
		}
	}
	if len(st.Evaluations) > 0 {
		fmt.Fprintln(w, "scores:")
		for _, e := range st.Evaluations {
			fmt.Fprintf(w, "  %s  avg %s · min %s · max %s · %d runs\n", e.Name, scoreText(round2(e.Values.Avg), e.Unit),
				scoreText(e.Values.Min, e.Unit), scoreText(e.Values.Max, e.Unit), e.Values.N)
		}
	}
	if len(st.Failures) > 0 {
		fmt.Fprintln(w, "latest failures:")
		for _, f := range st.Failures {
			fmt.Fprintf(w, "  %s  %s  %s: %s\n", f.ID, f.CreatedAt.Local().Format("01-02 15:04"), f.FailedStep, oneLine(f.Error, 80))
		}
	}
}

func round2(f float64) float64 { return math.Round(f*100) / 100 }

func printCompare(w io.Writer, c *api.RunComparison) {
	a, b := c.A, c.B
	fmt.Fprintf(w, "A %s  %s v%d  %s  took %s\n", a.Run.ID, a.Run.Name, a.Run.Version, a.Run.Status, took(a.Metrics.DurationMS))
	fmt.Fprintf(w, "B %s  %s v%d  %s  took %s\n", b.Run.ID, b.Run.Name, b.Run.Version, b.Run.Status, took(b.Metrics.DurationMS))
	if !c.SameVersion {
		fmt.Fprintln(w, "(not the same version of the workflow)")
	}
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "STEP\tA\tB\tΔ (B−A)")
	side := func(x *api.RunCompareStep) string {
		if x == nil {
			return "—"
		}
		t := x.Status + " " + took(x.DurationMS)
		if x.Attempts > 1 {
			t += fmt.Sprintf(" ×%d", x.Attempts)
		}
		return t
	}
	for _, d := range c.Steps {
		delta := ""
		if d.A != nil && d.B != nil {
			sign := "+"
			ms := d.DeltaMS
			if ms < 0 {
				sign, ms = "-", -ms
			}
			delta = sign + took(ms)
			if ms == 0 {
				delta = "0"
			}
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", d.Step, side(d.A), side(d.B), delta)
	}
	tw.Flush()
	for _, e := range c.Evaluations {
		val := func(v *float64) string {
			if v == nil {
				return "—"
			}
			return scoreText(*v, e.Unit)
		}
		line := fmt.Sprintf("score %s: A %s · B %s", e.Name, val(e.A), val(e.B))
		if e.Better != "" {
			line += " · better: " + strings.ToUpper(e.Better)
		}
		fmt.Fprintln(w, line)
	}
}
