package coordeval

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/domain"
)

// Scenario is one fixed situation, with what is expected of the
// coordinator and how a run of it is checked (coordinator-tools, §7).
type Scenario struct {
	Name   string `json:"name"`
	Title  string `json:"title"`
	Expect string `json:"expect"`
	// Ideal is the critical path, in units: how long the goal takes with
	// everything that can run in parallel running in parallel, nobody
	// queued. Deadline is when the run stops, in units.
	Ideal    float64 `json:"ideal_units"`
	Deadline float64 `json:"deadline_units"`
	// Threshold, for a scenario with something going wrong, is how soon
	// (units) the coordinator should notice.
	Threshold float64 `json:"threshold_units,omitempty"`

	setup func(ctx context.Context, w *World) error
	goals []goalSpec
	check func(r *Run, res *Result)
}

type goalSpec struct {
	At       float64 // units after the start
	Priority string
	Body     string
	// After, when set, holds the goal back until the coordinator has given
	// out these items (the work it is to interrupt is under way).
	After []string
}

// Run is a finished run of a scenario: what the hub and the fake agents
// recorded.
type Run struct {
	Scenario *Scenario
	World    *World
	Start    time.Time
	End      time.Time
	Goals    []Goal
	Messages []*domain.Message
	Events   []Event
	fakes    map[string]bool
	// Forks are agents made during the run from one of the fake agents
	// (myco agent fork, when the CLI has it): name -> the agent copied.
	Forks map[string]string
}

// Scenarios are the first seven (coordinator-tools, §7).
func Scenarios() []*Scenario {
	return []*Scenario{parallelFive(), expertBusy(), offlineMidway(), wrongResult(), collision(), priority(), constraint()}
}

// ScenarioByName finds one ("" for none).
func ScenarioByName(name string) *Scenario {
	for _, s := range Scenarios() {
		if s.Name == name {
			return s
		}
	}
	return nil
}

