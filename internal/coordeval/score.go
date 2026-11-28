package coordeval

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/domain"
	"github.com/myco-ai/myco/internal/infra/sqlite"
)

// Result is a scenario's score and what it is made of (coordinator-tools,
// §7): the result right (else 0), the person's constraint kept (else 0),
// then deductions for time over the critical path, needless questions to
// the person, waste, and how late trouble was noticed.
type Result struct {
	Scenario string `json:"scenario"`
	Title    string `json:"title"`
	// Passed: the result is right and nothing vetoed it.
	Passed bool `json:"passed"`
	Score  int  `json:"score"`
	// Correct is the checker's verdict on the result; Veto, when set, is
	// the constraint broken (the score is 0).
	Correct bool   `json:"correct"`
	Veto    string `json:"veto,omitempty"`
	// Elapsed and Ideal are in units: from the goal to its answer, and the
	// critical path. Ratio is Elapsed / Ideal.
	Elapsed float64 `json:"elapsed_units"`
	Ideal   float64 `json:"ideal_units"`
	Ratio   float64 `json:"time_ratio"`
	// Interruptions are questions put to the person that were not theirs
	// to decide (the drill has none).
	Interruptions int `json:"interruptions"`
	// Waste: the same piece of work given out again, stopped, or done
	// twice, beyond what the scenario calls for.
	Waste int `json:"waste"`
	// Detect is how long (units) it took to notice the trouble (-1:
	// never), for scenarios with one.
	Detect float64 `json:"detect_units,omitempty"`
	// Deductions and Notes say why.
	Deductions []string `json:"deductions,omitempty"`
	Notes      []string `json:"notes,omitempty"`

	failed   string
	deducted int
}

func (res *Result) fail(why string) {
	if res.failed == "" {
		res.failed = why
	}
}

func (res *Result) veto(why string) {
	if res.Veto == "" {
		res.Veto = why
	}
}

func (res *Result) penalty(n int, why string) {
	if n <= 0 {
		return
	}
	res.deducted += n
	res.Deductions = append(res.Deductions, fmt.Sprintf("-%d %s", n, why))
}

func (res *Result) note(s string) { res.Notes = append(res.Notes, s) }

// expected extra hand-outs, stops and repeats per scenario (the work that
// is part of doing it right).
var allowedWaste = map[string]int{"offline-midway": 1, "wrong-result": 1, "priority": 2}

// score works out the result of a finished run.
func score(r *Run) Result {
	sc := r.Scenario
	res := Result{Scenario: sc.Name, Title: sc.Title, Ideal: sc.Ideal}
	sc.check(r, &res)
	res.Correct = res.failed == ""
	if !res.Correct {
		res.Deductions = append([]string{"result: " + res.failed}, res.Deductions...)
	}
	// time over the critical path
	if res.Ideal > 0 && res.Elapsed > 0 {
		res.Ratio = math.Round(res.Elapsed/res.Ideal*100) / 100
		if res.Ratio > 1.5 {
			res.penalty(min(30, int((res.Ratio-1.5)*10)), fmt.Sprintf("took %.1f× the critical path", res.Ratio))
		}
	}
	// questions to the person
	res.Interruptions = r.interruptions()
	if res.Interruptions > 0 {
		res.penalty(min(20, 10*res.Interruptions), fmt.Sprintf("asked the person %d time(s) with nothing for them to decide", res.Interruptions))
	}
	// waste
	res.Waste = max(0, r.waste()-allowedWaste[sc.Name])
	if res.Waste > 0 {
		res.penalty(min(20, 5*res.Waste), fmt.Sprintf("%d piece(s) of work given out again, stopped or done twice", res.Waste))
	}
	// noticing trouble
	if sc.Threshold > 0 {
		switch {
		case res.Detect < 0:
			res.penalty(20, "never noticed")
		case res.Detect > sc.Threshold:
			res.penalty(min(20, int(20*(res.Detect-sc.Threshold)/(2*sc.Threshold))+1), fmt.Sprintf("noticed after %.1f units (threshold %.0f)", res.Detect, sc.Threshold))
		}
	}
	res.Passed = res.Correct && res.Veto == ""
	if res.Passed {
		res.Score = max(0, 100-res.deducted)
	}
	if res.Veto != "" {
		res.Deductions = append([]string{"veto: " + res.Veto}, res.Deductions...)
	}
	return res
}

