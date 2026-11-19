package domain

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// A workflow is a graph the hub walks: each step hands work to an agent
// (a task), runs a command on a computer (an exec node), or asks a person
// to decide; edges say which step comes next. Agents only hand back
// results: the hub alone decides what happens next (docs/design/workflow.md).

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
	// MaxRunEvents bounds what one run keeps in its log.
	MaxRunEvents = 500
	// DefaultHumanTimeout is how long a decision waits when the step names
	// no timeout; then it counts as "no".
	DefaultHumanTimeout = 72 * time.Hour
)

// WorkflowDef is what people (usually an agent) write: JSON.
type WorkflowDef struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Project     string `json:"project,omitempty"`
	// Inputs is the start form: an object schema.
	Inputs *Schema    `json:"inputs,omitempty"`
	Steps  []StepDef  `json:"steps"`
	Edges  []EdgeDef  `json:"edges,omitempty"`
	Layout *DefLayout `json:"layout,omitempty"`
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
	// Output is the shape an agent's result must have.
	Output *Schema `json:"output,omitempty"`
	// Timeout: Go duration, or "<n>d" for days ("" = none; human steps
	// wait DefaultHumanTimeout).
	Timeout string `json:"timeout,omitempty"`
	// Retry: how many more times a failed step is sent again.
	Retry int `json:"retry,omitempty"`
	// Dispatch "all": every matching edge out of it is taken (in
	// parallel), not just the first.
	Dispatch string `json:"dispatch,omitempty"`
	// Join "all": it starts only when every step with an edge into it has
	// arrived.
	Join string `json:"join,omitempty"`
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
func (d *WorkflowDef) Problems() []string {
	var out []string
	bad := func(f string, a ...any) { out = append(out, fmt.Sprintf(f, a...)) }
	d.Name = strings.TrimSpace(d.Name)
	if d.Name == "" {
		bad("name is required")
	} else if len([]rune(d.Name)) > MaxWorkflowName {
		bad("name is longer than %d characters", MaxWorkflowName)
	}
	if d.Inputs != nil {
		if p := d.Inputs.problems("inputs"); len(p) > 0 {
			out = append(out, p...)
		}
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
		s := &d.Steps[i]
		where := fmt.Sprintf("steps[%d]", i)
		if s.ID != "" {
			where = "step " + s.ID
		}
		if !stepIDRe.MatchString(s.ID) {
			bad("%s: id must be letters, digits, _ or - (starting with a letter), at most 40", where)
		} else if ids[s.ID] {
			bad("%s: id is used twice", where)
		}
		ids[s.ID] = true
		n := 0
		for _, v := range []string{s.Agent, s.Role, s.Computer, s.Human} {
			if v != "" {
				n++
			}
		}
		if n != 1 {
			bad("%s: set exactly one of agent, role, computer, human", where)
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
			if s.Output != nil || len(s.Options) > 0 || s.Prompt != "" {
				bad("%s: prompt, output and options are for agent and human steps", where)
			}
		case StepHuman:
			if len(s.Options) < 2 {
				bad(`%s: a human step needs at least two options, e.g. ["合并", "不合并"]`, where)
			}
			if s.Output != nil || s.Run != "" || s.Retry > 0 {
				bad("%s: output, run and retry are for agent and computer steps", where)
			}
		}
		if _, err := ParseWorkflowDuration(s.Timeout); err != nil {
			bad("%s: timeout %q is not a duration (e.g. 30m, 2h, 1d)", where, s.Timeout)
		}
		if s.Retry < 0 || s.Retry > 5 {
			bad("%s: retry must be 0 to 5", where)
		}
		if s.Dispatch != "" && s.Dispatch != "all" {
			bad(`%s: dispatch can only be "all"`, where)
		}
		if s.Join != "" && s.Join != "all" {
			bad(`%s: join can only be "all"`, where)
		}
		if s.Output != nil {
			out = append(out, s.Output.problems(where+": output")...)
		}
		if len([]rune(s.Role)) > 40 {
			bad("%s: role is longer than 40 characters", where)
		}
		for _, t := range []string{s.Prompt, s.Run, s.Title, s.Agent, s.Computer, s.Role} {
			if err := CheckTemplate(t); err != nil {
				bad("%s: %v", where, err)
			}
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
			if _, err := ParseExpr(e.When); err != nil {
				bad("%s: when: %v", where, err)
			}
		}
	}
	if len(out) > 0 {
		return out
	}
	// Loops must be bounded; every step must be reachable; joins need
	// several ways in.
	// Every loop has an edge to the same or an earlier step (in steps
	// order), so bounding those bounds every loop.
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
		if back(e.To) && e.MaxIterations == 0 {
			bad("edges[%d] (%s → %s) goes back to an earlier step: give it maxIterations (and else for what happens after)", i, e.From, e.To)
		}
	}
	seen := g.reachable(d.Steps[0].ID)
	for _, s := range d.Steps {
		if !seen[s.ID] {
			bad("step %s: no edge leads to it from the first step", s.ID)
		}
		if s.Join == "all" && len(g.preds[s.ID]) < 2 {
			bad("step %s: join all needs edges from at least two steps", s.ID)
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
	Output    map[string]any
	Summary   string
	Error     string
	Retries   int
	Bounced   bool
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
	Active     bool // running or waiting only
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
