package coordeval

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
)

// FakeSpec is a scripted agent: what it can do, how fast, and what goes
// wrong with it.
type FakeSpec struct {
	Name        string
	Description string
	// Skill is what it does, as its capability and in its description.
	Skill string
	// Speed scales how long items take it (1 = as the item says).
	Speed float64
	// Jitter is the spread of its latency, as a fraction (0.2 = ±20%).
	Jitter float64
	// GoesOfflineAt, between 0 and 1, makes it go away that far into the
	// first item it works on, without answering, for good.
	GoesOfflineAt float64
	// Unless, when set, is a phrase of the person's constraint (for
	// example "legacy"): a brief without it, and without a "not", makes
	// it also touch that thing (a violation).
	Unless string
	// Queue is work it already has, from another team, when the drill
	// starts: items it does first.
	Queue []string
}

// Fake is a running scripted agent.
type Fake struct {
	FakeSpec
	w       *World
	c       *client.Client
	offline bool
	done    map[string]int // answers given per item
}

// AddFake registers a scripted agent.
func (w *World) AddFake(ctx context.Context, spec FakeSpec) (*Fake, error) {
	if spec.Speed <= 0 {
		spec.Speed = 1
	}
	desc := spec.Description
	if desc == "" {
		desc = "Does " + spec.Skill + " work"
	}
	reg, err := w.admin.Register(ctx, api.RegisterRequest{Name: spec.Name, Description: desc, Capabilities: []string{spec.Skill},
		Labels: []string{"skill=" + spec.Skill}}, false)
	if err != nil {
		return nil, fmt.Errorf("agent %s: %w", spec.Name, err)
	}
	c, err := client.New(w.URL, reg.Token)
	if err != nil {
		return nil, err
	}
	f := &Fake{FakeSpec: spec, w: w, c: c, done: map[string]int{}}
	w.mu.Lock()
	w.fakes = append(w.fakes, f)
	w.mu.Unlock()
	return f, nil
}

// preload queues the fake's other-team work (before the drill runs).
func (w *World) preload(ctx context.Context) error {
	var other *client.Client
	for _, f := range w.fakes {
		if len(f.Queue) == 0 {
			continue
		}
		if other == nil {
			reg, err := w.admin.Register(ctx, api.RegisterRequest{Name: OtherTeam, Description: "Another team's agent"}, false)
			if err != nil {
				return err
			}
			if other, err = client.New(w.URL, reg.Token); err != nil {
				return err
			}
		}
		for _, item := range f.Queue {
			if _, err := other.Send(ctx, api.SendRequest{To: f.Name, Type: "task", Body: "Please do " + item + " for our team."}, false); err != nil {
				return err
			}
		}
	}
	return nil
}

var notWords = regexp.MustCompile(`(?i)\b(not|never|don't|do not|must not|mustn't|without|leave|untouched|avoid)\b|不要|别|不许|不能|勿|保持不动`)

// run takes tasks one at a time, as a runner does.
func (f *Fake) run(ctx context.Context) {
	for ctx.Err() == nil {
		if f.offline {
			<-ctx.Done()
			return
		}
		msgs, err := f.c.Pull(ctx, 1, f.w.opts.Unit)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(f.w.opts.Unit / 2):
			}
			continue
		}
		for _, m := range msgs {
			if m.Type != "task" {
				continue
			}
			f.handle(ctx, m)
			if f.offline {
				break
			}
		}
	}
}

