package coordeval

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
)

// Baseline is a scripted coordinator, for testing the harness without
// spending tokens. It reads a goal's items (its "- name: ..." lines), its
// constraint and whether it is urgent, and coordinates as the scenarios
// expect: in parallel on idle agents of the right kind, the person's
// constraint in every brief, results checked, a lost agent's work given to
// another, two edits of one file one after the other, an urgent request
// first (a low-priority task is stopped and given out again later), and a
// word to the person when the only agent able to do it is busy.
//
// Naive does none of that: everything to the first agent that fits, the
// goal's items only, its answer passed on as it is. The checkers must
// catch it.
type Baseline struct {
	Naive bool
}

// Name says which.
func (b Baseline) Name() string {
	if b.Naive {
		return "naive"
	}
	return "baseline"
}

// Start serves the coordinator's inbox until stopped.
func (b Baseline) Start(ctx context.Context, w *World) (func(), error) {
	ctx, cancel := context.WithCancel(ctx)
	s := &baseline{w: w, c: w.CoordClient(), naive: b.Naive, busy: map[string]*assignment{}}
	s.wg.Add(1)
	go s.serve(ctx)
	return func() { cancel(); s.wg.Wait() }, nil
}

type baseline struct {
	w     *World
	c     *client.Client
	naive bool
	wg    sync.WaitGroup

	mu   sync.Mutex
	busy map[string]*assignment // agent -> what it does for us now
	// urgent counts urgent items waiting for an agent: the rest wait.
	urgent int
}

type assignment struct {
	agent, item, msg string
	high             bool
	stopped          bool
}

type goalItem struct {
	name, line, resource string
	numbers              []int
}

var resourceRe = regexp.MustCompile(`\b[\w-]+\.(toml|yaml|yml|json|md|go|txt)\b`)

func (s *baseline) serve(ctx context.Context) {
	defer s.wg.Done()
	for ctx.Err() == nil {
		msgs, err := s.c.Pull(ctx, 5, s.w.Unit())
		if err != nil {
			sleepCtx(ctx, s.w.Unit())
			continue
		}
		for _, m := range msgs {
			if m.Type != "task" {
				continue
			}
			_ = s.c.Ack(ctx, m.ID)
			s.wg.Add(1)
			go func(m api.Message) {
				defer s.wg.Done()
				s.goal(ctx, m)
			}(m)
		}
	}
}

func parseGoal(body string) (items []goalItem, constraint string, urgent bool) {
	for _, line := range strings.Split(body, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "- ") {
			name, _, _ := strings.Cut(strings.TrimPrefix(t, "- "), ":")
			it := goalItem{name: strings.TrimSpace(name), line: t, resource: resourceRe.FindString(t), numbers: numbersFor(t, strings.TrimSpace(name))}
			items = append(items, it)
			continue
		}
		for _, sentence := range regexp.MustCompile(`[.!?。]\s*`).Split(t, -1) {
			if regexp.MustCompile(`(?i)\b(do not|don't|never|must not)\b`).MatchString(sentence) {
				constraint = strings.TrimSpace(sentence) + "."
			}
		}
	}
	urgent = regexp.MustCompile(`(?i)^\s*urgent`).MatchString(body)
	return items, constraint, urgent
}

// skillFor is the kind of agent the goal asks for: an agent's capability
// the goal names.
func (s *baseline) skillFor(ctx context.Context, body string) string {
	agents, err := s.c.ListAgents(ctx, "", false)
	if err != nil {
		return ""
	}
	lower := strings.ToLower(body)
	for _, a := range agents {
		for _, c := range a.Capabilities {
			if c != "coordinate" && strings.Contains(lower, strings.ToLower(c)) {
				return c
			}
		}
	}
	return ""
}

