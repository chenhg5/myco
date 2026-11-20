package domain

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"time"
)

// Usage is what an agent's run cost: tokens and money, as its CLI reports
// them (the last "usage" step of its trace).
type Usage struct {
	InputTokens  int64   `json:"input_tokens,omitempty"`
	OutputTokens int64   `json:"output_tokens,omitempty"`
	CostUSD      float64 `json:"cost_usd,omitempty"`
}

// Add adds u's counts to x.
func (x *Usage) Add(u *Usage) {
	if u == nil {
		return
	}
	x.InputTokens += u.InputTokens
	x.OutputTokens += u.OutputTokens
	x.CostUSD += u.CostUSD
}

// Zero reports whether nothing was counted.
func (x *Usage) Zero() bool {
	return x == nil || (x.InputTokens == 0 && x.OutputTokens == 0 && x.CostUSD == 0)
}

// RunMetrics are a run's runtime facts, worked out from its steps.
type RunMetrics struct {
	// DurationMS is the wall-clock time from start to end (or now).
	DurationMS int64
	// ActiveMS is the time some agent or computer step was at work;
	// WaitingMS the time a decision waited for someone (and nothing else
	// was at work).
	ActiveMS  int64
	WaitingMS int64
	// Steps counts step starts; Attempts adds the retries (and send-backs).
	Steps    int
	Done     int
	Failed   int
	Attempts int
	Retries  int
	Bounces  int
	Usage    Usage
}

// StepDurationMS is how long a step instance took (to now while open; 0
// when it has not started).
func StepDurationMS(s *RunStep, now time.Time) int64 {
	if s.StartedAt.IsZero() {
		return 0
	}
	end := s.EndedAt
	if end.IsZero() {
		if StepOver(s.Status) {
			return 0
		}
		end = now
	}
	if end.Before(s.StartedAt) {
		return 0
	}
	return end.Sub(s.StartedAt).Milliseconds()
}

// Attempts is how many times a step instance's task went out.
func (s *RunStep) Attempts() int {
	if n := len(s.Tasks); n > 0 {
		return n
	}
	if s.Kind == StepHuman {
		return 1
	}
	return 1 + s.Retries + s.BounceCount()
}

// BounceCount is how many times its result was sent back.
func (s *RunStep) BounceCount() int {
	if s.Bounces > 0 {
		return s.Bounces
	}
	if s.Bounced {
		return 1
	}
	return 0
}

// TaskIDs are the tasks a step instance sent, oldest first.
func (s *RunStep) TaskIDs() []string {
	if len(s.Tasks) > 0 {
		return s.Tasks
	}
	if s.MessageID != "" {
		return []string{s.MessageID}
	}
	return nil
}

// MetricsOf works out a run's metrics from its steps at now.
func MetricsOf(run *WorkflowRun, steps []*RunStep, now time.Time) RunMetrics {
	var m RunMetrics
	end := run.EndedAt
	if end.IsZero() {
		end = now
	}
	if !run.CreatedAt.IsZero() && end.After(run.CreatedAt) {
		m.DurationMS = end.Sub(run.CreatedAt).Milliseconds()
	}
	var work, wait []span
	for _, s := range steps {
		m.Steps++
		switch s.Status {
		case StepDone:
			m.Done++
		case StepFailed:
			m.Failed++
		}
		m.Retries += s.Retries
		m.Bounces += s.BounceCount()
		m.Attempts += s.Attempts()
		m.Usage.Add(s.Usage)
		d := StepDurationMS(s, end)
		if d <= 0 {
			continue
		}
		sp := span{s.StartedAt.UnixMilli(), s.StartedAt.UnixMilli() + d}
		if s.Kind == StepHuman {
			wait = append(wait, sp)
		} else {
			work = append(work, sp)
		}
	}
	m.ActiveMS = covered(work)
	// Waiting is a decision's time when nothing else was at work.
	m.WaitingMS = covered(append(append([]span{}, work...), wait...)) - m.ActiveMS
	return m
}

