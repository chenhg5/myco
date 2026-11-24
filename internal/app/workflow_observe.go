package app

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/domain"
)

// What a run did, cost, produced and scored: metrics worked out from its
// steps, the files its agents handed back, each step's log, scores, and a
// workflow's statistics over its recent runs.

// Artifact is a file an agent handed back on a step's task (with its
// answer, or with a progress report).
type Artifact struct {
	Step    string // the step's id
	StepRun string // the step instance (RunStep.ID)
	Try     int    // which of the step's tasks (1: the first)
	Kind    string // reply or progress
	Message string
	From    domain.AgentName
	At      time.Time
	File    domain.Attachment
}

// observe adds what a run produced and measured to its detail.
func (s *WorkflowService) observe(ctx context.Context, d *RunDetail) {
	s.fillUsage(ctx, d.Steps)
	d.Metrics = domain.MetricsOf(d.Run, d.Steps, s.clock.Now())
	d.Artifacts = s.artifacts(ctx, d.Steps)
	if ev, err := s.repo.Evaluations(ctx, []string{d.Run.ID}); err == nil {
		d.Evaluations = ev
	}
}

// fillUsage reads what each agent step's runs cost from their traces, and
// keeps it on steps that are over (traces are dropped after a week).
func (s *WorkflowService) fillUsage(ctx context.Context, steps []*domain.RunStep) {
	if s.traces == nil {
		return
	}
	var ids []string
	for _, st := range steps {
		if st.Kind == domain.StepAgent && (st.Usage == nil || !domain.StepOver(st.Status)) {
			ids = append(ids, st.TaskIDs()...)
		}
	}
	if len(ids) == 0 {
		return
	}
	last, err := s.traces.Last(ctx, ids, "usage")
	if err != nil {
		s.log.Warn("workflows: reading usage failed", "err", err)
		return
	}
	now := s.clock.Now()
	for _, st := range steps {
		if st.Kind != domain.StepAgent || (st.Usage != nil && domain.StepOver(st.Status)) {
			continue
		}
		var u domain.Usage
		found, all := 0, true
		for _, id := range st.TaskIDs() {
			e := last[id]
			if e == nil {
				all = false
				continue
			}
			var x domain.Usage
			if json.Unmarshal([]byte(e.Data), &x) == nil {
				u.Add(&x)
				found++
			}
		}
		if found == 0 {
			continue
		}
		st.Usage = &u
		// Over, and every task's run reported (or long enough ago that
		// the rest never will): keep it.
		if domain.StepOver(st.Status) && (all || now.Sub(st.EndedAt) > 10*time.Minute) {
			if err := s.repo.SaveStep(ctx, st); err != nil {
				s.log.Warn("workflows: keeping usage failed", "step", st.ID, "err", err)
			}
		}
	}
}

// threadOf lists the messages of the thread task id is in.
func (s *WorkflowService) threadOf(ctx context.Context, id string, seen map[string]bool) []*domain.Message {
	m, err := s.msgRepo.Get(ctx, id)
	if err != nil {
		return nil
	}
	t := m.ThreadID
	if t == "" {
		t = m.ID
	}
	if seen[t] {
		return nil
	}
	seen[t] = true
	ms, err := s.msgRepo.ListThread(ctx, t)
	if err != nil {
		return []*domain.Message{m}
	}
	return ms
}

// tryOf is which of st's tasks message m answers or reports on (1-based).
func tryOf(st *domain.RunStep, m *domain.Message) int {
	ids := st.TaskIDs()
	for i, id := range ids {
		if m.ID == id || m.ReplyTo == id {
			return i + 1
		}
	}
	return len(ids)
}

