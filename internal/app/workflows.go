package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chenhg5/myco/internal/domain"
)

// WorkflowService keeps workflows and walks their runs. Each agent step is
// an ordinary task sent in the name of whoever started the run (tagged with
// the run's id); computer steps are commands for an exec node; human steps
// wait for a decision. The service alone decides what comes next: when a
// task is answered (Kick, or the next Tick), it reads the result, checks
// it, and follows the edges (docs/design/workflow.md).
type WorkflowService struct {
	repo     domain.WorkflowRepository
	messages *MessageService
	msgRepo  domain.MessageRepository
	ids      IDGenerator
	clock    Clock
	log      *slog.Logger
	// One advance at a time: runs are small and few.
	mu   sync.Mutex
	kick chan struct{}
}

// WorkflowTick is how often runs are checked without a kick.
const WorkflowTick = 5 * time.Second

func NewWorkflowService(repo domain.WorkflowRepository, messages *MessageService, msgRepo domain.MessageRepository,
	ids IDGenerator, clock Clock, log *slog.Logger) *WorkflowService {
	if log == nil {
		log = slog.Default()
	}
	s := &WorkflowService{repo: repo, messages: messages, msgRepo: msgRepo, ids: ids, clock: clock, log: log, kick: make(chan struct{}, 1)}
	messages.SetWorkflowHook(func(string) { s.Kick() })
	return s
}

// Kick asks the loop to look at runs now (a workflow task was answered).
func (s *WorkflowService) Kick() {
	select {
	case s.kick <- struct{}{}:
	default:
	}
}

// Run checks runs every tick, and at once when kicked, until ctx is done.
func (s *WorkflowService) Run(ctx context.Context, tick time.Duration) {
	t := time.NewTicker(tick)
	defer t.Stop()
	s.Tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-s.kick:
			// Let a burst of replies land first.
			time.Sleep(200 * time.Millisecond)
		}
		s.Tick(ctx)
	}
}

func ownsBy(c Caller, owner, ownerID string) bool {
	if c.Manages {
		return true
	}
	if ownerID != "" {
		return ownerID == c.MemberID
	}
	return owner == c.Name
}

// WorkflowCheck is what validating a definition found: problems block
// saving it; warnings (an agent that is not there yet) do not.
type WorkflowCheck struct {
	Problems []string
	Warnings []string
}

// Check validates def and looks up the agents and computers it names.
func (s *WorkflowService) Check(ctx context.Context, def *domain.WorkflowDef) WorkflowCheck {
	out := WorkflowCheck{Problems: def.Problems()}
	r := aliasResolver{agents: s.messages.agents, aliases: s.messages.aliases}
	var everyone []*domain.Agent
	for _, st := range def.Steps {
		if st.Role != "" {
			if everyone == nil {
				everyone, _ = s.messages.agents.List(ctx)
			}
			if !strings.Contains(st.Role, "{{") && !slices.ContainsFunc(everyone, func(a *domain.Agent) bool {
				return a.Kind() == domain.KindAgent && domain.MatchesRole(a, st.Role, "")
			}) {
				out.Warnings = append(out.Warnings, fmt.Sprintf("step %s: no agent has the role %q yet", st.ID, st.Role))
			}
			continue
		}
		var name string
		switch st.Kind() {
		case domain.StepAgent:
			name = st.Agent
		case domain.StepComputer:
			name = st.Computer
		default:
			continue
		}
		if strings.Contains(name, "{{") {
			continue
		}
		n, err := domain.ParseAgentName(name)
		if err != nil {
			out.Problems = append(out.Problems, fmt.Sprintf("step %s: %v", st.ID, err))
			continue
		}
		a, _, err := r.resolve(ctx, n)
		if err != nil {
			out.Warnings = append(out.Warnings, fmt.Sprintf("step %s: there is no agent %s yet", st.ID, name))
			continue
		}
		if st.Kind() == domain.StepComputer && a.Binding.Harness != domain.HarnessExec {
			out.Warnings = append(out.Warnings, fmt.Sprintf("step %s: %s is not a computer (an exec node); it will get the command as a task", st.ID, name))
		}
	}
	return out
}

// ApplyOut reports what Apply did.
type ApplyOut struct {
	Workflow *domain.Workflow
	Created  bool
	Changed  bool
	Check    WorkflowCheck
}

// Apply creates the workflow named in def, or gives the existing one a new
// version when def differs.
func (s *WorkflowService) Apply(ctx context.Context, c Caller, def *domain.WorkflowDef, dryRun bool) (*ApplyOut, error) {
	chk := s.Check(ctx, def)
	if len(chk.Problems) > 0 {
		return nil, domain.Errf(domain.CodeInvalidArgument, "the workflow has %d problem(s): %s", len(chk.Problems), strings.Join(chk.Problems, "; ")).
			WithSuggestion("fix them and check again with: myco workflow validate --file <file>")
	}
	if def.Project != "" && s.messages.projects != nil {
		if _, err := s.messages.projects.Get(ctx, def.Project); err != nil {
			return nil, domain.Errf(domain.CodeNotFound, "project %s not found", def.Project).WithSuggestion("list projects with: myco project list")
		}
	}
	now := s.clock.Now()
	w, err := s.repo.GetByName(ctx, def.Name)
	switch {
	case domain.IsCode(err, domain.CodeNotFound):
		if n, err := s.repo.Count(ctx); err != nil {
			return nil, err
		} else if n >= domain.MaxWorkflows {
			return nil, domain.Errf(domain.CodeInvalidArgument, "this workspace has %d workflows, the most it can have", n)
		}
		w = &domain.Workflow{ID: s.ids.New("wf"), Name: def.Name, Project: def.Project, Owner: c.Name, OwnerID: c.MemberID,
			Version: 1, Def: def, Enabled: true, CreatedAt: now, UpdatedAt: now}
		if !dryRun {
			if err := s.repo.Create(ctx, w, c.who()); err != nil {
				return nil, err
			}
		}
		return &ApplyOut{Workflow: w, Created: true, Changed: true, Check: chk}, nil
	case err != nil:
		return nil, err
	}
	if !ownsBy(c, w.Owner, w.OwnerID) {
		return nil, domain.Errf(domain.CodePermissionDenied, "workflow %s belongs to %s", w.Name, w.Owner).
			WithSuggestion("give yours another name, or ask %s (or someone who manages the workspace) to change it", w.Owner)
	}
	if defJSONEqual(w.Def, def) {
		return &ApplyOut{Workflow: w, Check: chk}, nil
	}
	w.Def, w.Project, w.UpdatedAt = def, def.Project, now
	w.Version++
	if !dryRun {
		if err := s.repo.Save(ctx, w, true, c.who()); err != nil {
			return nil, err
		}
	}
	return &ApplyOut{Workflow: w, Changed: true, Check: chk}, nil
}