func (s *baseline) candidates(ctx context.Context, skill string) []api.Agent {
	agents, err := s.c.ListAgents(ctx, skill, false)
	if err != nil {
		return nil
	}
	var out []api.Agent
	for _, a := range agents {
		if a.Name != Lead && a.Online {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		li, lj := out[i].Queued+out[i].InProgress, out[j].Queued+out[j].InProgress
		if li != lj {
			return li < lj
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func (s *baseline) goal(ctx context.Context, g api.Message) {
	items, constraint, urgent := parseGoal(g.Body)
	skill := s.skillFor(ctx, g.Body)
	if s.naive {
		s.naiveGoal(ctx, g, items, skill)
		return
	}
	results := make([]string, len(items))
	// items sharing a file go one after the other; the rest in parallel
	groups := map[string][]int{}
	var order []string
	for i, it := range items {
		key := it.resource
		if key == "" {
			key = "#" + strconv.Itoa(i)
		}
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], i)
	}
	var wg sync.WaitGroup
	for _, key := range order {
		wg.Add(1)
		go func(idx []int) {
			defer wg.Done()
			for _, i := range idx {
				results[i] = s.item(ctx, g, items[i], constraint, skill, urgent)
			}
		}(groups[key])
	}
	wg.Wait()
	if ctx.Err() != nil {
		return
	}
	var lines []string
	for i, it := range items {
		lines = append(lines, it.name+": "+results[i])
	}
	_, _ = s.c.Reply(ctx, g.ID, strings.Join(lines, "\n"), false)
}

// item gets one item done: on a free agent of the right kind, checked,
// again elsewhere when its agent goes away or answers wrong.
func (s *baseline) item(ctx context.Context, g api.Message, it goalItem, constraint, skill string, urgent bool) string {
	tried := map[string]bool{}
	note := ""
	toldWait := false
	for ctx.Err() == nil {
		if urgent {
			// the rest wait while this one waits for an agent
			s.mu.Lock()
			s.urgent++
			s.mu.Unlock()
		}
		agent := ""
		for ctx.Err() == nil {
			if agent = s.pick(ctx, skill, tried, urgent, g, it, &toldWait); agent != "" || !urgent {
				break
			}
			sleepCtx(ctx, s.w.Unit()/2)
		}
		if urgent {
			s.mu.Lock()
			s.urgent--
			s.mu.Unlock()
		}
		if agent == "" {
			sleepCtx(ctx, s.w.Unit())
			continue
		}
		brief := fmt.Sprintf("%s\n\nPlease do this one piece of work and reply with its result.\n%s", it.line, note)
		if constraint != "" {
			brief += "\nFrom the person's request, in their words: " + constraint
		}
		sent, err := s.c.Send(ctx, api.SendRequest{To: agent, Type: "task", Body: brief, Project: Project}, false)
		if err != nil {
			s.release(agent)
			sleepCtx(ctx, s.w.Unit())
			continue
		}
		a := &assignment{agent: agent, item: it.name, msg: sent.Message.ID, high: urgent}
		s.mu.Lock()
		s.busy[agent] = a
		s.mu.Unlock()
		answer, outcome := s.await(ctx, a)
		s.release(agent)
		switch outcome {
		case "stopped":
			// made way for something urgent: again, later
			continue
		case "gone":
			tried[agent] = true
			continue
		}
		result := resultFor(answer, it.name)
		if len(it.numbers) > 0 {
			want := 0
			for _, n := range it.numbers {
				want += n
			}
			if got, err := strconv.Atoi(result); err != nil || got != want {
				// a wrong total: done again, by someone else
				tried[agent] = true
				note = "An earlier total for this list was wrong; please add it up carefully."
				continue
			}
		}
		return result
	}
	return ""
}

// pick reserves a free agent of the right kind; for an urgent item with
// none free, it stops a low-priority task to make room.
func (s *baseline) pick(ctx context.Context, skill string, tried map[string]bool, urgent bool, g api.Message, it goalItem, toldWait *bool) string {
	cands := s.candidates(ctx, skill)
	s.mu.Lock()
	defer s.mu.Unlock()
	if !urgent && s.urgent > 0 {
		return ""
	}
	for _, a := range cands {
		if tried[a.Name] || s.busy[a.Name] != nil {
			continue
		}
		if a.Queued+a.InProgress > 0 {
			// busy with others' work: the only one there is, say so
			free := 0
			for _, b := range cands {
				if s.busy[b.Name] == nil && b.Queued+b.InProgress == 0 {
					free++
				}
			}
			if free > 0 {
				continue
			}
			if !*toldWait {
				*toldWait = true
				msg := fmt.Sprintf("%s is the only agent who can do %s, and it is busy with %d other task(s): I have queued %s behind them and will send you the result when it is done.",
					a.Name, it.name, a.Queued+a.InProgress, it.name)
				go func() { _, _ = s.c.Progress(context.WithoutCancel(ctx), g.ID, msg, false) }()
			}
		}
		s.busy[a.Name] = &assignment{agent: a.Name}
		return a.Name
	}
	if urgent {
		// make room: stop one low-priority task (none already being
		// stopped), which goes out again later
		for _, a := range s.busy {
			if a.stopped {
				return ""
			}
		}
		for name, a := range s.busy {
			if !a.high && a.msg != "" && !tried[name] {
				a.stopped = true
				go func(id string) { _, _ = s.c.StopTask(context.WithoutCancel(ctx), id, false) }(a.msg)
				break
			}
		}
	}
	return ""
}

func (s *baseline) release(agent string) {
	s.mu.Lock()
	delete(s.busy, agent)
	s.mu.Unlock()
}

// await waits for a task's reply: "ok" with it, "gone" when its agent went
// away, "stopped" when we stopped it.
func (s *baseline) await(ctx context.Context, a *assignment) (string, string) {
	for ctx.Err() == nil {
		reply, err := s.c.WaitReply(ctx, a.msg, s.w.Unit())
		if err == nil && reply != nil {
			return reply.Body, "ok"
		}
		s.mu.Lock()
		stopped := a.stopped
		s.mu.Unlock()
		if stopped {
			return "", "stopped"
		}
		if agents, err := s.c.ListAgents(ctx, "", false); err == nil {
			for _, ag := range agents {
				if ag.Name == a.agent && !ag.Online {
					_, _ = s.c.StopTask(ctx, a.msg, false)
					return "", "gone"
				}
			}
		}
	}
	return "", "gone"
}

// resultFor is an item's result in a reply: what follows "total" or
// "result" on the line that names it.
func resultFor(reply, item string) string {
	for _, line := range strings.Split(reply, "\n") {
		if !strings.Contains(strings.ToLower(line), strings.ToLower(item)) {
			continue
		}
		if m := regexp.MustCompile(`(?i)(?:total|result)\s+(\S+)`).FindStringSubmatch(line); m != nil {
			return strings.TrimRight(m[1], ".,;")
		}
	}
	return strings.TrimSpace(reply)
}

// naiveGoal: everything to the first agent that fits, the goal's items
// only, its answer passed on as it is.
func (s *baseline) naiveGoal(ctx context.Context, g api.Message, items []goalItem, skill string) {
	cands := s.candidates(ctx, skill)
	if len(cands) == 0 {
		return
	}
	// the first by name, however busy
	sort.Slice(cands, func(i, j int) bool { return cands[i].Name < cands[j].Name })
	var lines []string
	for _, it := range items {
		lines = append(lines, it.line)
	}
	sent, err := s.c.Send(ctx, api.SendRequest{To: cands[0].Name, Type: "task", Body: strings.Join(lines, "\n"), Project: Project}, false)
	if err != nil {
		return
	}
	for ctx.Err() == nil {
		reply, err := s.c.WaitReply(ctx, sent.Message.ID, s.w.Unit())
		if err == nil && reply != nil {
			_, _ = s.c.Reply(ctx, g.ID, reply.Body, false)
			return
		}
	}
}

func sleepCtx(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}