type span struct{ from, to int64 }

// covered is the time the spans cover together (overlaps counted once).
func covered(sp []span) int64 {
	if len(sp) == 0 {
		return 0
	}
	sort.Slice(sp, func(i, j int) bool { return sp[i].from < sp[j].from })
	var total int64
	cur := sp[0]
	for _, s := range sp[1:] {
		if s.from > cur.to {
			total += cur.to - cur.from
			cur = s
			continue
		}
		if s.to > cur.to {
			cur.to = s.to
		}
	}
	return total + cur.to - cur.from
}

// Dist summarizes values (durations in ms, scores).
type Dist struct {
	N   int     `json:"n"`
	Avg float64 `json:"avg"`
	Min float64 `json:"min"`
	P50 float64 `json:"p50"`
	P90 float64 `json:"p90"`
	P95 float64 `json:"p95"`
	Max float64 `json:"max"`
}

// DistOf summarizes xs (nearest-rank percentiles).
func DistOf(xs []float64) Dist {
	if len(xs) == 0 {
		return Dist{}
	}
	v := append([]float64(nil), xs...)
	sort.Float64s(v)
	sum := 0.0
	for _, x := range v {
		sum += x
	}
	rank := func(p float64) float64 {
		i := int(math.Ceil(p*float64(len(v)))) - 1
		if i < 0 {
			i = 0
		}
		return v[i]
	}
	return Dist{N: len(v), Avg: sum / float64(len(v)), Min: v[0], P50: rank(0.5), P90: rank(0.9), P95: rank(0.95), Max: v[len(v)-1]}
}

// RunEvaluation is a score given to a run (or one of its steps): by a
// person, a program or an agent, after the run or while it goes.
type RunEvaluation struct {
	ID    int64
	RunID string
	Step  string // a step id, or empty for the whole run
	Name  string // what is measured: quality, tests_passed, …
	Value float64
	Unit  string // optional: "%", "分", "个", …
	// Direction is whether more is better ("higher") or less ("lower");
	// empty: neither (a count).
	Direction string
	Note      string
	By        string
	At        time.Time
}

// Evaluation directions.
const (
	BetterHigher = "higher"
	BetterLower  = "lower"
)

var evalName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,63}$`)

// MaxRunEvaluations bounds the scores a run keeps.
const MaxRunEvaluations = 200

// Validate checks an evaluation before it is stored.
func (e *RunEvaluation) Validate() error {
	if !evalName.MatchString(e.Name) {
		return Errf(CodeInvalidArgument, "name %q: use letters, digits, _ . - (starting with a letter, at most 64)", e.Name).
			WithSuggestion("for example: --name quality")
	}
	if math.IsNaN(e.Value) || math.IsInf(e.Value, 0) {
		return Errf(CodeInvalidArgument, "the value must be a number")
	}
	switch e.Direction {
	case "", BetterHigher, BetterLower:
	default:
		return Errf(CodeInvalidArgument, "direction %q: use higher, lower or leave it out", e.Direction)
	}
	if len([]rune(e.Unit)) > 16 {
		return Errf(CodeInvalidArgument, "the unit is at most 16 characters")
	}
	if len([]rune(e.Note)) > 2000 {
		return Errf(CodeInvalidArgument, "the note is at most 2000 characters")
	}
	return nil
}

// Better says which of a and b is better for direction: "a", "b" or ""
// (equal, or no direction).
func Better(direction string, a, b float64) string {
	if a == b {
		return ""
	}
	switch direction {
	case BetterHigher:
		if a > b {
			return "a"
		}
		return "b"
	case BetterLower:
		if a < b {
			return "a"
		}
		return "b"
	}
	return ""
}

// HumanMS writes a duration for logs ("1m05s", "320ms").
func HumanMS(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	switch {
	case d < time.Second:
		return fmt.Sprintf("%dms", ms)
	case d < time.Minute:
		return fmt.Sprintf("%.1fs", d.Seconds())
	case d < time.Hour:
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
}