func defJSONEqual(a, b *domain.WorkflowDef) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}

func (c Caller) who() string {
	if c.Agent != "" {
		return string(c.Agent)
	}
	return c.Name
}

// find loads a workflow by id or name that the caller may see.
func (s *WorkflowService) find(ctx context.Context, c Caller, ref string) (*domain.Workflow, error) {
	w, err := s.repo.Get(ctx, ref)
	if domain.IsCode(err, domain.CodeNotFound) {
		w, err = s.repo.GetByName(ctx, ref)
	}
	if err != nil {
		return nil, err
	}
	if !ownsBy(c, w.Owner, w.OwnerID) {
		return nil, domain.Errf(domain.CodeNotFound, "workflow %s not found", ref)
	}
	return w, nil
}

// NameOf is a workflow's name ("" when it is gone).
func (s *WorkflowService) NameOf(ctx context.Context, id string) string {
	if w, err := s.repo.Get(ctx, id); err == nil {
		return w.Name
	}
	return ""
}

// WorkflowOut is a workflow with how its runs are going.
type WorkflowOut struct {
	Workflow *domain.Workflow
	Active   int
	LastRun  *domain.WorkflowRun
}

// List returns the workflows the caller may see.
func (s *WorkflowService) List(ctx context.Context, c Caller) ([]*WorkflowOut, error) {
	all, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	out := []*WorkflowOut{}
	for _, w := range all {
		if !ownsBy(c, w.Owner, w.OwnerID) {
			continue
		}
		o := &WorkflowOut{Workflow: w}
		if runs, err := s.repo.ListRuns(ctx, domain.RunFilter{WorkflowID: w.ID, Limit: 1}); err == nil && len(runs) > 0 {
			o.LastRun = runs[0]
		}
		if runs, err := s.repo.ListRuns(ctx, domain.RunFilter{WorkflowID: w.ID, Active: true}); err == nil {
			o.Active = len(runs)
		}
		out = append(out, o)
	}
	return out, nil
}

// Get returns one workflow (by id or name).
func (s *WorkflowService) Get(ctx context.Context, c Caller, ref string) (*WorkflowOut, error) {
	w, err := s.find(ctx, c, ref)
	if err != nil {
		return nil, err
	}
	o := &WorkflowOut{Workflow: w}
	if runs, err := s.repo.ListRuns(ctx, domain.RunFilter{WorkflowID: w.ID, Active: true}); err == nil {
		o.Active = len(runs)
	}
	return o, nil
}

// SetEnabled turns a workflow on or off (off: it cannot be started).
func (s *WorkflowService) SetEnabled(ctx context.Context, c Caller, ref string, on bool) (*domain.Workflow, error) {
	w, err := s.find(ctx, c, ref)
	if err != nil {
		return nil, err
	}
	w.Enabled, w.UpdatedAt = on, s.clock.Now()
	return w, s.repo.Save(ctx, w, false, c.who())
}

// Delete removes a workflow and its runs, stopping the ones still going.
func (s *WorkflowService) Delete(ctx context.Context, c Caller, ref string) error {
	w, err := s.find(ctx, c, ref)
	if err != nil {
		return err
	}
	runs, err := s.repo.ListRuns(ctx, domain.RunFilter{WorkflowID: w.ID, Active: true})
	if err != nil {
		return err
	}
	for _, r := range runs {
		if _, err := s.Cancel(ctx, c, r.ID); err != nil {
			s.log.Warn("workflows: stopping a run of a removed workflow failed", "run", r.ID, "err", err)
		}
	}
	return s.repo.Delete(ctx, w.ID)
}

// RunDetail is a run with its steps, log and the definition it follows.
type RunDetail struct {
	Run    *domain.WorkflowRun
	Def    *domain.WorkflowDef
	Steps  []*domain.RunStep
	Events []*domain.RunEvent
}

// StartInput starts a run.
type StartInput struct {
	Inputs  map[string]any
	Trigger string // manual (default), agent, automation
	DryRun  bool
}

// StartPreview is what a dry run would send first.
type StartPreview struct {
	Inputs map[string]any
	Step   string
	Target string
	Body   string
}