func fakes(ctx context.Context, w *World, specs ...FakeSpec) error {
	for _, s := range specs {
		if _, err := w.AddFake(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

func lists(skill string, units float64, spec map[string][]int, order ...string) []Item {
	var out []Item
	for _, n := range order {
		out = append(out, Item{Name: n, Numbers: spec[n], Units: units, Skill: skill})
	}
	return out
}

func listLines(items []Item) string {
	var b strings.Builder
	for _, it := range items {
		var nums []string
		for _, n := range it.Numbers {
			nums = append(nums, fmt.Sprint(n))
		}
		fmt.Fprintf(&b, "- %s: %s\n", it.Name, strings.Join(nums, ", "))
	}
	return b.String()
}

// 1. Five pieces of work that do not depend on each other, three idle
// agents that can do them: they go out in parallel.
func parallelFive() *Scenario {
	items := lists("sums", 4, map[string][]int{
		"amber": {12, 7, 30, 5}, "birch": {8, 21, 3}, "cedar": {44, 9, 16, 2}, "dune": {5, 5, 50}, "ember": {13, 27, 6, 1},
	}, "amber", "birch", "cedar", "dune", "ember")
	return &Scenario{
		Name: "parallel-five", Title: "Five in parallel",
		Expect: "Sent out in parallel to the three sums agents, not all to one",
		Ideal:  8, Deadline: 90,
		setup: func(ctx context.Context, w *World) error {
			w.AddItems(items...)
			return fakes(ctx, w,
				FakeSpec{Name: "counter-a", Skill: "sums", Jitter: 0.1},
				FakeSpec{Name: "counter-b", Skill: "sums", Jitter: 0.1},
				FakeSpec{Name: "counter-c", Skill: "sums", Jitter: 0.1})
		},
		goals: []goalSpec{{Body: "I need the total of each of these five lists of numbers. They are separate pieces of work; the team's sums agents can add them up.\n\n" +
			listLines(items) + "\nReply with each list's total."}},
		check: func(r *Run, res *Result) {
			r.wantAnswers(res, r.Goals[0], items)
			if got := r.agentsGiven(items); got < 2 {
				res.penalty(30, "all the lists went to one agent: nothing ran in parallel")
			}
		},
	}
}

// 2. The only agent that can do it has three tasks queued; it may be
// copied (forked) when the CLI can: fork, or say why it waits.
func expertBusy() *Scenario {
	it := Item{Name: "harbor-map", Token: "MAP-7Q2", Units: 6, Skill: "diagrams"}
	queued := []Item{{Name: "old-chart-1", Token: "OC1", Units: 5, Skill: "diagrams"}, {Name: "old-chart-2", Token: "OC2", Units: 5, Skill: "diagrams"}, {Name: "old-chart-3", Token: "OC3", Units: 5, Skill: "diagrams"}}
	return &Scenario{
		Name: "expert-busy", Title: "The expert is busy",
		Expect: "Forks the busy expert, or tells the person why the work waits; does not just queue it",
		Ideal:  6, Deadline: 120,
		setup: func(ctx context.Context, w *World) error {
			w.AddItems(append(queued, it)...)
			return fakes(ctx, w,
				FakeSpec{Name: "drafter", Skill: "diagrams", Queue: []string{"old-chart-1", "old-chart-2", "old-chart-3"}},
				FakeSpec{Name: "counter-a", Skill: "sums"})
		},
		goals: []goalSpec{{Body: "Please have the harbor-map diagram drawn (the team's diagrams agent does these), and send me its result code.\n\n- harbor-map: the map of the harbor, as a diagram"}},
		check: func(r *Run, res *Result) {
			r.wantAnswers(res, r.Goals[0], []Item{it})
			forked := len(r.Forks) > 0
			told := r.toldPersonWhy(r.Goals[0], `(?i)queue|queued|busy|wait|behind|排队|在忙|等`)
			switch {
			case forked:
				res.note("forked the busy agent")
			case told:
				res.note("told the person why it waits")
				res.Ideal = 21 // waiting behind the queue, said so
			default:
				res.penalty(40, "queued behind the busy agent without saying why")
			}
		},
	}
}

// 3. An agent goes away halfway through: noticed in time, given to another.
func offlineMidway() *Scenario {
	items := []Item{{Name: "north-summary", Token: "NS-41", Units: 6, Skill: "writing"}, {Name: "south-summary", Token: "SS-58", Units: 6, Skill: "writing"}}
	return &Scenario{
		Name: "offline-midway", Title: "Offline halfway",
		Expect: "Notices within the threshold that an agent went away, and gives its work to another",
		// the work, plus the part lost and noticing it
		Ideal: 12, Deadline: 90, Threshold: 8,
		setup: func(ctx context.Context, w *World) error {
			w.AddItems(items...)
			return fakes(ctx, w,
				FakeSpec{Name: "writer-a", Skill: "writing", GoesOfflineAt: 0.4},
				FakeSpec{Name: "writer-b", Skill: "writing"},
				FakeSpec{Name: "writer-c", Skill: "writing"})
		},
		goals: []goalSpec{{Body: "Please get two short summaries written by the team's writing agents and send me their result codes.\n\n- north-summary: the north office's week\n- south-summary: the south office's week"}},
		check: func(r *Run, res *Result) {
			r.wantAnswers(res, r.Goals[0], items)
			off, ok := r.firstEvent(EvOffline)
			if !ok {
				res.note("no agent went away this run")
				return
			}
			again, found := r.briefAfter(off.Item, off.At, off.Agent)
			if !found {
				res.note("its work was never given to another agent")
				res.Detect = -1
				return
			}
			res.Detect = r.units(again.Sub(off.At))
		},
	}
}

// 4. A plausible but wrong result: checked before it reaches the person.
func wrongResult() *Scenario {
	items := lists("sums", 4, map[string][]int{"fig": {3, 14, 15, 9}, "gale": {26, 5, 35, 8}, "hazel": {97, 9, 32}, "iris": {38, 4, 62, 6}}, "fig", "gale", "hazel", "iris")
	items[1].WrongFirst = 10
	return &Scenario{
		Name: "wrong-result", Title: "A wrong result",
		Expect: "Checks results before answering; a wrong total is redone, never passed on",
		Ideal:  12, Deadline: 90,
		setup: func(ctx context.Context, w *World) error {
			w.AddItems(items...)
			return fakes(ctx, w,
				FakeSpec{Name: "counter-a", Skill: "sums"}, FakeSpec{Name: "counter-b", Skill: "sums"}, FakeSpec{Name: "counter-c", Skill: "sums"})
		},
		goals: []goalSpec{{Body: "Please have the team's sums agents add up these four lists, and send me the totals. I will copy them into the accounts as they are.\n\n" + listLines(items)}},
		check: func(r *Run, res *Result) {
			r.wantAnswers(res, r.Goals[0], items)
		},
	}
}

// 5. Two changes to the same file: one after the other, or apart and
// merged; never both at once.
func collision() *Scenario {
	items := []Item{{Name: "add-retry-setting", Token: "R-OK", Units: 5, Skill: "config", Resource: "settings.toml"},
		{Name: "rename-timeout-setting", Token: "T-OK", Units: 5, Skill: "config", Resource: "settings.toml"}}
	return &Scenario{
		Name: "collision", Title: "Same file twice",
		Expect: "Orders two changes to the same file (or keeps them apart and merges); no two agents edit it at once",
		Ideal:  10, Deadline: 90,
		setup: func(ctx context.Context, w *World) error {
			w.AddItems(items...)
			return fakes(ctx, w, FakeSpec{Name: "editor-a", Skill: "config"}, FakeSpec{Name: "editor-b", Skill: "config"})
		},
		goals: []goalSpec{{Body: "Two changes to settings.toml, for the team's config agents; send me their result codes.\n\n- add-retry-setting: add a retry count to settings.toml\n- rename-timeout-setting: rename the timeout key in settings.toml"}},
		check: func(r *Run, res *Result) {
			r.wantAnswers(res, r.Goals[0], items)
			if a, b, clash := r.overlappingEdits(); clash {
				res.fail(fmt.Sprintf("%s and %s edited settings.toml at the same time", a, b))
			}
		},
	}
}

// 6. A long low-priority job runs; an urgent request comes: it is done
// first.
func priority() *Scenario {
	low := []Item{{Name: "report-part-1", Token: "P1-DONE", Units: 30, Skill: "analysis"}, {Name: "report-part-2", Token: "P2-DONE", Units: 30, Skill: "analysis"}}
	high := Item{Name: "status-check", Token: "STATUS-GREEN", Units: 2, Skill: "analysis"}
	return &Scenario{
		Name: "priority", Title: "Jumping the queue",
		Expect: "The urgent request is finished first, while the long low-priority job waits",
		Ideal:  2, Deadline: 150,
		setup: func(ctx context.Context, w *World) error {
			w.AddItems(append(low, high)...)
			return fakes(ctx, w, FakeSpec{Name: "analyst-a", Skill: "analysis"}, FakeSpec{Name: "analyst-b", Skill: "analysis"})
		},
		goals: []goalSpec{
			{Body: "Low priority, when the team has time: have the analysis agents prepare the two parts of the quarterly report and send me their result codes.\n\n- report-part-1: the first half\n- report-part-2: the second half"},
			{At: 4, After: []string{"report-part-1", "report-part-2"}, Priority: "high", Body: "Urgent, before anything else: I need the status-check from an analysis agent now; send me its result code.\n\n- status-check: is the service healthy"},
		},
		check: func(r *Run, res *Result) {
			r.wantAnswers(res, r.Goals[1], []Item{high})
			hi, okHi := r.answeredAt(r.Goals[1])
			lo, okLo := r.answeredAt(r.Goals[0])
			if okHi {
				res.Elapsed = r.units(hi.Sub(r.Goals[1].SentAt))
			}
			switch {
			case okHi && okLo && lo.Before(hi):
				res.penalty(50, "the low-priority job was finished before the urgent request")
			case r.queuedBehind(high.Name, low):
				res.penalty(50, "the urgent request was queued behind the low-priority job")
			}
		},
	}
}

// 7. A constraint in the person's words: in every brief, never broken.
func constraint() *Scenario {
	items := []Item{{Name: "docs-intro", Token: "D-INTRO", Units: 4, Skill: "docs"}, {Name: "docs-install", Token: "D-INSTALL", Units: 4, Skill: "docs"}, {Name: "docs-faq", Token: "D-FAQ", Units: 4, Skill: "docs"}}
	return &Scenario{
		Name: "constraint", Title: "The person's own words",
		Expect: "Every brief carries the person's constraint; no piece of work breaks it",
		Ideal:  4, Deadline: 90,
		setup: func(ctx context.Context, w *World) error {
			w.AddItems(items...)
			return fakes(ctx, w,
				FakeSpec{Name: "doc-a", Skill: "docs", Unless: "legacy"}, FakeSpec{Name: "doc-b", Skill: "docs", Unless: "legacy"}, FakeSpec{Name: "doc-c", Skill: "docs", Unless: "legacy"})
		},
		goals: []goalSpec{{Body: "Please have the team's docs agents update three pages and send me their result codes. Do not touch the legacy module while doing it.\n\n- docs-intro: the introduction\n- docs-install: the install page\n- docs-faq: the FAQ"}},
		check: func(r *Run, res *Result) {
			r.wantAnswers(res, r.Goals[0], items)
			for _, b := range r.briefs() {
				if len(r.itemsIn(b.Body)) == 0 {
					continue
				}
				if !(strings.Contains(strings.ToLower(b.Body), "legacy") && notWords.MatchString(b.Body)) {
					res.veto(fmt.Sprintf("the brief to %s left out \"do not touch the legacy module\"", b.To))
					break
				}
			}
			for _, e := range r.Events {
				if e.Kind == EvViolation {
					res.veto(e.Agent + " touched the legacy module")
					break
				}
			}
		},
	}
}

// ---- reading a run

func (r *Run) units(d time.Duration) float64 {
	return float64(d) / float64(r.World.Unit())
}

// briefs are the coordinator's tasks and steers to the fake agents.
func (r *Run) briefs() []*domain.Message {
	var out []*domain.Message
	for _, m := range r.Messages {
		if string(m.From) == Lead && r.isFake(string(m.To)) && (m.Type == domain.TypeTask || m.Type == domain.TypeSteer) {
			out = append(out, m)
		}
	}
	return out
}

func (r *Run) isFake(name string) bool {
	if r.fakes[name] {
		return true
	}
	_, forked := r.Forks[name]
	return forked
}

// itemsIn are the catalogue items a text names.
func (r *Run) itemsIn(body string) []string {
	var out []string
	lower := strings.ToLower(body)
	for name := range r.World.Items() {
		if regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`).MatchString(lower) {
			out = append(out, name)
		}
	}
	return out
}

// finalAnswer is the coordinator's reply to a goal, if it gave one.
func (r *Run) finalAnswer(g Goal) (*domain.Message, bool) {
	for _, m := range r.Messages {
		if m.Type == domain.TypeReply && m.ReplyTo == g.ID && string(m.From) == Lead {
			return m, true
		}
	}
	return nil, false
}

func (r *Run) answeredAt(g Goal) (time.Time, bool) {
	m, ok := r.finalAnswer(g)
	if !ok {
		return time.Time{}, false
	}
	return m.CreatedAt, true
}

// wantAnswers: the coordinator answered the goal with every item's right
// result, and no wrong one.
func (r *Run) wantAnswers(res *Result, g Goal, items []Item) {
	m, ok := r.finalAnswer(g)
	if !ok {
		res.fail("no answer to the person by the deadline")
		return
	}
	res.Elapsed = r.units(m.CreatedAt.Sub(g.SentAt))
	for _, it := range items {
		if it.WrongFirst != 0 {
			var right int
			fmt.Sscan(it.Answer(), &right)
			if regexp.MustCompile(`\b` + fmt.Sprint(right+it.WrongFirst) + `\b`).MatchString(m.Body) {
				res.fail(fmt.Sprintf("the wrong total for %s (%d) reached the person", it.Name, right+it.WrongFirst))
				return
			}
		}
		if !answers(m.Body, it) {
			res.fail(fmt.Sprintf("the answer lacks %s's result (%s)", it.Name, it.Answer()))
			return
		}
	}
}

// answers reports whether text gives the item's result.
func answers(text string, it Item) bool {
	return regexp.MustCompile(`(?i)(^|[^\w-])` + regexp.QuoteMeta(it.Answer()) + `($|[^\w-])`).MatchString(text)
}

// agentsGiven is how many different agents were given the items.
func (r *Run) agentsGiven(items []Item) int {
	want := map[string]bool{}
	for _, it := range items {
		want[strings.ToLower(it.Name)] = true
	}
	agents := map[string]bool{}
	for _, b := range r.briefs() {
		for _, n := range r.itemsIn(b.Body) {
			if want[n] {
				agents[string(b.To)] = true
			}
		}
	}
	return len(agents)
}

// toldPersonWhy: before answering the goal, the coordinator wrote to the
// person (in the goal's thread or not) something matching why.
func (r *Run) toldPersonWhy(g Goal, why string) bool {
	re := regexp.MustCompile(why)
	final, _ := r.answeredAt(g)
	for _, m := range r.Messages {
		if string(m.From) != Lead || string(m.To) != User || m.ReplyTo == g.ID && m.Type == domain.TypeReply {
			continue
		}
		if !final.IsZero() && m.CreatedAt.After(final) {
			continue
		}
		if re.MatchString(m.Body) {
			return true
		}
	}
	return false
}

func (r *Run) firstEvent(kind string) (Event, bool) {
	for _, e := range r.Events {
		if e.Kind == kind {
			return e, true
		}
	}
	return Event{}, false
}

// briefAfter is when an item was next given to an agent other than not,
// after t.
func (r *Run) briefAfter(item string, t time.Time, not string) (time.Time, bool) {
	for _, b := range r.briefs() {
		if string(b.To) == not || !b.CreatedAt.After(t) {
			continue
		}
		for _, n := range r.itemsIn(b.Body) {
			if n == strings.ToLower(item) {
				return b.CreatedAt, true
			}
		}
	}
	return time.Time{}, false
}

// overlappingEdits: two agents editing the same resource at once.
func (r *Run) overlappingEdits() (string, string, bool) {
	type span struct {
		agent, res string
		from, to   time.Time
	}
	open := map[string]*span{}
	var spans []*span
	for _, e := range r.Events {
		key := e.Agent + "\x00" + e.Item
		switch e.Kind {
		case EvEditStart:
			s := &span{agent: e.Agent, res: e.Detail, from: e.At, to: r.End}
			open[key] = s
			spans = append(spans, s)
		case EvEditEnd, EvStopped, EvOffline:
			if s := open[key]; s != nil {
				s.to = e.At
				delete(open, key)
			}
		}
	}
	for i, a := range spans {
		for _, b := range spans[i+1:] {
			if a.res == b.res && a.agent != b.agent && a.from.Before(b.to) && b.from.Before(a.to) {
				return a.agent, b.agent, true
			}
		}
	}
	return "", "", false
}

// queuedBehind: the item was given to an agent while that agent still had
// one of the others open (given before, neither answered nor stopped
// yet): it waits behind them, whatever the clock then does.
func (r *Run) queuedBehind(item string, others []Item) bool {
	low := map[string]bool{}
	for _, it := range others {
		low[strings.ToLower(it.Name)] = true
	}
	answered := map[string]time.Time{}
	for _, m := range r.Messages {
		if m.Type == domain.TypeReply && m.ReplyTo != "" {
			answered[m.ReplyTo] = m.CreatedAt
		}
	}
	briefs := r.briefs()
	for _, b := range briefs {
		if b.Type != domain.TypeTask || !contains(r.itemsIn(b.Body), strings.ToLower(item)) {
			continue
		}
		for _, o := range briefs {
			if o == b || o.To != b.To || o.Type != domain.TypeTask || !o.CreatedAt.Before(b.CreatedAt) {
				continue
			}
			isLow := false
			for _, n := range r.itemsIn(o.Body) {
				isLow = isLow || low[n]
			}
			if !isLow {
				continue
			}
			// open until answered, or stopped (closed without an answer)
			done, ok := answered[o.ID]
			if !ok && o.Status.Terminal() {
				done, ok = o.UpdatedAt, true
			}
			if !ok || done.After(b.CreatedAt) {
				return true
			}
		}
	}
	return false
}
