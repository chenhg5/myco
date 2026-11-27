package coordeval

import (
	"context"
	"strings"
	"testing"
	"time"
)

func testOpts(t *testing.T) RunOptions {
	return RunOptions{Options: Options{Dir: t.TempDir(), Unit: 150 * time.Millisecond, Seed: 7}}
}

// The scripted baseline does what each scenario expects: every one passes.
func TestTheBaselinePassesEveryScenario(t *testing.T) {
	for _, sc := range Scenarios() {
		t.Run(sc.Name, func(t *testing.T) {
			t.Parallel() // separate worlds
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			res, _, err := RunScenario(ctx, sc, Baseline{}, testOpts(t))
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("%s: score %d ratio %.2f waste %d detect %.1f %v %v", sc.Name, res.Score, res.Ratio, res.Waste, res.Detect, res.Deductions, res.Notes)
			// right, nothing wasted, nobody bothered (time depends on the
			// machine's load: it only deducts)
			if !res.Passed || res.Waste != 0 || res.Interruptions != 0 || res.Score < 40 {
				t.Fatalf("baseline on %s: %+v", sc.Name, res)
			}
		})
	}
}

// A naive coordinator (everything to the first agent, the goal's items only,
// answers passed on unchecked) is caught where each scenario looks.
func TestTheCheckersCatchANaiveCoordinator(t *testing.T) {
	want := map[string]func(Result) bool{
		"parallel-five":  func(r Result) bool { return r.Score <= 70 && hasDeduction(r, "nothing ran in parallel") },
		"expert-busy":    func(r Result) bool { return hasDeduction(r, "without saying why") },
		"offline-midway": func(r Result) bool { return !r.Passed && r.Score == 0 },
		"wrong-result":   func(r Result) bool { return !r.Passed && hasDeduction(r, "wrong total") },
		"priority":       func(r Result) bool { return hasDeduction(r, "low-priority job was finished before") || !r.Passed },
		"constraint":     func(r Result) bool { return r.Veto != "" && r.Score == 0 },
	}
	for _, sc := range Scenarios() {
		check, ok := want[sc.Name]
		if !ok {
			continue
		}
		t.Run(sc.Name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			res, _, err := RunScenario(ctx, sc, Baseline{Naive: true}, testOpts(t))
			if err != nil {
				t.Fatal(err)
			}
			if !check(res) {
				t.Fatalf("naive on %s not caught: %+v", sc.Name, res)
			}
		})
	}
}

func hasDeduction(r Result, part string) bool {
	for _, d := range r.Deductions {
		if strings.Contains(d, part) {
			return true
		}
	}
	return false
}

// asker answers by itself, after asking the person something they have no
// call on and waiting for their answer.
type asker struct{}

func (asker) Name() string { return "asker" }

func (asker) Start(ctx context.Context, w *World) (func(), error) {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		c := w.CoordClient()
		for ctx.Err() == nil {
			msgs, err := c.Pull(ctx, 1, w.Unit())
			if err != nil || len(msgs) == 0 || msgs[0].Type != "task" {
				continue
			}
			g := msgs[0]
			_ = c.Ack(ctx, g.ID)
			if _, _, err := c.Ask(ctx, g.ID, "Shall I add them up?\n1. Yes\n2. No", nil, false); err != nil {
				return
			}
			for ctx.Err() == nil {
				if steers, err := c.Steers(ctx, g.ID, true); err == nil && len(steers) > 0 {
					break
				}
				sleepCtx(ctx, w.Unit())
			}
			var lines []string
			for _, it := range w.Items() {
				lines = append(lines, it.Name+": "+it.Answer())
			}
			_, _ = c.Reply(ctx, g.ID, strings.Join(lines, "\n"), false)
		}
	}()
	return func() { cancel(); <-done }, nil
}

// A question to the person is answered (the run goes on) and counted.
func TestAQuestionToThePersonIsAnsweredAndCounted(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	res, _, err := RunScenario(ctx, ScenarioByName("parallel-five"), asker{}, testOpts(t))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Correct || res.Interruptions != 1 || !hasDeduction(res, "asked the person 1 time") {
		t.Fatalf("%+v", res)
	}
}