// items are the catalogue's items a brief names.
func (f *Fake) items(body string) []Item {
	var out []Item
	lower := strings.ToLower(body)
	for name, it := range f.w.Items() {
		if regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`).MatchString(lower) {
			out = append(out, it)
		}
	}
	// in the order the brief names them
	idx := func(it Item) int { return strings.Index(lower, strings.ToLower(it.Name)) }
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if idx(out[j]) < idx(out[i]) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// numbersFor are the numbers a brief gives for a list item: those on the
// line that names it.
func numbersFor(body, name string) []int {
	for _, line := range strings.Split(body, "\n") {
		l := strings.ToLower(line)
		i := strings.Index(l, strings.ToLower(name))
		if i < 0 {
			continue
		}
		var out []int
		for _, s := range regexp.MustCompile(`-?\d+`).FindAllString(line[i+len(name):], -1) {
			if n, err := strconv.Atoi(s); err == nil {
				out = append(out, n)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

func (f *Fake) handle(ctx context.Context, m api.Message) {
	_ = f.c.Ack(ctx, m.ID)
	items := f.items(m.Body)
	if len(items) == 0 {
		f.w.record(Event{Agent: f.Name, Kind: EvAskBack, Msg: m.ID, Detail: "no item named"})
		_, _ = f.c.Reply(ctx, m.ID, "I can't tell what this is about: which item do you mean? Please name it and give me what I need.", false)
		return
	}
	// a list needs its numbers in the brief
	for _, it := range items {
		if len(it.Numbers) > 0 && len(numbersFor(m.Body, it.Name)) == 0 {
			f.w.record(Event{Agent: f.Name, Kind: EvAskBack, Item: it.Name, Msg: m.ID, Detail: "numbers missing"})
			_, _ = f.c.Reply(ctx, m.ID, fmt.Sprintf("I need the numbers of the %s list to add it up; the request did not have them.", it.Name), false)
			return
		}
	}
	for _, it := range items {
		if it.Skill != "" && it.Skill != f.Skill {
			f.w.record(Event{Agent: f.Name, Kind: EvAskBack, Item: it.Name, Msg: m.ID, Detail: "not its skill"})
			_, _ = f.c.Reply(ctx, m.ID, fmt.Sprintf("%s is %s work; I only do %s. Please ask someone who does %s.", it.Name, it.Skill, f.Skill, it.Skill), false)
			return
		}
	}
	if f.Unless != "" && !(strings.Contains(strings.ToLower(m.Body), strings.ToLower(f.Unless)) && notWords.MatchString(m.Body)) {
		f.w.record(Event{Agent: f.Name, Kind: EvViolation, Msg: m.ID, Detail: "touched " + f.Unless})
	}
	var lines []string
	for i, it := range items {
		units := it.Units * f.Speed * (1 + f.Jitter*(2*f.w.randFloat()-1))
		f.w.record(Event{Agent: f.Name, Kind: EvStart, Item: it.Name, Msg: m.ID})
		if it.Resource != "" {
			f.w.record(Event{Agent: f.Name, Kind: EvEditStart, Item: it.Name, Msg: m.ID, Detail: it.Resource})
		}
		if i == 0 {
			_, _ = f.c.Progress(ctx, m.ID, "Working on "+it.Name+".", false)
		}
		if !f.work(ctx, m.ID, it, f.w.Units(units)) {
			return
		}
		if it.Resource != "" {
			f.w.record(Event{Agent: f.Name, Kind: EvEditEnd, Item: it.Name, Msg: m.ID, Detail: it.Resource})
		}
		answer := f.w.answer(it)
		f.done[it.Name]++
		f.w.record(Event{Agent: f.Name, Kind: EvFinish, Item: it.Name, Msg: m.ID, Detail: answer})
		if len(it.Numbers) > 0 {
			lines = append(lines, fmt.Sprintf("%s: total %s", it.Name, answer))
		} else {
			lines = append(lines, fmt.Sprintf("%s: done, result %s", it.Name, answer))
		}
	}
	_, _ = f.c.Reply(ctx, m.ID, strings.Join(lines, "\n"), false)
}

// work spends d on an item, saying so now and then; false when it stopped
// (went offline, or the task was stopped).
func (f *Fake) work(ctx context.Context, msgID string, it Item, d time.Duration) bool {
	offlineAt := time.Duration(math.MaxInt64)
	if f.GoesOfflineAt > 0 && len(f.done) == 0 {
		offlineAt = time.Duration(f.GoesOfflineAt * float64(d))
	}
	step := max(f.w.opts.Unit/2, 10*time.Millisecond)
	for spent := time.Duration(0); spent < d; spent += step {
		if spent >= offlineAt {
			f.offline = true
			f.w.record(Event{Agent: f.Name, Kind: EvOffline, Item: it.Name, Msg: msgID})
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(min(step, d-spent)):
		}
		// stopped by its sender?
		if got, err := f.c.GetMessage(ctx, msgID); err == nil && (got.Status == "done" || got.Status == "failed") {
			f.w.record(Event{Agent: f.Name, Kind: EvStopped, Item: it.Name, Msg: msgID})
			return false
		}
	}
	return true
}

// answer is what an agent answers for an item: its first answer for an
// item that is answered wrong once is off (a plausible, wrong total),
// whoever does it; later ones are right.
func (w *World) answer(it Item) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.answered[strings.ToLower(it.Name)]++
	if it.WrongFirst != 0 && w.answered[strings.ToLower(it.Name)] == 1 && len(it.Numbers) > 0 {
		sum, _ := strconv.Atoi(it.Answer())
		return strconv.Itoa(sum + it.WrongFirst)
	}
	return it.Answer()
}

func (w *World) randFloat() float64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.rng.Float64()
}