// interruptions are the coordinator's questions to the person: progress
// updates on their goals that ask (and tasks to them, should agents ever
// be able to send them one).
func (r *Run) interruptions() int {
	n := 0
	for _, m := range r.Messages {
		if string(m.From) != Lead || string(m.To) != User {
			continue
		}
		if m.Type == domain.TypeTask || (m.Type == domain.TypeProgress && m.Ask) {
			n++
		}
	}
	return n
}

// waste: hand-outs of a piece of work beyond the first, stopped tasks,
// and pieces done more than once.
func (r *Run) waste() int {
	given := map[string]int{}
	for _, b := range r.briefs() {
		if b.Type != domain.TypeTask {
			continue
		}
		for _, n := range r.itemsIn(b.Body) {
			given[n]++
		}
	}
	n := 0
	for _, c := range given {
		n += max(0, c-1)
	}
	for _, e := range r.Events {
		if e.Kind == EvStopped {
			n++
		}
	}
	return n
}

// ---- running

// Coordinator is what a scenario tests: started once the world is ready
// (the goal is sent after), stopped at the deadline.
type Coordinator interface {
	Name() string
	Start(ctx context.Context, w *World) (stop func(), err error)
}

// RunOptions configure a run of scenarios.
type RunOptions struct {
	Options
	// Keep keeps each scenario's directory (the hub's database, the CLI's
	// files) for a look afterwards.
	Keep bool
}

// RunScenario runs one scenario against a coordinator and scores it.
func RunScenario(ctx context.Context, sc *Scenario, coord Coordinator, opts RunOptions) (Result, *Run, error) {
	dir, err := os.MkdirTemp(opts.Dir, "coordeval-"+sc.Name+"-")
	if err != nil {
		return Result{}, nil, err
	}
	if !opts.Keep {
		defer os.RemoveAll(dir)
	}
	o := opts.Options
	switch coord.(type) {
	case Baseline, *Baseline:
		o.clock = newScenarioClock()
		ctx = context.WithValue(ctx, scenarioClockKey{}, o.clock)
		var done func()
		ctx, done = scenarioActorContext(ctx)
		defer done()
	}
	o.Dir = dir
	w, err := NewWorld(ctx, o)
	if err != nil {
		return Result{}, nil, err
	}
	defer w.Close()
	if err := sc.setup(ctx, w); err != nil {
		return Result{}, nil, fmt.Errorf("%s: setting up: %w", sc.Name, err)
	}
	if err := w.preload(ctx); err != nil {
		return Result{}, nil, fmt.Errorf("%s: other work: %w", sc.Name, err)
	}
	if err := w.SetupProject(ctx, sc.Title+": "+sc.Expect); err != nil {
		return Result{}, nil, fmt.Errorf("%s: project: %w", sc.Name, err)
	}
	if sc.Threshold > 0 {
		// the hub's alerts, when it has them, at the scenario's pace
		if err := w.SetAlertAfter(ctx, w.Units(sc.Threshold/2)); err != nil {
			return Result{}, nil, fmt.Errorf("%s: alerts: %w", sc.Name, err)
		}
	}
	fakeNames := map[string]bool{}
	before := map[string]bool{}
	if agents, err := w.Agents(ctx); err == nil {
		for _, a := range agents {
			before[a.Name] = true
		}
	}
	for _, f := range w.fakes {
		fakeNames[f.Name] = true
	}
	w.Run(ctx)
	stop, err := coord.Start(ctx, w)
	if err != nil {
		return Result{}, nil, fmt.Errorf("%s: starting %s: %w", sc.Name, coord.Name(), err)
	}
	start := w.now()
	deadline := start.Add(w.Units(sc.Deadline))
	pending := append([]goalSpec(nil), sc.goals...)
	for ctx.Err() == nil && w.now().Before(deadline) {
		for len(pending) > 0 && w.now().Sub(start) >= w.Units(pending[0].At) && w.briefed(ctx, pending[0].After) {
			if _, err := w.SendGoal(ctx, pending[0].Body, pending[0].Priority); err != nil {
				stop()
				return Result{}, nil, fmt.Errorf("%s: sending the goal: %w", sc.Name, err)
			}
			pending = pending[1:]
		}
		if len(pending) == 0 && allAnswered(ctx, w) {
			break
		}
		sleepCtx(ctx, min(w.Unit()/2, 250*time.Millisecond))
	}
	stop()
	w.Stop()
	r := &Run{Scenario: sc, World: w, Start: start, End: w.now(), Goals: w.Goals(), Events: w.Events(), fakes: fakeNames, Forks: map[string]string{}}
	if r.Messages, err = w.Messages(ctx); err != nil {
		return Result{}, nil, err
	}
	// agents that appeared during the run as copies of a fake one
	if agents, err := w.Agents(ctx); err == nil {
		for _, a := range agents {
			if before[a.Name] {
				continue
			}
			for _, l := range a.Labels {
				if from, ok := strings.CutPrefix(l, "fork-of="); ok && fakeNames[from] {
					r.Forks[a.Name] = from
				}
			}
		}
	}
	return score(r), r, nil
}