// artifacts lists the files the steps' agents handed back.
func (s *WorkflowService) artifacts(ctx context.Context, steps []*domain.RunStep) []Artifact {
	out := []Artifact{}
	for _, st := range steps {
		if st.Kind == domain.StepHuman || st.Target == "" {
			continue
		}
		seen, files := map[string]bool{}, map[string]bool{}
		for _, id := range st.TaskIDs() {
			for _, m := range s.threadOf(ctx, id, seen) {
				if string(m.From) != st.Target || len(m.Attachments) == 0 {
					continue
				}
				if m.Type != domain.TypeReply && m.Type != domain.TypeProgress {
					continue
				}
				for _, a := range m.Attachments {
					if files[a.FileID] {
						continue // shown in a progress report, then handed back
					}
					files[a.FileID] = true
					out = append(out, Artifact{Step: st.StepID, StepRun: st.ID, Try: tryOf(st, m), Kind: string(m.Type), Message: m.ID,
						From: m.From, At: m.CreatedAt, File: a})
				}
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out
}

// LogEntry is a line of a step's log.
type LogEntry struct {
	At time.Time
	// Kind: task (what was sent), progress, reply, failure, event (the
	// run's log), stdout, stderr (a computer step), trace (what the agent
	// did: Sub is text, thinking, tool, tool_result, error, usage, output).
	Kind        string
	Sub         string
	Try         int
	From        string
	Text        string
	Attachments []domain.Attachment
}

// StepLog is everything a step instance did, in time order.
type StepLog struct {
	Run     *domain.WorkflowRun
	Step    *domain.RunStep
	Entries []LogEntry
	// Clipped: the log was longer than MaxStepLog lines.
	Clipped bool
}

// MaxStepLog bounds a step log's lines.
const MaxStepLog = 3000

// StepLog returns step's log in run id: step is a step instance id, or a
// step id (its latest start). With trace it includes what the agent did.
func (s *WorkflowService) StepLog(ctx context.Context, c Caller, id, step string, trace bool) (*StepLog, error) {
	run, err := s.getRun(ctx, c, id)
	if err != nil {
		return nil, err
	}
	steps, err := s.repo.Steps(ctx, run.ID)
	if err != nil {
		return nil, err
	}
	var st *domain.RunStep
	for _, x := range steps {
		if x.ID == step {
			st = x
		}
	}
	if st == nil {
		for _, x := range steps {
			if x.StepID == step {
				st = x // the latest
			}
		}
	}
	if st == nil {
		return nil, domain.Errf(domain.CodeNotFound, "run %s has no step %s", id, step).
			WithSuggestion("see its steps with: myco workflow run show --id %s", id)
	}
	out := &StepLog{Run: run, Step: st}
	add := func(e LogEntry) { out.Entries = append(out.Entries, e) }
	text := func(body string) string {
		if domain.IsSealed(body) {
			return "（端到端加密，在 App 里看）"
		}
		return body
	}

	seen := map[string]bool{}
	for _, tid := range st.TaskIDs() {
		for _, m := range s.threadOf(ctx, tid, seen) {
			e := LogEntry{At: m.CreatedAt, Try: tryOf(st, m), From: string(m.From), Text: text(m.Body), Attachments: m.Attachments}
			switch {
			case m.Type == domain.TypeTask && slices.Contains(st.TaskIDs(), m.ID):
				e.Kind = "task"
			case m.Type == domain.TypeProgress:
				e.Kind = "progress"
			case m.Type == domain.TypeReply && st.Kind == domain.StepComputer:
				continue // its output is below
			case m.Type == domain.TypeReply:
				e.Kind = "reply"
				if m.Result != "" {
					e.Text = strings.TrimSpace(e.Text + "\n\n" + m.Result)
				}
			case m.Type == domain.TypeEvent:
				e.Kind = "failure"
			default:
				continue
			}
			add(e)
		}
		if trace && s.traces != nil && st.Kind == domain.StepAgent {
			evs, err := s.traces.List(ctx, tid, 0, 1000)
			if err == nil {
				try := 0
				for i, x := range st.TaskIDs() {
					if x == tid {
						try = i + 1
					}
				}
				for _, e := range evs {
					if le, ok := traceEntry(e); ok {
						le.Try = try
						add(le)
					}
				}
			}
		}
	}
	if st.Kind == domain.StepComputer && st.Output != nil {
		at := st.EndedAt
		if at.IsZero() {
			at = s.clock.Now()
		}
		for _, k := range []string{"stdout", "stderr"} {
			if v, _ := st.Output[k].(string); v != "" {
				add(LogEntry{At: at, Kind: k, Try: st.Attempts(), From: st.Target, Text: v})
			}
		}
		if code, ok := st.Output["exit_code"].(float64); ok {
			add(LogEntry{At: at, Kind: "event", Sub: "exit", Text: fmt.Sprintf("退出码 %d", int(code))})
		}
	}
	if events, err := s.repo.Events(ctx, run.ID); err == nil {
		end := st.EndedAt
		for _, e := range events {
			if e.Step != st.StepID || e.At.Before(st.StartedAt) || (!end.IsZero() && e.At.After(end.Add(time.Second))) {
				continue
			}
			add(LogEntry{At: e.At, Kind: "event", Sub: e.Kind, Text: e.Text})
		}
	}
	sort.SliceStable(out.Entries, func(i, j int) bool { return out.Entries[i].At.Before(out.Entries[j].At) })
	if len(out.Entries) > MaxStepLog {
		out.Entries, out.Clipped = out.Entries[len(out.Entries)-MaxStepLog:], true
	}
	return out, nil
}

// traceEntry turns a trace step into a log line (deltas, which the final
// text repeats, are left out).
func traceEntry(e *domain.TraceEvent) (LogEntry, bool) {
	var d struct {
		Kind         string  `json:"kind"`
		Stream       string  `json:"stream"`
		Text         string  `json:"text"`
		Tool         string  `json:"tool"`
		Input        string  `json:"input"`
		Output       string  `json:"output"`
		Failed       bool    `json:"failed"`
		InputTokens  int64   `json:"input_tokens"`
		OutputTokens int64   `json:"output_tokens"`
		CostUSD      float64 `json:"cost_usd"`
		DurationMS   int64   `json:"duration_ms"`
	}
	if json.Unmarshal([]byte(e.Data), &d) != nil {
		return LogEntry{}, false
	}
	if d.Kind == "" {
		d.Kind = e.Kind
	}
	le := LogEntry{At: e.At, Kind: "trace", Sub: d.Kind}
	switch d.Kind {
	case "text", "thinking", "error":
		le.Text = d.Text
	case "tool":
		le.Text = strings.TrimSpace(d.Tool + " " + clipRunes(d.Input, 2000))
	case "tool_result":
		le.Text = clipRunes(d.Output, 4000)
		if d.Failed {
			le.Sub = "tool_error"
		}
	case "output":
		le.Text, le.Sub = d.Text, "output:"+d.Stream
	case "usage":
		parts := []string{}
		if d.InputTokens+d.OutputTokens > 0 {
			parts = append(parts, fmt.Sprintf("输入 %d / 输出 %d tokens", d.InputTokens, d.OutputTokens))
		}
		if d.CostUSD > 0 {
			parts = append(parts, fmt.Sprintf("$%.4f", d.CostUSD))
		}
		if d.DurationMS > 0 {
			parts = append(parts, domain.HumanMS(d.DurationMS))
		}
		le.Text = strings.Join(parts, " · ")
	default:
		return LogEntry{}, false
	}
	if strings.TrimSpace(le.Text) == "" {
		return LogEntry{}, false
	}
	return le, true
}

// EvaluateInput is a score to give a run.
type EvaluateInput struct {
	Step      string
	Name      string
	Value     float64
	Unit      string
	Direction string
	Note      string
}

// Evaluate gives run id a score. Who may: those who may see the run, and
// the agents its steps went to (an agent step that checks the work).
func (s *WorkflowService) Evaluate(ctx context.Context, c Caller, id string, in EvaluateInput) (*domain.RunEvaluation, error) {
	run, err := s.repo.GetRun(ctx, id)
	if err != nil {
		return nil, err
	}
	steps, err := s.repo.Steps(ctx, run.ID)
	if err != nil {
		return nil, err
	}
	ok := mayRun(c, run)
	for _, st := range steps {
		if c.Agent != "" && st.Target == string(c.Agent) {
			ok = true
		}
	}
	if !ok {
		return nil, domain.Errf(domain.CodeNotFound, "workflow run %s not found", id)
	}
	if in.Step != "" {
		def, err := s.defOf(ctx, run)
		if err != nil {
			return nil, err
		}
		if def.Step(in.Step) == nil {
			return nil, domain.Errf(domain.CodeNotFound, "the workflow has no step %s", in.Step)
		}
	}
	e := &domain.RunEvaluation{RunID: run.ID, Step: in.Step, Name: strings.TrimSpace(in.Name), Value: in.Value, Unit: strings.TrimSpace(in.Unit),
		Direction: in.Direction, Note: strings.TrimSpace(in.Note), By: c.who(), At: s.clock.Now()}
	if err := e.Validate(); err != nil {
		return nil, err
	}
	if err := s.repo.AddEvaluation(ctx, e); err != nil {
		return nil, err
	}
	return e, nil
}

// StepStats are one step's figures over a workflow's recent runs.
type StepStats struct {
	Step     string
	Title    string
	Kind     string
	Runs     int // starts (a step in a loop counts each time)
	Done     int
	Failed   int
	Retries  int
	Bounces  int
	Duration domain.Dist // ms, of the starts that ended
	Usage    domain.Usage
	// The latest failure.
	LastError    string
	LastErrorRun string
	LastErrorAt  time.Time
}

// RunPoint is a run in the statistics' sample.
type RunPoint struct {
	ID         string
	Status     string
	Version    int
	CreatedAt  time.Time
	DurationMS int64
	Error      string
	FailedStep string
	Usage      domain.Usage
}

// VersionStats are the figures of one version of the workflow.
type VersionStats struct {
	Version   int
	Runs      int
	Succeeded int
	Failed    int
	Cancelled int
	Duration  domain.Dist
}

// EvalStats summarize a score over the runs that have it (each run's
// latest).
type EvalStats struct {
	Name      string
	Unit      string
	Direction string
	Values    domain.Dist
	Last      float64
	LastRun   string
}

// WorkflowStats are a workflow's figures: counts over all its runs, and
// distributions over its recent ones (Sample).
type WorkflowStats struct {
	Workflow string
	Name     string
	Total    int
	ByStatus map[string]int
	// SuccessRate is succeeded / ended, over all runs (0 without ended).
	SuccessRate float64
	Sample      int
	Version     int // only this version (0: all)
	Duration    domain.Dist
	Active      domain.Dist
	Waiting     domain.Dist
	Usage       domain.Usage // over the sample
	CostPerRun  float64
	Steps       []StepStats
	Versions    []VersionStats
	Evaluations []EvalStats
	Recent      []RunPoint // newest first (at most 60)
	Failures    []RunPoint // the latest failed runs (at most 10)
}

// DefaultStatsSample and MaxStatsSample bound the runs statistics read.
const (
	DefaultStatsSample = 200
	MaxStatsSample     = 2000
)

// Stats works out a workflow's figures over its last limit runs (of one
// version, when version is not 0).
func (s *WorkflowService) Stats(ctx context.Context, c Caller, ref string, limit, version int) (*WorkflowStats, error) {
	w, err := s.find(ctx, c, ref)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = DefaultStatsSample
	}
	if limit > MaxStatsSample {
		limit = MaxStatsSample
	}
	counts, err := s.repo.RunCounts(ctx, w.ID)
	if err != nil {
		return nil, err
	}
	st := &WorkflowStats{Workflow: w.ID, Name: w.Name, ByStatus: counts, Version: version,
		Steps: []StepStats{}, Versions: []VersionStats{}, Evaluations: []EvalStats{}, Recent: []RunPoint{}, Failures: []RunPoint{}}
	ended := 0
	for k, n := range counts {
		st.Total += n
		if domain.FlowOver(k) {
			ended += n
		}
	}
	if ended > 0 {
		st.SuccessRate = float64(counts[domain.FlowSucceeded]) / float64(ended)
	}
	read := limit
	if version > 0 {
		read = MaxStatsSample // then keep the version's
	}
	all, err := s.repo.ListRuns(ctx, domain.RunFilter{WorkflowID: w.ID, Limit: read})
	if err != nil {
		return nil, err
	}
	var runs []*domain.WorkflowRun
	for _, r := range all {
		if !mayRun(c, r) || (version > 0 && r.Version != version) {
			continue
		}
		runs = append(runs, r)
		if len(runs) == limit {
			break
		}
	}
	st.Sample = len(runs)
	if len(runs) == 0 {
		return st, nil
	}
	ids := make([]string, len(runs))
	for i, r := range runs {
		ids[i] = r.ID
	}
	stepsOf, err := s.repo.StepsOf(ctx, ids)
	if err != nil {
		return nil, err
	}
	var flat []*domain.RunStep
	for _, id := range ids {
		flat = append(flat, stepsOf[id]...)
	}
	s.fillUsage(ctx, flat)
	evals, err := s.repo.Evaluations(ctx, ids)
	if err != nil {
		return nil, err
	}

	now := s.clock.Now()
	var dur, act, wait []float64
	type stepAcc struct {
		StepStats
		d     []float64
		order int
	}
	accs := map[string]*stepAcc{}
	vers := map[int]*VersionStats{}
	verDur := map[int][]float64{}
	for _, r := range runs {
		steps := stepsOf[r.ID]
		m := domain.MetricsOf(r, steps, now)
		st.Usage.Add(&m.Usage)
		p := RunPoint{ID: r.ID, Status: r.Status, Version: r.Version, CreatedAt: r.CreatedAt, DurationMS: m.DurationMS, Error: r.Error, Usage: m.Usage}
		for _, x := range steps {
			if x.Status == domain.StepFailed {
				p.FailedStep = x.StepID
			}
		}
		if len(st.Recent) < 60 {
			st.Recent = append(st.Recent, p)
		}
		if r.Status == domain.FlowFailed && len(st.Failures) < 10 {
			st.Failures = append(st.Failures, p)
		}
		v := vers[r.Version]
		if v == nil {
			v = &VersionStats{Version: r.Version}
			vers[r.Version] = v
		}
		v.Runs++
		switch r.Status {
		case domain.FlowSucceeded:
			v.Succeeded++
		case domain.FlowFailed:
			v.Failed++
		case domain.FlowCancelled:
			v.Cancelled++
		}
		if domain.FlowOver(r.Status) {
			dur = append(dur, float64(m.DurationMS))
			act = append(act, float64(m.ActiveMS))
			wait = append(wait, float64(m.WaitingMS))
			verDur[r.Version] = append(verDur[r.Version], float64(m.DurationMS))
		}
		for _, x := range steps {
			a := accs[x.StepID]
			if a == nil {
				a = &stepAcc{StepStats: StepStats{Step: x.StepID, Kind: x.Kind}, order: len(accs)}
				accs[x.StepID] = a
			}
			a.Runs++
			a.Retries += x.Retries
			a.Bounces += x.BounceCount()
			a.Usage.Add(x.Usage)
			switch x.Status {
			case domain.StepDone:
				a.Done++
			case domain.StepFailed:
				a.Failed++
				if x.EndedAt.After(a.LastErrorAt) {
					a.LastError, a.LastErrorRun, a.LastErrorAt = x.Error, r.ID, x.EndedAt
				}
			}
			if domain.StepOver(x.Status) {
				if d := domain.StepDurationMS(x, now); d > 0 {
					a.d = append(a.d, float64(d))
				}
			}
		}
	}
	st.Duration, st.Active, st.Waiting = domain.DistOf(dur), domain.DistOf(act), domain.DistOf(wait)
	if st.Sample > 0 {
		st.CostPerRun = st.Usage.CostUSD / float64(st.Sample)
	}

	// Steps in the definition's order, then any the definition no longer has.
	pos := map[string]int{}
	for i, x := range w.Def.Steps {
		pos[x.ID] = i
	}
	list := make([]*stepAcc, 0, len(accs))
	for _, a := range accs {
		if d := w.Def.Step(a.Step); d != nil {
			a.Title = d.Title
		}
		a.Duration = domain.DistOf(a.d)
		list = append(list, a)
	}
	sort.Slice(list, func(i, j int) bool {
		pi, iok := pos[list[i].Step]
		pj, jok := pos[list[j].Step]
		if iok != jok {
			return iok
		}
		if iok {
			return pi < pj
		}
		return list[i].order < list[j].order
	})
	for _, a := range list {
		st.Steps = append(st.Steps, a.StepStats)
	}
	for _, v := range vers {
		v.Duration = domain.DistOf(verDur[v.Version])
		st.Versions = append(st.Versions, *v)
	}
	sort.Slice(st.Versions, func(i, j int) bool { return st.Versions[i].Version > st.Versions[j].Version })

	// Scores: each run's latest of a name.
	type evKey struct{ run, name string }
	latest := map[evKey]*domain.RunEvaluation{}
	for _, e := range evals {
		latest[evKey{e.RunID, e.Name}] = e // oldest first: the last wins
	}
	byName := map[string]*EvalStats{}
	vals := map[string][]float64{}
	lastAt := map[string]time.Time{}
	for _, e := range latest {
		x := byName[e.Name]
		if x == nil {
			x = &EvalStats{Name: e.Name}
			byName[e.Name] = x
		}
		vals[e.Name] = append(vals[e.Name], e.Value)
		if !e.At.Before(lastAt[e.Name]) {
			lastAt[e.Name] = e.At
			x.Last, x.LastRun, x.Unit, x.Direction = e.Value, e.RunID, e.Unit, e.Direction
		}
	}
	for name, x := range byName {
		x.Values = domain.DistOf(vals[name])
		st.Evaluations = append(st.Evaluations, *x)
	}
	sort.Slice(st.Evaluations, func(i, j int) bool { return st.Evaluations[i].Name < st.Evaluations[j].Name })
	return st, nil
}

// RunSide is one run in a comparison.
type RunSide struct {
	Run     *domain.WorkflowRun
	Metrics domain.RunMetrics
}

// StepDelta compares a step in two runs (over all its starts in each).
type StepDelta struct {
	Step      string
	Title     string
	A, B      *StepSide // nil: it did not run there
	FasterIn  string    // "a", "b" or ""
	DeltaMS   int64     // B − A
	DeltaCost float64   // B − A
}

// StepSide is a step's figures in one run.
type StepSide struct {
	Status     string // of its latest start
	Runs       int
	Attempts   int
	DurationMS int64
	Usage      domain.Usage
	Summary    string
	Error      string
}

// EvalDelta compares a score in two runs (each one's latest).
type EvalDelta struct {
	Name      string
	Unit      string
	Direction string
	A, B      *float64
	Delta     float64 // B − A, when both have it
	Better    string  // "a", "b" or ""
}

// RunComparison sets two runs side by side.
type RunComparison struct {
	A, B        RunSide
	SameVersion bool
	Steps       []StepDelta
	Evaluations []EvalDelta
}

// Compare sets runs a and b side by side: time, attempts, cost per step,
// and their scores.
func (s *WorkflowService) Compare(ctx context.Context, c Caller, a, b string) (*RunComparison, error) {
	da, err := s.GetRun(ctx, c, a)
	if err != nil {
		return nil, err
	}
	db, err := s.GetRun(ctx, c, b)
	if err != nil {
		return nil, err
	}
	out := &RunComparison{A: RunSide{da.Run, da.Metrics}, B: RunSide{db.Run, db.Metrics},
		SameVersion: da.Run.WorkflowID == db.Run.WorkflowID && da.Run.Version == db.Run.Version,
		Steps:       []StepDelta{}, Evaluations: []EvalDelta{}}
	now := s.clock.Now()
	side := func(d *RunDetail, id string) *StepSide {
		var x *StepSide
		for _, st := range d.Steps {
			if st.StepID != id {
				continue
			}
			if x == nil {
				x = &StepSide{}
			}
			x.Runs++
			x.Attempts += st.Attempts()
			x.DurationMS += domain.StepDurationMS(st, now)
			x.Usage.Add(st.Usage)
			x.Status, x.Summary, x.Error = st.Status, st.Summary, st.Error
		}
		return x
	}
	var order []string
	seen := map[string]bool{}
	for _, d := range []*RunDetail{da, db} {
		for _, x := range d.Def.Steps {
			if !seen[x.ID] {
				seen[x.ID] = true
				order = append(order, x.ID)
			}
		}
	}
	for _, id := range order {
		sa, sb := side(da, id), side(db, id)
		if sa == nil && sb == nil {
			continue
		}
		d := StepDelta{Step: id, A: sa, B: sb}
		if x := db.Def.Step(id); x != nil {
			d.Title = x.Title
		} else if x := da.Def.Step(id); x != nil {
			d.Title = x.Title
		}
		if sa != nil && sb != nil {
			d.DeltaMS = sb.DurationMS - sa.DurationMS
			d.DeltaCost = sb.Usage.CostUSD - sa.Usage.CostUSD
			d.FasterIn = domain.Better(domain.BetterLower, float64(sa.DurationMS), float64(sb.DurationMS))
		}
		out.Steps = append(out.Steps, d)
	}
	latest := func(evs []*domain.RunEvaluation) map[string]*domain.RunEvaluation {
		m := map[string]*domain.RunEvaluation{}
		for _, e := range evs {
			m[e.Name] = e
		}
		return m
	}
	ea, eb := latest(da.Evaluations), latest(db.Evaluations)
	var names []string
	for n := range ea {
		names = append(names, n)
	}
	for n := range eb {
		if ea[n] == nil {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	for _, n := range names {
		d := EvalDelta{Name: n}
		for _, e := range []*domain.RunEvaluation{ea[n], eb[n]} {
			if e != nil {
				d.Unit, d.Direction = e.Unit, e.Direction
			}
		}
		if e := ea[n]; e != nil {
			v := e.Value
			d.A = &v
		}
		if e := eb[n]; e != nil {
			v := e.Value
			d.B = &v
		}
		if d.A != nil && d.B != nil {
			d.Delta = *d.B - *d.A
			d.Better = domain.Better(d.Direction, *d.A, *d.B)
		}
		out.Evaluations = append(out.Evaluations, d)
	}
	return out, nil
}
