package domain

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// A workflow is a graph the hub walks: each step hands work to an agent
// (a task), runs a command on a computer (an exec node), or asks a person
// to decide; edges say which step comes next. Agents only hand back
// results: the hub alone decides what happens next.

// Step kinds.
const (
	StepAgent    = "agent"
	StepComputer = "computer"
	StepHuman    = "human"
)

// Edge targets that are not steps.
const (
	EdgeEnd  = "$end"
	EdgeFail = "$fail"
)

// StarterHuman, as a human step's person, is whoever started the run.
const StarterHuman = "$starter"

// Limits.
const (
	MaxWorkflowName     = 80
	MaxWorkflowSteps    = 50
	MaxWorkflowEdges    = 200
	MaxWorkflowDefBytes = 256 << 10
	MaxWorkflows        = 500
	// MaxRunSteps bounds the step instances one run may start (loops
	// included), so a mistake cannot spin forever.
	MaxRunSteps = 200
	// MaxRunStepsUnbounded is the same for a workflow that allows
	// unbounded cycles.
	MaxRunStepsUnbounded = 2000
	// MaxRunEvents bounds what one run keeps in its log.
	MaxRunEvents = 500
	// DefaultHumanTimeout is how long a decision waits when the step names
	// no timeout; then it counts as "no".
	DefaultHumanTimeout = 72 * time.Hour
)

// WorkflowDef is what people (usually an agent) write: JSON.
type WorkflowDef struct {
	Name string `json:"name"`
	// Title, Description, Tags and Group are for people (lists, the App);
	// they do not change how it runs.
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Group       string   `json:"group,omitempty"`
	Project     string   `json:"project,omitempty"`
	// Inputs is the start form: an object schema (JSON Schema).
	Inputs *Schema    `json:"inputs,omitempty"`
	Steps  []StepDef  `json:"steps"`
	Edges  []EdgeDef  `json:"edges,omitempty"`
	Layout *DefLayout `json:"layout,omitempty"`
	// Entry is the step a run starts at ("" = the first).
	Entry string `json:"entry,omitempty"`
	// AllowUnboundedCycles lets an edge go back without maxIterations
	// (the loop ends when a condition says so, or the run is cancelled).
	AllowUnboundedCycles bool `json:"allowUnboundedCycles,omitempty"`
}

// EntryStep is the step a run starts at.
func (d *WorkflowDef) EntryStep() string {
	if d.Entry != "" {
		return d.Entry
	}
	if len(d.Steps) > 0 {
		return d.Steps[0].ID
	}
	return ""
}

// DefLayout is optional drawing hints for the App (unused by the engine).
type DefLayout struct {
	Direction string `json:"direction,omitempty"` // "down" (default) or "right"
}

// StepDef is one step. Exactly one of Agent, Computer and Human is set.
type StepDef struct {
	ID    string `json:"id"`
	Title string `json:"title,omitempty"`
	// Agent: the agent (or alias) the task goes to; Prompt is the task.
	Agent  string `json:"agent,omitempty"`
	Prompt string `json:"prompt,omitempty"`
	// Role, instead of Agent: the task goes to an agent whose role (分工)
	// matches, the least busy one when it starts (see MatchesRole).
	Role string `json:"role,omitempty"`
	// Computer: the exec node the command (Run) runs on. Parse "json"
	// reads its stdout as JSON into output.json.
	Computer string `json:"computer,omitempty"`
	Run      string `json:"run,omitempty"`
	Parse    string `json:"parse,omitempty"`
	// Human: who decides (a member name, or "$starter"); Prompt explains,
	// Options are the choices.
	Human   string   `json:"human,omitempty"`
	Options []string `json:"options,omitempty"`
	// Output (or OutputSchema, the same) is the JSON Schema an agent's
	// result must fit; InputSchema, the one its input must fit before it
	// is sent (the run's inputs for the entry step, the step before's
	// result, an edge's map, or a join's object keyed by step).
	Output       *Schema `json:"output,omitempty"`
	OutputSchema *Schema `json:"outputSchema,omitempty"`
	InputSchema  *Schema `json:"inputSchema,omitempty"`
	// Timeout: Go duration, or "<n>d" for days ("" = none; human steps
	// wait DefaultHumanTimeout).
	Timeout string `json:"timeout,omitempty"`
	// Retry: how many more times a failed step is sent again.
	Retry int `json:"retry,omitempty"`
	// Dispatch "all": every matching edge out of it is taken (in
	// parallel); "first" (the default): only the first.
	Dispatch string `json:"dispatch,omitempty"`
	// Join: it starts only when the steps it waits for have all arrived:
	// "all" (every step with an edge into it) or {"from": [ids]}. Its
	// input is an object keyed by those steps.
	Join *JoinSpec `json:"join,omitempty"`
}