// briefed reports whether the coordinator has given out every one of
// these items (to anyone).
func (w *World) briefed(ctx context.Context, items []string) bool {
	if len(items) == 0 {
		return true
	}
	db, err := sqlite.Open(w.db)
	if err != nil {
		return false
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT body FROM messages WHERE from_agent=? AND type='task'`, Lead)
	if err != nil {
		return false
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var body string
		if rows.Scan(&body) != nil {
			continue
		}
		lower := strings.ToLower(body)
		for _, it := range items {
			if strings.Contains(lower, strings.ToLower(it)) {
				seen[it] = true
			}
		}
	}
	return len(seen) == len(items)
}

func allAnswered(ctx context.Context, w *World) bool {
	goals := w.Goals()
	if len(goals) == 0 {
		return false
	}
	for _, g := range goals {
		m, err := w.goal(ctx, g.ID)
		if err != nil || m.Status != "done" {
			return false
		}
	}
	return true
}

// Report is a run of all (or some) scenarios.
type Report struct {
	Coordinator string    `json:"coordinator"`
	When        time.Time `json:"when"`
	Unit        string    `json:"unit"`
	Seed        int64     `json:"seed"`
	Passed      int       `json:"passed"`
	Total       int       `json:"total"`
	Mean        float64   `json:"mean_score"`
	Results     []Result  `json:"results"`
}

// RunAll runs scenarios (all when none are named) one after another.
func RunAll(ctx context.Context, coord Coordinator, opts RunOptions, names ...string) (*Report, error) {
	rep := &Report{Coordinator: coord.Name(), When: time.Now().UTC(), Unit: opts.Unit.String(), Seed: opts.Seed}
	for _, sc := range Scenarios() {
		if len(names) > 0 && !contains(names, sc.Name) {
			continue
		}
		res, _, err := RunScenario(ctx, sc, coord, opts)
		if err != nil {
			return rep, err
		}
		rep.Results = append(rep.Results, res)
	}
	sum := 0
	for _, r := range rep.Results {
		sum += r.Score
		if r.Passed {
			rep.Passed++
		}
	}
	rep.Total = len(rep.Results)
	if rep.Total > 0 {
		rep.Mean = math.Round(float64(sum)/float64(rep.Total)*10) / 10
	}
	return rep, nil
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

// Table is the report as a myco-ui Table block (docs/rich-messages.md).
func (rep *Report) Table() string {
	rows := [][]string{}
	for _, r := range rep.Results {
		pass := "✗"
		if r.Passed {
			pass = "✓"
		}
		why := strings.Join(r.Deductions, "; ")
		if why == "" {
			why = strings.Join(r.Notes, "; ")
		}
		rows = append(rows, []string{r.Title, pass, fmt.Sprint(r.Score), fmt.Sprintf("%.1f", r.Ratio), fmt.Sprint(r.Interruptions), fmt.Sprint(r.Waste), why})
	}
	block := map[string]any{"type": "Table", "props": map[string]any{
		"columns": []string{"Scenario", "Passed", "Score", "Time ÷ critical path", "Questions", "Waste", "Why"},
		"rows":    rows,
		"align":   []string{"left", "center", "right", "right", "right", "right", "left"},
	}}
	b, _ := json.Marshal(block)
	return fmt.Sprintf("Coordination test · %s · %d/%d passed · mean %.1f\n\n```myco-ui\n%s\n```\n", rep.Coordinator, rep.Passed, rep.Total, rep.Mean, b)
}

// Write saves the report as JSON (path) and its table next to it (.md).
func (rep *Report) Write(path string) error {
	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.WriteFile(strings.TrimSuffix(path, filepath.Ext(path))+".md", []byte(rep.Table()), 0o644)
}