// Start begins a run of the workflow. With DryRun it only checks the
// inputs and shows the first step.
func (s *WorkflowService) Start(ctx context.Context, c Caller, ref string, in StartInput) (*RunDetail, *StartPreview, error) {
	w, err := s.find(ctx, c, ref)
	if err != nil {
		return nil, nil, err
	}
	if !w.Enabled {
		return nil, nil, domain.Errf(domain.CodeInvalidState, "workflow %s is turned off", w.Name).
			WithSuggestion("turn it on with: myco workflow enable --name %q", w.Name)
	}
	inputs := w.Def.Inputs.Coerce(in.Inputs)
	if w.Def.Inputs != nil {
		if p := w.Def.Inputs.Check(inputs); len(p) > 0 {
			return nil, nil, domain.Errf(domain.CodeInvalidArgument, "the inputs do not fit: %s", strings.Join(p, "; ")).
				WithSuggestion("see what it takes with: myco workflow get --name %q", w.Name)
		}
	}
	trigger := in.Trigger
	if trigger == "" {
		trigger = domain.FlowByHand
		if c.Agent != "" {
			trigger = domain.FlowByAgent
		}
	}
	now := s.clock.Now()
	run := &domain.WorkflowRun{ID: s.ids.New("run"), WorkflowID: w.ID, Name: w.Name, Version: w.Version, Project: w.Project,
		Status: domain.FlowRunning, Inputs: inputs, Owner: c.Name, OwnerID: c.MemberID, By: c.who(), Trigger: trigger,
		CreatedAt: now, UpdatedAt: now}
	if in.DryRun {
		f := &flow{s: s, ctx: ctx, run: run, def: w.Def}
		first := &w.Def.Steps[0]
		body, target, err := f.compose(first, 1)
		if err != nil {
			return nil, nil, domain.Errf(domain.CodeInvalidArgument, "step %s: %v", first.ID, err)
		}
		return nil, &StartPreview{Inputs: inputs, Step: first.ID, Target: target, Body: body}, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.repo.CreateRun(ctx, run); err != nil {
		return nil, nil, err
	}
	f := &flow{s: s, ctx: ctx, run: run, def: w.Def, dirty: map[*domain.RunStep]bool{}}
	f.event(domain.EvStarted, "", fmt.Sprintf("%s 启动了（第 %d 版）", c.who(), w.Version))
	f.activate(w.Def.Steps[0].ID)
	f.settle()
	if err := f.save(); err != nil {
		return nil, nil, err
	}
	return s.detail(ctx, run)
}

func (s *WorkflowService) detail(ctx context.Context, run *domain.WorkflowRun) (*RunDetail, *StartPreview, error) {
	d, err := s.runDetail(ctx, run)
	return d, nil, err
}

func (s *WorkflowService) runDetail(ctx context.Context, run *domain.WorkflowRun) (*RunDetail, error) {
	def, err := s.defOf(ctx, run)
	if err != nil {
		return nil, err
	}
	steps, err := s.repo.Steps(ctx, run.ID)
	if err != nil {
		return nil, err
	}
	events, err := s.repo.Events(ctx, run.ID)
	if err != nil {
		return nil, err
	}
	return &RunDetail{Run: run, Def: def, Steps: steps, Events: events}, nil
}

func (s *WorkflowService) defOf(ctx context.Context, run *domain.WorkflowRun) (*domain.WorkflowDef, error) {
	v, err := s.repo.Version(ctx, run.WorkflowID, run.Version)
	if err != nil {
		return nil, err
	}
	return v.Def, nil
}

func mayRun(c Caller, run *domain.WorkflowRun) bool { return ownsBy(c, run.Owner, run.OwnerID) }

// getRun loads a run the caller may see.
func (s *WorkflowService) getRun(ctx context.Context, c Caller, id string) (*domain.WorkflowRun, error) {
	run, err := s.repo.GetRun(ctx, id)
	if err != nil {
		return nil, err
	}
	if !mayRun(c, run) {
		return nil, domain.Errf(domain.CodeNotFound, "workflow run %s not found", id)
	}
	return run, nil
}

// GetRun returns a run with its steps and log.
func (s *WorkflowService) GetRun(ctx context.Context, c Caller, id string) (*RunDetail, error) {
	run, err := s.getRun(ctx, c, id)
	if err != nil {
		return nil, err
	}
	return s.runDetail(ctx, run)
}

// Runs lists runs the caller may see: of one workflow (ref) or all, the
// newest first.
func (s *WorkflowService) Runs(ctx context.Context, c Caller, ref string, active bool, limit int) ([]*domain.WorkflowRun, error) {
	f := domain.RunFilter{Active: active, Limit: limit}
	if ref != "" {
		w, err := s.find(ctx, c, ref)
		if err != nil {
			return nil, err
		}
		f.WorkflowID = w.ID
	}
	runs, err := s.repo.ListRuns(ctx, f)
	if err != nil {
		return nil, err
	}
	out := []*domain.WorkflowRun{}
	for _, r := range runs {
		if mayRun(c, r) {
			out = append(out, r)
		}
	}
	return out, nil
}

// Decision is a human step waiting for someone.
type Decision struct {
	Run  *domain.WorkflowRun
	Step *domain.RunStep
	Def  *domain.StepDef
}

// Decisions lists the decisions waiting for the caller (everyone's, for
// those who manage the workspace).
func (s *WorkflowService) Decisions(ctx context.Context, c Caller) ([]*Decision, error) {
	runs, err := s.repo.ListRuns(ctx, domain.RunFilter{Active: true})
	if err != nil {
		return nil, err
	}
	out := []*Decision{}
	for _, r := range runs {
		steps, err := s.repo.Steps(ctx, r.ID)
		if err != nil {
			return nil, err
		}
		var def *domain.WorkflowDef
		for _, st := range steps {
			if st.Status != domain.StepWaiting || !mayDecide(c, r, st) {
				continue
			}
			if def == nil {
				if def, err = s.defOf(ctx, r); err != nil {
					return nil, err
				}
			}
			out = append(out, &Decision{Run: r, Step: st, Def: def.Step(st.StepID)})
		}
	}
	return out, nil
}

func mayDecide(c Caller, run *domain.WorkflowRun, st *domain.RunStep) bool {
	return c.Manages || ownsBy(c, run.Owner, run.OwnerID) || (st.Target != "" && st.Target == c.Name)
}

// Cancel stops a run: its open tasks are stopped, its decisions withdrawn.
func (s *WorkflowService) Cancel(ctx context.Context, c Caller, id string) (*domain.WorkflowRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load(ctx, c, id)
	if err != nil {
		return nil, err
	}
	if domain.FlowOver(f.run.Status) {
		return f.run, nil
	}
	f.quiet = c.Agent != "" && string(c.Agent) == f.run.By
	f.end(domain.FlowCancelled, c.who()+" 取消了")
	return f.run, f.save()
}

// Retry restarts a failed or cancelled run at a step (default: the one
// that failed), keeping what the steps before it handed back.
func (s *WorkflowService) Retry(ctx context.Context, c Caller, id, from string) (*domain.WorkflowRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.load(ctx, c, id)
	if err != nil {
		return nil, err
	}
	if !domain.FlowOver(f.run.Status) {
		return nil, domain.Errf(domain.CodeInvalidState, "run %s is still going", id).WithSuggestion("cancel it first, or wait for it to end")
	}
	if from == "" {
		for i := len(f.steps) - 1; i >= 0; i-- {
			if st := f.steps[i]; st.Status == domain.StepFailed || st.Status == domain.StepCancelled {
				from = st.StepID
				break
			}
		}
	}
	if from == "" {
		return nil, domain.Errf(domain.CodeInvalidArgument, "no step failed in run %s", id).WithSuggestion("name the step to start from: --from <step>")
	}
	if f.def.Step(from) == nil {
		return nil, domain.Errf(domain.CodeNotFound, "the workflow has no step %s", from)
	}
	f.run.Status, f.run.Error, f.run.EndedAt = domain.FlowRunning, "", time.Time{}
	f.run.State.Joins = nil
	f.event(domain.EvRetry, from, c.who()+" 从这一步重新开始")
	f.activate(from)
	f.settle()
	return f.run, f.save()
}

// Decide answers a human step.
func (s *WorkflowService) Decide(ctx context.Context, c Caller, id, step, choice, note string) (*domain.WorkflowRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, err := s.repo.GetRun(ctx, id)
	if err != nil {
		return nil, err
	}
	f, err := s.open(ctx, run)
	if err != nil {
		return nil, err
	}
	var st *domain.RunStep
	for _, x := range f.steps {
		if x.StepID == step && x.Status == domain.StepWaiting {
			st = x
		}
	}
	if st == nil {
		return nil, domain.Errf(domain.CodeInvalidState, "step %s of run %s is not waiting for a decision", step, id)
	}
	if !mayDecide(c, run, st) {
		return nil, domain.Errf(domain.CodePermissionDenied, "this decision is %s's to make", st.Target)
	}
	def := f.def.Step(step)
	if !slices.Contains(def.Options, choice) {
		return nil, domain.Errf(domain.CodeInvalidArgument, "choose one of: %s", strings.Join(def.Options, ", "))
	}
	f.decided(st, choice, strings.TrimSpace(note), c.who())
	f.settle()
	return f.run, f.save()
}

// load opens a run the caller may change.
func (s *WorkflowService) load(ctx context.Context, c Caller, id string) (*flow, error) {
	run, err := s.getRun(ctx, c, id)
	if err != nil {
		return nil, err
	}
	return s.open(ctx, run)
}

func (s *WorkflowService) open(ctx context.Context, run *domain.WorkflowRun) (*flow, error) {
	def, err := s.defOf(ctx, run)
	if err != nil {
		return nil, err
	}
	steps, err := s.repo.Steps(ctx, run.ID)
	if err != nil {
		return nil, err
	}
	return &flow{s: s, ctx: ctx, run: run, def: def, steps: steps, dirty: map[*domain.RunStep]bool{}}, nil
}

// Tick moves every active run along: answered tasks, timeouts, decisions
// that expired. It is also what picks runs up again after a restart.
func (s *WorkflowService) Tick(ctx context.Context) {
	runs, err := s.repo.ListRuns(ctx, domain.RunFilter{Active: true})
	if err != nil {
		s.log.Warn("workflows: reading active runs failed", "err", err)
		return
	}
	for _, r := range runs {
		if ctx.Err() != nil {
			return
		}
		s.mu.Lock()
		if err := s.tickRun(ctx, r.ID); err != nil {
			s.log.Warn("workflows: moving a run along failed", "run", r.ID, "err", err)
		}
		s.mu.Unlock()
	}
}

func (s *WorkflowService) tickRun(ctx context.Context, id string) error {
	run, err := s.repo.GetRun(ctx, id) // fresh, under the lock
	if err != nil || domain.FlowOver(run.Status) {
		return err
	}
	f, err := s.open(ctx, run)
	if err != nil {
		return err
	}
	f.poll()
	f.settle()
	return f.save()
}

// flow is one run being moved along (under WorkflowService.mu).
type flow struct {
	s     *WorkflowService
	ctx   context.Context
	run   *domain.WorkflowRun
	def   *domain.WorkflowDef
	steps []*domain.RunStep
	dirty map[*domain.RunStep]bool
	// changed: the run itself needs saving.
	changed bool
	// quiet: do not tell the agent that started it that it ended (it
	// cancelled the run itself).
	quiet bool
	// picked says why the last step by role went to whom.
	picked string
}

func (f *flow) now() time.Time { return f.s.clock.Now() }

func (f *flow) event(kind, step, text string) {
	if err := f.s.repo.AddEvent(f.ctx, &domain.RunEvent{RunID: f.run.ID, At: f.now(), Kind: kind, Step: step, Text: text}); err != nil {
		f.s.log.Warn("workflows: writing the run log failed", "run", f.run.ID, "err", err)
	}
}

func (f *flow) touch(st *domain.RunStep) { f.dirty[st] = true }

func (f *flow) save() error {
	for st := range f.dirty {
		if err := f.s.repo.SaveStep(f.ctx, st); err != nil {
			return err
		}
	}
	f.dirty = map[*domain.RunStep]bool{}
	f.run.UpdatedAt = f.now()
	return f.s.repo.SaveRun(f.ctx, f.run)
}

// env is what conditions and templates see. With st, output and status
// are that step's.
func (f *flow) env(st *domain.RunStep) map[string]any {
	steps := map[string]any{}
	for _, x := range f.steps {
		if x.Status == domain.StepDone || x.Status == domain.StepFailed {
			steps[x.StepID] = map[string]any{"output": anyMap(x.Output), "summary": x.Summary, "status": x.Status, "attempt": float64(x.Attempt), "error": x.Error}
		}
	}
	env := map[string]any{
		"inputs": anyMap(f.run.Inputs),
		"steps":  steps,
		"run":    map[string]any{"id": f.run.ID, "name": f.run.Name, "by": f.run.By},
	}
	if st != nil {
		env["output"] = anyMap(st.Output)
		env["status"] = st.Status
		env["attempt"] = float64(st.Attempt)
		env["error"] = st.Error
	}
	return env
}

func anyMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func (f *flow) count(stepID string) int {
	n := 0
	for _, x := range f.steps {
		if x.StepID == stepID {
			n++
		}
	}
	return n
}

// activate starts a step.
func (f *flow) activate(id string) {
	if domain.FlowOver(f.run.Status) {
		return
	}
	if len(f.steps) >= domain.MaxRunSteps {
		f.end(domain.FlowFailed, fmt.Sprintf("这次运行已经走了 %d 步，停下来了（是不是有个没完的循环？）", domain.MaxRunSteps))
		return
	}
	def := f.def.Step(id)
	now := f.now()
	st := &domain.RunStep{ID: f.s.ids.New("rs"), RunID: f.run.ID, StepID: id, Kind: def.Kind(), Attempt: f.count(id) + 1,
		Status: domain.StepRunning, StartedAt: now}
	if d := def.TimeoutDur(); d > 0 {
		st.DueAt = now.Add(d)
	}
	f.steps = append(f.steps, st)
	f.touch(st)
	body, target, err := f.compose(def, st.Attempt)
	st.Input, st.Target = body, target
	if err != nil {
		f.failed(st, fmt.Sprintf("写不出这一步的内容：%v", err))
		return
	}
	if def.Kind() == domain.StepHuman {
		st.Status = domain.StepWaiting
		f.event(domain.EvStep, id, "等 "+target+" 决定")
		return
	}
	if err := f.send(st, body, ""); err != nil {
		f.failed(st, "发不出去："+errText(err))
		return
	}
	if def.Role != "" {
		f.event(domain.EvStep, id, fmt.Sprintf("按分工「%s」交给 %s（%s）", def.Role, target, f.picked))
		return
	}
	f.event(domain.EvStep, id, "交给 "+target)
}

// compose renders a step: the task (or command, or question) and who it
// goes to.
func (f *flow) compose(def *domain.StepDef, attempt int) (body, target string, err error) {
	env := f.env(nil)
	switch def.Kind() {
	case domain.StepHuman:
		target = def.Human
		if target == domain.StarterHuman {
			target = f.run.Owner
		}
		text := def.Prompt
		if text == "" {
			text = def.Title
		}
		body, err = domain.RenderTemplate(text, env)
		return body, target, err
	case domain.StepComputer:
		if target, err = domain.RenderTemplate(def.Computer, env); err != nil {
			return "", "", err
		}
		body, err = domain.RenderTemplate(def.Run, env)
		return body, target, err
	}
	if def.Role != "" {
		role, err := domain.RenderTemplate(def.Role, env)
		if err != nil {
			return "", "", err
		}
		pick, why, err := f.s.pickByRole(f.ctx, f.run, role)
		if err != nil {
			return "", "", err
		}
		target, f.picked = string(pick), why
	} else if target, err = domain.RenderTemplate(def.Agent, env); err != nil {
		return "", "", err
	}
	prompt, err := domain.RenderTemplate(def.Prompt, env)
	if err != nil {
		return "", "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[工作流「%s」· 第 %d 步 %s", f.run.Name, f.stepIndex(def.ID)+1, def.Label())
	if attempt > 1 {
		fmt.Fprintf(&b, " · 第 %d 次", attempt)
	}
	b.WriteString("]\n\n")
	b.WriteString(strings.TrimSpace(prompt))
	b.WriteString("\n\n---\n这是工作流里的一步：只做这一步，不要自己安排下一步，工作流会接着走。\n")
	if def.Output != nil {
		ex, _ := json.MarshalIndent(withSummary(def.Output.Example()), "", "  ")
		b.WriteString("做完后交一个结构化结果（JSON 对象）：\n  myco message reply --id <这条任务的 id> --body \"给人看的说明\" --result result.json\n")
		b.WriteString("result.json 像这样：\n" + string(ex) + "\n")
		if req := def.Output.Required; len(req) > 0 {
			b.WriteString("必须有：" + strings.Join(req, "、") + "。")
		}
		b.WriteString("summary 一句话说做了什么，reason 说为什么。旧版 myco 没有 --result：把这个 JSON 放在回复最后的 ```json 代码块里。\n")
	} else {
		b.WriteString("做完照常 myco message reply 回复，第一行写一句话总结。\n")
	}
	return b.String(), target, nil
}

func withSummary(v any) any {
	m, ok := v.(map[string]any)
	if !ok {
		return v
	}
	if _, has := m["summary"]; !has {
		m["summary"] = "一句话说做了什么"
	}
	if _, has := m["reason"]; !has {
		m["reason"] = "为什么这样"
	}
	return m
}

func (f *flow) stepIndex(id string) int {
	for i, s := range f.def.Steps {
		if s.ID == id {
			return i
		}
	}
	return 0
}

// send sends st's task (in thread, for a follow-up) and records its id.
func (f *flow) send(st *domain.RunStep, body, thread string) error {
	out, err := f.s.messages.SendFromMember(f.ctx, f.run.Owner, f.run.OwnerID, SendInput{
		To: st.Target, Type: string(domain.TypeTask), Body: body, ThreadID: thread, Project: f.projectFor(thread), Workflow: f.run.ID,
	})
	if err != nil {
		return err
	}
	st.MessageID = out.Message.ID
	f.touch(st)
	return nil
}

func (f *flow) projectFor(thread string) string {
	if thread != "" {
		return ""
	}
	return f.run.Project
}

// RecentlySeen is how long after its last contact an agent counts as
// around when picking one by role.
const RecentlySeen = 10 * time.Minute

// pickByRole chooses the agent for a step by role (分工): of the agents the
// run's owner may give tasks to whose role matches (in the run's project
// first, when some there do), the one with the fewest open tasks, then the
// one seen most recently. why says how busy it is.
func (s *WorkflowService) pickByRole(ctx context.Context, run *domain.WorkflowRun, role string) (domain.AgentName, string, error) {
	all, err := s.messages.agents.List(ctx)
	if err != nil {
		return "", "", err
	}
	notes := map[domain.AgentName]string{}
	inProject := map[domain.AgentName]bool{}
	if run.Project != "" && s.messages.projects != nil {
		if p, err := s.messages.projects.Get(ctx, run.Project); err == nil {
			if ms, err := s.messages.projects.Members(ctx, p.ID); err == nil {
				for _, m := range ms {
					notes[m.Agent], inProject[m.Agent] = m.Note, true
				}
			}
		}
	}
	sender := &domain.Agent{Name: ConsoleSender(run.Owner), Owner: run.OwnerID}
	var cands []*domain.Agent
	for _, a := range all {
		if a.Kind() == domain.KindAgent && domain.MaySendTo(sender, a) && domain.MatchesRole(a, role, notes[a.Name]) {
			cands = append(cands, a)
		}
	}
	if len(cands) == 0 {
		return "", "", domain.Errf(domain.CodeNotFound, "没有分工是「%s」的 agent", role).
			WithSuggestion("give one that role: a label role=%s, or say it in its description", role)
	}
	if slices.ContainsFunc(cands, func(a *domain.Agent) bool { return inProject[a.Name] }) {
		cands = slices.DeleteFunc(cands, func(a *domain.Agent) bool { return !inProject[a.Name] })
	}
	open := map[domain.AgentName]int{}
	if counts, err := s.msgRepo.OpenCounts(ctx); err == nil {
		for name, by := range counts {
			for _, n := range by {
				open[name] += n
			}
		}
	}
	now := s.clock.Now()
	around := func(a *domain.Agent) bool { return now.Sub(a.LastSeenAt) < RecentlySeen }
	slices.SortStableFunc(cands, func(a, b *domain.Agent) int {
		if open[a.Name] != open[b.Name] {
			return open[a.Name] - open[b.Name]
		}
		if around(a) != around(b) {
			if around(a) {
				return -1
			}
			return 1
		}
		return strings.Compare(string(a.Name), string(b.Name))
	})
	best := cands[0]
	why := "空闲"
	if n := open[best.Name]; n > 0 {
		why = fmt.Sprintf("手上还有 %d 件，已是最少的", n)
	}
	return best.Name, why, nil
}

// poll looks at every open step.
func (f *flow) poll() {
	now := f.now()
	for _, st := range slices.Clone(f.steps) {
		if domain.FlowOver(f.run.Status) {
			return
		}
		switch st.Status {
		case domain.StepWaiting:
			if !st.DueAt.IsZero() && now.After(st.DueAt) {
				f.event(domain.EvTimeout, st.StepID, "没人决定，过期了：当作不做")
				st.Output = map[string]any{"choice": "", "timed_out": true}
				st.Summary = "过期了，没人决定"
				f.finish(st)
			}
		case domain.StepRunning:
			f.pollTask(st, now)
		}
	}
}

func (f *flow) pollTask(st *domain.RunStep, now time.Time) {
	if st.MessageID == "" {
		f.failed(st, "这一步的任务没有发出去")
		return
	}
	m, err := f.s.msgRepo.Get(f.ctx, st.MessageID)
	if err != nil {
		if domain.IsCode(err, domain.CodeNotFound) {
			f.failed(st, "这一步的任务不见了")
		}
		return
	}
	if !m.Status.Terminal() {
		if !st.DueAt.IsZero() && now.After(st.DueAt) {
			f.stopTask(st)
			f.event(domain.EvTimeout, st.StepID, "超时了")
			f.failed(st, "超时了（"+f.def.Step(st.StepID).Timeout+"）")
		}
		return
	}
	reply, err := f.s.msgRepo.FindReply(f.ctx, st.MessageID)
	if err != nil && !domain.IsCode(err, domain.CodeNotFound) {
		return
	}
	if m.Status == domain.StatusFailed || (reply != nil && reply.Type == domain.TypeEvent) {
		why := m.Error
		if why == "" && reply != nil {
			why = firstLine(reply.Body, 300)
		}
		if why == "" {
			why = "任务失败了"
		}
		f.failed(st, why)
		return
	}
	f.collect(st, reply)
}

// collect reads a finished task's answer into the step.
func (f *flow) collect(st *domain.RunStep, reply *domain.Message) {
	def := f.def.Step(st.StepID)
	if reply == nil {
		if def.Output != nil {
			f.failed(st, "任务被关掉了，没有交结果")
			return
		}
		st.Output, st.Summary = map[string]any{"text": ""}, "（没有回复）"
		f.finish(st)
		return
	}
	if def.Kind() == domain.StepComputer {
		f.collectCommand(st, def, reply)
		return
	}
	var out map[string]any
	if reply.Result != "" {
		_ = json.Unmarshal([]byte(reply.Result), &out)
	} else if def.Output != nil {
		out = lastJSONBlock(reply.Body)
	}
	if def.Output != nil {
		var v any = out
		if out == nil {
			v = map[string]any{} // nothing handed back: say what is missing
		}
		if problems := def.Output.Check(v); len(problems) > 0 {
			if !st.Bounced {
				f.bounce(st, def, reply, problems)
				return
			}
			f.failed(st, "交的结果不符合要求："+strings.Join(problems, "；"))
			return
		}
	}
	if out == nil {
		out = map[string]any{}
	}
	if _, has := out["text"]; !has {
		out["text"] = reply.Body
	}
	st.Output = out
	st.Summary = summaryOf(out, reply.Body)
	f.finish(st)
}

func (f *flow) collectCommand(st *domain.RunStep, def *domain.StepDef, reply *domain.Message) {
	var out map[string]any
	if err := json.Unmarshal([]byte(reply.Body), &out); err != nil || out["exit_code"] == nil {
		out = map[string]any{"exit_code": float64(0), "stdout": reply.Body}
	}
	if def.Parse == "json" {
		var parsed any
		if s, _ := out["stdout"].(string); json.Unmarshal([]byte(strings.TrimSpace(s)), &parsed) == nil {
			out["json"] = parsed
		}
	}
	st.Output = out
	code, _ := out["exit_code"].(float64)
	st.Summary = fmt.Sprintf("退出码 %d", int(code))
	if code != 0 {
		why := st.Summary
		if e, _ := out["error"].(string); e != "" {
			why += "：" + e
		} else if s, _ := out["stderr"].(string); strings.TrimSpace(s) != "" {
			why += "：" + lastLine(s, 300)
		}
		f.failed(st, why)
		return
	}
	f.finish(st)
}

// bounce sends the task back once: the result did not fit.
func (f *flow) bounce(st *domain.RunStep, def *domain.StepDef, reply *domain.Message, problems []string) {
	ex, _ := json.MarshalIndent(withSummary(def.Output.Example()), "", "  ")
	body := "交的结果不符合这一步要的结构：\n- " + strings.Join(problems, "\n- ") +
		"\n\n请按这个结构重新交（myco message reply --id <这条任务的 id> --body \"...\" --result result.json）：\n" + string(ex)
	thread := reply.ThreadID
	if err := f.send(st, body, thread); err != nil {
		f.failed(st, "交的结果不符合要求，退回也没发出去："+errText(err))
		return
	}
	st.Bounced = true
	f.touch(st)
	f.event(domain.EvBounced, st.StepID, "结果不符合，退回重交："+strings.Join(problems, "；"))
}

func (f *flow) stopTask(st *domain.RunStep) {
	if st.MessageID == "" {
		return
	}
	if m, err := f.s.msgRepo.Get(f.ctx, st.MessageID); err != nil || m.Status.Terminal() {
		return
	}
	if _, err := f.s.messages.StopFromConsole(f.ctx, f.run.Owner, true, st.MessageID, func(domain.AgentName) error { return nil }); err != nil {
		f.s.log.Warn("workflows: stopping a step's task failed", "run", f.run.ID, "task", st.MessageID, "err", err)
	}
}

// failed marks st failed: it is sent again while it has retries, else the
// run follows its error edges (or fails).
func (f *flow) failed(st *domain.RunStep, why string) {
	def := f.def.Step(st.StepID)
	f.touch(st)
	if st.Kind != domain.StepHuman && st.Retries < def.Retry && st.Input != "" && st.Target != "" {
		st.Retries++
		f.event(domain.EvRetry, st.StepID, fmt.Sprintf("失败了（%s），再发一次（第 %d 次重试）", why, st.Retries))
		if err := f.send(st, st.Input, ""); err == nil {
			st.Bounced = false
			if d := def.TimeoutDur(); d > 0 {
				st.DueAt = f.now().Add(d)
			}
			return
		}
	}
	st.Status, st.Error, st.EndedAt = domain.StepFailed, why, f.now()
	f.event(domain.EvFailed, st.StepID, why)
	f.after(st)
}

// finish marks st done and follows its edges.
func (f *flow) finish(st *domain.RunStep) {
	st.Status, st.EndedAt = domain.StepDone, f.now()
	f.touch(st)
	f.event(domain.EvDone, st.StepID, st.Summary)
	f.after(st)
}

func (f *flow) decided(st *domain.RunStep, choice, note, by string) {
	st.Output = map[string]any{"choice": choice, "note": note, "by": by}
	st.Summary = choice
	if note != "" {
		st.Summary += "：" + note
	}
	f.event(domain.EvDecided, st.StepID, by+" 选了「"+choice+"」")
	f.finish(st)
}

// after follows the edges out of a step that ended.
func (f *flow) after(st *domain.RunStep) {
	if domain.FlowOver(f.run.Status) {
		return
	}
	def := f.def.Step(st.StepID)
	var edges []domain.IndexedEdge
	all := f.def.Out(st.StepID)
	for _, e := range all {
		if e.Edge.OnError == (st.Status == domain.StepFailed) {
			edges = append(edges, e)
		}
	}
	if st.Status == domain.StepFailed && len(edges) == 0 {
		f.end(domain.FlowFailed, fmt.Sprintf("步骤 %s 失败了：%s", def.Label(), st.Error))
		return
	}
	env := f.env(st)
	taken := 0
	for _, e := range edges {
		if e.Edge.When != "" {
			ok, err := domain.EvalBool(e.Edge.When, env)
			if err != nil {
				f.end(domain.FlowFailed, fmt.Sprintf("%s → %s 的条件算不出来：%v", e.Edge.From, e.Edge.To, err))
				return
			}
			if !ok {
				continue
			}
		}
		taken++
		f.take(st, e)
		if def.Dispatch != "all" || domain.FlowOver(f.run.Status) {
			break
		}
	}
	if taken == 0 && len(edges) > 0 {
		f.end(domain.FlowFailed, fmt.Sprintf("步骤 %s 之后没有路可走：没有一条边的条件成立", def.Label()))
	}
}

// take follows one edge.
func (f *flow) take(st *domain.RunStep, ie domain.IndexedEdge) {
	e := ie.Edge
	to := e.To
	if e.MaxIterations > 0 {
		key := strconv.Itoa(ie.Index)
		if f.run.State.Edges == nil {
			f.run.State.Edges = map[string]int{}
		}
		if f.run.State.Edges[key] >= e.MaxIterations {
			to = e.Else
			if to == "" {
				to = domain.EdgeFail
			}
			f.event(domain.EvEdge, st.StepID, fmt.Sprintf("%s → %s 已经走了 %d 次，到上限了，改走 %s", e.From, e.To, e.MaxIterations, to))
			if to == domain.EdgeFail {
				f.end(domain.FlowFailed, fmt.Sprintf("%s → %s 到了最多 %d 次", e.From, e.To, e.MaxIterations))
				return
			}
		} else {
			f.run.State.Edges[key]++
		}
	}
	switch to {
	case domain.EdgeEnd:
		f.event(domain.EvEdge, st.StepID, st.StepID+" → 结束")
		return
	case domain.EdgeFail:
		f.end(domain.FlowFailed, fmt.Sprintf("从 %s 走到了「失败」", f.def.Step(st.StepID).Label()))
		return
	}
	f.event(domain.EvEdge, st.StepID, st.StepID+" → "+to)
	target := f.def.Step(to)
	if target.Join == "all" {
		if f.run.State.Joins == nil {
			f.run.State.Joins = map[string][]string{}
		}
		arrived := f.run.State.Joins[to]
		if !slices.Contains(arrived, st.StepID) {
			arrived = append(arrived, st.StepID)
		}
		for _, p := range f.def.Preds(to) {
			if !slices.Contains(arrived, p) {
				f.run.State.Joins[to] = arrived
				return
			}
		}
		delete(f.run.State.Joins, to)
	}
	f.activate(to)
}

// settle sets the run's state from its steps: done when nothing is open.
func (f *flow) settle() {
	if domain.FlowOver(f.run.Status) {
		return
	}
	running, waiting := 0, 0
	for _, st := range f.steps {
		switch st.Status {
		case domain.StepRunning:
			running++
		case domain.StepWaiting:
			waiting++
		}
	}
	switch {
	case running > 0:
		f.run.Status = domain.FlowRunning
	case waiting > 0:
		f.run.Status = domain.FlowWaiting
	default:
		for to, arrived := range f.run.State.Joins {
			var missing []string
			for _, p := range f.def.Preds(to) {
				if !slices.Contains(arrived, p) {
					missing = append(missing, p)
				}
			}
			f.end(domain.FlowFailed, fmt.Sprintf("%s 等不齐：%s 没有走到它", to, strings.Join(missing, "、")))
			return
		}
		f.end(domain.FlowSucceeded, "")
	}
}

// end finishes the run, closing whatever is still open.
func (f *flow) end(status, why string) {
	now := f.now()
	for _, st := range f.steps {
		if domain.StepOver(st.Status) {
			continue
		}
		if st.Status == domain.StepRunning {
			f.stopTask(st)
		}
		st.Status, st.EndedAt = domain.StepCancelled, now
		if st.Error == "" {
			st.Error = "运行结束了"
		}
		f.touch(st)
	}
	f.run.Status, f.run.Error, f.run.EndedAt = status, why, now
	text := map[string]string{domain.FlowSucceeded: "完成了", domain.FlowFailed: "失败了", domain.FlowCancelled: "取消了"}[status]
	if why != "" {
		text += "：" + why
	}
	f.event(domain.EvFinished, "", text)
	f.tellStarter(text)
}

// tellStarter sends the agent that started the run a note that it ended,
// with each step's summary, so it can go on with what it was doing.
func (f *flow) tellStarter(text string) {
	if f.quiet || f.run.Trigger != domain.FlowByAgent || f.run.By == "" {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[工作流「%s」%s]\n运行 %s\n", f.run.Name, text, f.run.ID)
	latest := map[string]*domain.RunStep{}
	var order []string
	for _, st := range f.steps {
		if _, seen := latest[st.StepID]; !seen {
			order = append(order, st.StepID)
		}
		latest[st.StepID] = st
	}
	for _, id := range order {
		st := latest[id]
		line := st.Summary
		if st.Status == domain.StepFailed || st.Status == domain.StepCancelled {
			line = st.Error
		}
		fmt.Fprintf(&b, "- %s（%s）：%s\n", f.def.Step(id).Label(), st.Target, line)
	}
	fmt.Fprintf(&b, "详情：myco workflow run show --id %s", f.run.ID)
	if _, err := f.s.messages.SendFromMember(f.ctx, f.run.Owner, f.run.OwnerID, SendInput{
		To: f.run.By, Type: string(domain.TypeNote), Body: b.String(), Workflow: f.run.ID,
	}); err != nil {
		f.s.log.Warn("workflows: telling the agent that started a run failed", "run", f.run.ID, "agent", f.run.By, "err", err)
	}
}

var jsonBlockRe = regexp.MustCompile("(?s)```json\\s*\\n(.*?)```")

// lastJSONBlock reads the last ```json block of a reply (nil: none).
func lastJSONBlock(body string) map[string]any {
	ms := jsonBlockRe.FindAllStringSubmatch(body, -1)
	for i := len(ms) - 1; i >= 0; i-- {
		var out map[string]any
		if json.Unmarshal([]byte(ms[i][1]), &out) == nil {
			return out
		}
	}
	return nil
}

func summaryOf(out map[string]any, body string) string {
	if s, ok := out["summary"].(string); ok && strings.TrimSpace(s) != "" {
		return firstLine(s, 200)
	}
	return firstLine(body, 200)
}

func firstLine(s string, n int) string {
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(strings.TrimLeft(l, "#*->` ")); l != "" {
			return clipRunes(l, n)
		}
	}
	return ""
}

func lastLine(s string, n int) string {
	ls := strings.Split(strings.TrimSpace(s), "\n")
	return clipRunes(strings.TrimSpace(ls[len(ls)-1]), n)
}

func clipRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