// Contract is the step's output schema (output or outputSchema).
func (s *StepDef) Contract() *Schema {
	if s.Output != nil {
		return s.Output
	}
	return s.OutputSchema
}

// JoinSpec is "all" or {"from": [step ids]}.
type JoinSpec struct {
	All  bool
	From []string
}

func (j *JoinSpec) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		if s != "all" {
			return fmt.Errorf(`join must be "all" or {"from": [step ids]}`)
		}
		j.All = true
		return nil
	}
	var o struct {
		From []string `json:"from"`
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&o); err != nil {
		return fmt.Errorf(`join must be "all" or {"from": [step ids]}: %v`, err)
	}
	j.From = o.From
	return nil
}

func (j JoinSpec) MarshalJSON() ([]byte, error) {
	if j.All || len(j.From) == 0 {
		return json.Marshal("all")
	}
	return json.Marshal(map[string]any{"from": j.From})
}

// Waits lists the steps a join waits for.
func (d *WorkflowDef) Waits(id string) []string {
	st := d.Step(id)
	if st == nil || st.Join == nil {
		return nil
	}
	if len(st.Join.From) > 0 {
		return st.Join.From
	}
	return d.Preds(id)
}

// Kind is the step's kind.
func (s *StepDef) Kind() string {
	switch {
	case s.Agent != "" || s.Role != "":
		return StepAgent
	case s.Computer != "":
		return StepComputer
	case s.Human != "":
		return StepHuman
	}
	return ""
}

// TimeoutDur is the step's timeout (0: none).
func (s *StepDef) TimeoutDur() time.Duration {
	d, _ := ParseWorkflowDuration(s.Timeout)
	if d == 0 && s.Kind() == StepHuman {
		return DefaultHumanTimeout
	}
	return d
}

// Label is what to call the step.
func (s *StepDef) Label() string {
	if s.Title != "" {
		return s.Title
	}
	return s.ID
}

// EdgeDef leads from one step to the next.
type EdgeDef struct {
	From string `json:"from"`
	To   string `json:"to"`
	// When must hold (an expression over output, inputs, steps; "" =
	// always).
	When string `json:"when,omitempty"`
	// MaxIterations bounds how often this edge is taken in one run (an
	// edge back to an earlier step must have one); past it, Else is taken
	// ("" = $fail).
	MaxIterations int    `json:"maxIterations,omitempty"`
	Else          string `json:"else,omitempty"`
	// OnError: taken only when From failed (other edges only when it
	// succeeded).
	OnError bool `json:"onError,omitempty"`
	// Map builds the next step's input (an object; each value a template:
	// "{{output.x}}" alone keeps x's type). Without it, the step's result
	// is passed on as it is.
	Map map[string]any `json:"map,omitempty"`
}

// ParseWorkflowDuration reads "90s", "2h", "1d", "1d12h" ("" = 0).
func ParseWorkflowDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	var days time.Duration
	if i := strings.Index(s, "d"); i > 0 {
		n, err := strconv.Atoi(s[:i])
		if err != nil || n < 0 {
			return 0, Errf(CodeInvalidArgument, "invalid duration %q", s)
		}
		days, s = time.Duration(n)*24*time.Hour, s[i+1:]
		if s == "" {
			return days, nil
		}
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		return 0, Errf(CodeInvalidArgument, "invalid duration %q", s).WithSuggestion(`use e.g. "30m", "2h" or "1d"`)
	}
	return days + d, nil
}

var stepIDRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]{0,39}$`)

// ParseWorkflowDef reads and checks a definition.
func ParseWorkflowDef(b []byte) (*WorkflowDef, error) {
	if len(b) > MaxWorkflowDefBytes {
		return nil, Errf(CodeInvalidArgument, "the workflow is %d bytes, more than %d", len(b), MaxWorkflowDefBytes)
	}
	var d WorkflowDef
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&d); err != nil {
		return nil, Errf(CodeInvalidArgument, "the workflow is not valid JSON: %v", err).
			WithSuggestion("see the format with: myco workflow validate --help")
	}
	if err := d.Validate(); err != nil {
		return nil, err
	}
	return &d, nil
}

// Problems lists everything wrong with the definition (none: valid).
// Schemas are compiled completely when CompleteContracts is set.
func (d *WorkflowDef) Problems() []string {
	c := CompleteContracts
	var out []string
	bad := func(f string, a ...any) { out = append(out, fmt.Sprintf(f, a...)) }
	d.Name = strings.TrimSpace(d.Name)
	if d.Name == "" {
		bad("name is required")
	} else if len([]rune(d.Name)) > MaxWorkflowName {
		bad("name is longer than %d characters", MaxWorkflowName)
	}
	if len([]rune(d.Title)) > 120 {
		bad("title is longer than 120 characters")
	}
	if len([]rune(d.Description)) > 2000 {
		bad("description is longer than 2000 characters")
	}
	if len([]rune(d.Group)) > 80 {
		bad("group is longer than 80 characters")
	}
	if len(d.Tags) > 20 {
		bad("tags: at most 20")
	}
	seenTag := map[string]bool{}
	for _, t := range d.Tags {
		if strings.TrimSpace(t) == "" || len([]rune(t)) > 40 || seenTag[t] {
			bad("tag %q is empty, longer than 40 characters, or there twice", t)
		}
		seenTag[t] = true
	}
	if d.Inputs != nil {
		out = append(out, d.Inputs.problems("inputs", c)...)
	}
	if len(d.Steps) == 0 {
		bad("steps: a workflow needs at least one step")
	}
	if len(d.Steps) > MaxWorkflowSteps {
		bad("steps: at most %d", MaxWorkflowSteps)
	}
	if len(d.Edges) > MaxWorkflowEdges {
		bad("edges: at most %d", MaxWorkflowEdges)
	}
	ids := map[string]bool{}
	for i := range d.Steps {
		if id := d.Steps[i].ID; stepIDRe.MatchString(id) {
			if ids[id] {
				bad("step %s: id is used twice", id)
			}
			ids[id] = true
		}
	}
	refs := refChecker{d: d, ids: ids}
	for i := range d.Steps {
		s := &d.Steps[i]
		where := fmt.Sprintf("steps[%d]", i)
		if s.ID != "" {
			where = "step " + s.ID
		}
		if !stepIDRe.MatchString(s.ID) {
			bad("%s: id must be letters, digits, _ or - (starting with a letter), at most 40", where)
		}
		n := 0
		for _, v := range []string{s.Agent, s.Role, s.Computer, s.Human} {
			if v != "" {
				n++
			}
		}
		if n != 1 {
			bad("%s: set exactly one of agent, role, computer, human", where)
		}
		if s.Output != nil && s.OutputSchema != nil {
			bad("%s: output and outputSchema are the same; set one", where)
		}
		switch s.Kind() {
		case StepAgent:
			if strings.TrimSpace(s.Prompt) == "" {
				bad("%s: an agent step needs a prompt (the task)", where)
			}
			if s.Run != "" || len(s.Options) > 0 || s.Parse != "" {
				bad("%s: run, parse and options are for computer and human steps", where)
			}
		case StepComputer:
			if strings.TrimSpace(s.Run) == "" {
				bad("%s: a computer step needs run (the command)", where)
			}
			if s.Parse != "" && s.Parse != "json" {
				bad(`%s: parse can only be "json"`, where)
			}
			if len(s.Options) > 0 || s.Prompt != "" {
				bad("%s: prompt and options are for agent and human steps", where)
			}
		case StepHuman:
			if len(s.Options) < 2 {
				bad(`%s: a human step needs at least two options, e.g. ["合并", "不合并"]`, where)
			}
			if s.Contract() != nil || s.Run != "" || s.Retry > 0 {
				bad("%s: output, run and retry are for agent and computer steps", where)
			}
		}
		if _, err := ParseWorkflowDuration(s.Timeout); err != nil {
			bad("%s: timeout %q is not a duration (e.g. 30m, 2h, 1d)", where, s.Timeout)
		}
		if s.Retry < 0 || s.Retry > 5 {
			bad("%s: retry must be 0 to 5", where)
		}
		if s.Dispatch != "" && s.Dispatch != "all" && s.Dispatch != "first" {
			bad(`%s: dispatch is "first" (the default) or "all"`, where)
		}
		if c := s.Contract(); c != nil {
			out = append(out, c.problems(where+": output", CompleteContracts)...)
		}
		if s.InputSchema != nil {
			out = append(out, s.InputSchema.problems(where+": inputSchema", c)...)
		}
		if len([]rune(s.Role)) > 40 {
			bad("%s: role is longer than 40 characters", where)
		}
		for _, t := range []string{s.Prompt, s.Run, s.Title, s.Agent, s.Computer, s.Role} {
			if err := CheckTemplate(t); err != nil {
				bad("%s: %v", where, err)
				continue
			}
			out = append(out, refs.template(where, t, false)...)
		}
	}
	target := func(t string) bool { return t == EdgeEnd || t == EdgeFail || ids[t] }
	for i, e := range d.Edges {
		where := fmt.Sprintf("edges[%d] (%s → %s)", i, e.From, e.To)
		if !ids[e.From] {
			bad("%s: from must be a step", where)
		}
		if !target(e.To) {
			bad("%s: to must be a step, $end or $fail", where)
		}
		if e.Else != "" && !target(e.Else) {
			bad("%s: else must be a step, $end or $fail", where)
		}
		if e.MaxIterations < 0 {
			bad("%s: maxIterations cannot be negative", where)
		}
		if e.When != "" {
			x, err := ParseExpr(e.When)
			if err != nil {
				bad("%s: when: %v", where, err)
			} else {
				out = append(out, refs.paths(where+": when", x.Refs(), true)...)
			}
		}
		for k, v := range e.Map {
			out = append(out, refs.value(fmt.Sprintf("%s: map.%s", where, k), v)...)
		}
	}
	if d.Entry != "" && !ids[d.Entry] {
		bad("entry %q is not a step", d.Entry)
	}
	if len(out) > 0 {
		return out
	}
	// Loops must be bounded (unless allowed not to be); every step must
	// be reachable; joins need several ways in. Every loop has an edge to
	// the same or an earlier step (in steps order), so bounding those
	// bounds every loop.
	g := d.graph()
	pos := map[string]int{}
	for i, s := range d.Steps {
		pos[s.ID] = i
	}
	for i, e := range d.Edges {
		back := func(t string) bool { p, ok := pos[t]; return ok && p <= pos[e.From] }
		if back(e.Else) {
			bad("edges[%d] (%s → %s): else cannot go back to an earlier step (it is what happens when the loop is over)", i, e.From, e.To)
		}
		if back(e.To) && e.MaxIterations == 0 && !d.AllowUnboundedCycles {
			bad("edges[%d] (%s → %s) goes back to an earlier step: give it maxIterations (and else for what happens after), or set allowUnboundedCycles", i, e.From, e.To)
		}
	}
	seen := g.reachable(d.EntryStep())
	for _, s := range d.Steps {
		if !seen[s.ID] {
			bad("step %s: no edge leads to it from the entry step", s.ID)
		}
		if s.Join == nil {
			continue
		}
		preds := g.preds[s.ID]
		if len(s.Join.From) == 0 {
			if len(preds) < 2 {
				bad("step %s: join all needs edges from at least two steps", s.ID)
			}
			continue
		}
		if len(s.Join.From) < 2 {
			bad("step %s: join.from needs at least two steps", s.ID)
		}
		for _, f := range s.Join.From {
			if !slices.Contains(preds, f) {
				bad("step %s: join.from %q has no edge into it", s.ID, f)
			}
		}
		for _, p := range preds {
			if !slices.Contains(s.Join.From, p) {
				bad("step %s: %s has an edge into it but is not in join.from", s.ID, p)
			}
		}
	}
	return out
}

// CompleteContracts, when set (by the hub), checks every schema
// completely: inputs, outputs, and the values they are given.
var CompleteContracts Contracts

// refChecker checks the paths templates and conditions read: a step's
// fields exist, an input is declared.
type refChecker struct {
	d   *WorkflowDef
	ids map[string]bool
}

var stepFields = map[string]bool{"output": true, "result": true, "input": true, "summary": true, "status": true, "error": true, "attempt": true}

func (r refChecker) template(where, t string, edge bool) []string {
	nodes, err := parseTemplate(t)
	if err != nil {
		return nil
	}
	return r.paths(where, tmplRefs(nodes), edge)
}

func (r refChecker) value(where string, v any) []string {
	switch x := v.(type) {
	case string:
		if err := CheckTemplate(x); err != nil {
			return []string{fmt.Sprintf("%s: %v", where, err)}
		}
		return r.template(where, x, true)
	case map[string]any:
		var out []string
		for k, e := range x {
			out = append(out, r.value(where+"."+k, e)...)
		}
		return out
	case []any:
		var out []string
		for i, e := range x {
			out = append(out, r.value(fmt.Sprintf("%s[%d]", where, i), e)...)
		}
		return out
	}
	return nil
}

// paths checks references; edge: output, status, error, attempt of the
// step that ended may be read too.
func (r refChecker) paths(where string, refs [][]string, edge bool) []string {
	var out []string
	declared := func(name string) bool {
		in := r.d.Inputs
		if in == nil || len(in.Properties) == 0 {
			return true
		}
		if ap, ok := in.Raw["additionalProperties"]; ok && ap != false {
			return true
		}
		_, ok := in.Properties[name]
		return ok
	}
	for _, p := range refs {
		root := p[0]
		switch root {
		case "inputs":
			if len(p) > 1 && !declared(p[1]) {
				out = append(out, fmt.Sprintf("%s: inputs.%s is not one of the workflow's inputs", where, p[1]))
			}
		case "steps", "nodes":
			if len(p) > 1 && !r.ids[p[1]] {
				out = append(out, fmt.Sprintf("%s: %s.%s: there is no step %s", where, root, p[1], p[1]))
			} else if len(p) > 2 && !stepFields[p[2]] {
				out = append(out, fmt.Sprintf("%s: %s.%s.%s: a step has output (result), input, summary, status, error, attempt", where, root, p[1], p[2]))
			}
		case "input", "run":
		case "output", "status", "error", "attempt":
			if !edge {
				out = append(out, fmt.Sprintf("%s: %s is only known on an edge (when, map); in a step use steps.<id>.%s", where, root, root))
			}
		default:
			out = append(out, fmt.Sprintf("%s: unknown name %q; use inputs, input, steps.<id>, nodes.<id>, run%s", where, root, map[bool]string{true: ", output, status, error, attempt"}[edge]))
		}
	}
	return out
}

// Validate is Problems as one error.
func (d *WorkflowDef) Validate() error {
	p := d.Problems()
	if len(p) == 0 {
		return nil
	}
	return Errf(CodeInvalidArgument, "the workflow has %d problem(s): %s", len(p), strings.Join(p, "; ")).
		WithSuggestion("fix them and check again with: myco workflow validate --file <file>")
}

// Step finds a step by id.
func (d *WorkflowDef) Step(id string) *StepDef {
	for i := range d.Steps {
		if d.Steps[i].ID == id {
			return &d.Steps[i]
		}
	}
	return nil
}

// Out returns the edges out of step id (with their indexes). With no edges
// at all, steps run in order.
func (d *WorkflowDef) Out(id string) []IndexedEdge {
	if len(d.Edges) == 0 {
		for i := range d.Steps {
			if d.Steps[i].ID == id {
				to := EdgeEnd
				if i+1 < len(d.Steps) {
					to = d.Steps[i+1].ID
				}
				return []IndexedEdge{{Index: i, Edge: EdgeDef{From: id, To: to}}}
			}
		}
		return nil
	}
	var out []IndexedEdge
	for i, e := range d.Edges {
		if e.From == id {
			out = append(out, IndexedEdge{Index: i, Edge: e})
		}
	}
	return out
}

// IndexedEdge is an edge and its place in Edges.
type IndexedEdge struct {
	Index int
	Edge  EdgeDef
}

// Preds lists the distinct steps with an edge into id.
func (d *WorkflowDef) Preds(id string) []string { return d.graph().preds[id] }

type wfGraph struct {
	next  map[string][]string
	preds map[string][]string
}

func (d *WorkflowDef) graph() wfGraph {
	g := wfGraph{next: map[string][]string{}, preds: map[string][]string{}}
	add := func(from, to string) {
		if to == EdgeEnd || to == EdgeFail || to == "" {
			return
		}
		g.next[from] = append(g.next[from], to)
		for _, p := range g.preds[to] {
			if p == from {
				return
			}
		}
		g.preds[to] = append(g.preds[to], from)
	}
	if len(d.Edges) == 0 {
		for i := 0; i+1 < len(d.Steps); i++ {
			add(d.Steps[i].ID, d.Steps[i+1].ID)
		}
	}
	for _, e := range d.Edges {
		add(e.From, e.To)
		add(e.From, e.Else)
	}
	return g
}

func (g wfGraph) reachable(from string) map[string]bool {
	seen := map[string]bool{from: true}
	stack := []string{from}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, m := range g.next[n] {
			if !seen[m] {
				seen[m] = true
				stack = append(stack, m)
			}
		}
	}
	return seen
}

// Workflow is a stored definition.
type Workflow struct {
	ID      string
	Name    string
	Project string
	// Owner is the member who made it (or "admin" on hubs without
	// members); OwnerID their id.
	Owner     string
	OwnerID   string
	Version   int
	Def       *WorkflowDef
	Enabled   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// WorkflowVersion is one applied definition.
type WorkflowVersion struct {
	WorkflowID string
	Version    int
	Def        *WorkflowDef
	By         string
	CreatedAt  time.Time
}

// Run states.
const (
	FlowRunning   = "running"
	FlowWaiting   = "waiting" // only decisions are open
	FlowSucceeded = "succeeded"
	FlowFailed    = "failed"
	FlowCancelled = "cancelled"
)

// FlowOver reports whether a run state is final.
func FlowOver(s string) bool { return s == FlowSucceeded || s == FlowFailed || s == FlowCancelled }

// Step instance states.
const (
	StepRunning   = "running"
	StepWaiting   = "waiting" // a decision
	StepDone      = "done"
	StepFailed    = "failed"
	StepCancelled = "cancelled"
)

// StepOver reports whether a step state is final.
func StepOver(s string) bool { return s == StepDone || s == StepFailed || s == StepCancelled }

// Run triggers.
const (
	FlowByHand       = "manual"
	FlowByAgent      = "agent"
	FlowByAutomation = "automation"
)

// WorkflowRun is one walk through a workflow.
type WorkflowRun struct {
	ID         string
	WorkflowID string
	Name       string // the workflow's name when it started
	Version    int
	Project    string
	Status     string
	Inputs     map[string]any
	// Owner/OwnerID: whose name the tasks go out in; By: who started it
	// (a member, or an agent); Trigger: manual, agent, automation.
	Owner     string
	OwnerID   string
	By        string
	Trigger   string
	Error     string
	State     RunState
	CreatedAt time.Time
	UpdatedAt time.Time
	EndedAt   time.Time
}

// RunState is the engine's bookkeeping: how often each edge was taken, and
// which steps have arrived at each join.
type RunState struct {
	Edges map[string]int      `json:"edges,omitempty"`
	Joins map[string][]string `json:"joins,omitempty"`
	// JoinIn is what each step that arrived at a join carried, by step.
	JoinIn map[string]map[string]any `json:"join_in,omitempty"`
}

// RunStep is one start of a step in a run (a step in a loop runs several
// times: Attempt counts them).
type RunStep struct {
	ID        string
	RunID     string
	StepID    string
	Kind      string
	Attempt   int
	Status    string
	Target    string // agent or computer; the person for a decision
	MessageID string
	Input     string // the task or command as sent
	// Payload is the step's input (JSON): the run's inputs, the step
	// before's result, an edge's map, or a join's object by step.
	Payload any
	Output  map[string]any
	Summary string
	Error   string
	Retries int
	Bounced bool
	// Bounces counts the times its result was sent back (v0.21.1).
	Bounces int
	// Tasks are the tasks it sent, oldest first: the first, retries and
	// send-backs (v0.21.1; MessageID is the latest).
	Tasks []string
	// Usage is what its agent's runs cost (from their traces).
	Usage     *Usage
	StartedAt time.Time
	EndedAt   time.Time
	DueAt     time.Time // timeout (zero: none)
}

// RunEvent is a line in a run's log.
type RunEvent struct {
	ID    int64
	RunID string
	At    time.Time
	Kind  string
	Step  string
	Text  string
}

// Run event kinds.
const (
	EvStarted   = "started"
	EvStep      = "step"
	EvDone      = "done"
	EvFailed    = "failed"
	EvBounced   = "bounced"
	EvEdge      = "edge"
	EvDecided   = "decided"
	EvTimeout   = "timeout"
	EvRetry     = "retry"
	EvCancelled = "cancelled"
	EvFinished  = "finished"
)

// RunFilter narrows run lists.
type RunFilter struct {
	WorkflowID string
	Active     bool   // running or waiting only
	Status     string // one state only
	Limit      int
}

// WorkflowRepository stores workflows and their runs.
type WorkflowRepository interface {
	Create(ctx context.Context, w *Workflow, by string) error
	// Save stores a changed workflow; with newVersion it also keeps
	// w.Def as version w.Version.
	Save(ctx context.Context, w *Workflow, newVersion bool, by string) error
	Get(ctx context.Context, id string) (*Workflow, error)
	GetByName(ctx context.Context, name string) (*Workflow, error)
	List(ctx context.Context) ([]*Workflow, error)
	Count(ctx context.Context) (int, error)
	Delete(ctx context.Context, id string) error
	Version(ctx context.Context, id string, version int) (*WorkflowVersion, error)

	CreateRun(ctx context.Context, r *WorkflowRun) error
	SaveRun(ctx context.Context, r *WorkflowRun) error
	GetRun(ctx context.Context, id string) (*WorkflowRun, error)
	ListRuns(ctx context.Context, f RunFilter) ([]*WorkflowRun, error)

	SaveStep(ctx context.Context, s *RunStep) error
	Steps(ctx context.Context, runID string) ([]*RunStep, error)
	// StepByMessage finds the step instance that sent message id.
	StepByMessage(ctx context.Context, messageID string) (*RunStep, error)

	AddEvent(ctx context.Context, e *RunEvent) error
	Events(ctx context.Context, runID string) ([]*RunEvent, error)

	// RunCounts counts a workflow's runs by state.
	RunCounts(ctx context.Context, workflowID string) (map[string]int, error)
	// StepsOf returns the steps of runs, by run id.
	StepsOf(ctx context.Context, runIDs []string) (map[string][]*RunStep, error)
	// AddEvaluation stores a score, dropping the run's oldest past
	// MaxRunEvaluations.
	AddEvaluation(ctx context.Context, e *RunEvaluation) error
	// Evaluations returns the scores of runs, oldest first.
	Evaluations(ctx context.Context, runIDs []string) ([]*RunEvaluation, error)
}

// MatchesRole reports whether an agent does role (分工): a label role=<x>
// or <x>, a skill named <x>, or <x> in its name, display name or
// description (or note, its role in a project). Case does not matter.
func MatchesRole(a *Agent, role, note string) bool {
	r := strings.ToLower(strings.TrimSpace(role))
	if r == "" {
		return false
	}
	for _, l := range a.Labels {
		l = strings.ToLower(l)
		if l == r || l == "role="+r {
			return true
		}
	}
	for _, sk := range a.Skills {
		if strings.ToLower(sk.Name) == r {
			return true
		}
	}
	for _, text := range []string{string(a.Name), a.DisplayName, a.Description, note} {
		if strings.Contains(strings.ToLower(text), r) {
			return true
		}
	}
	return false
}
