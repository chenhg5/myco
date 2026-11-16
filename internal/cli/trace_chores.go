package cli

import (
	"regexp"
	"strings"

	"github.com/chenhg5/myco/internal/harness"
)

// A session that receives in its own conversation (myco inbox wait) wakes
// up now and then only to keep listening: its wait timed out and it starts
// it again, or a message came and it pulls and acknowledges it. Such a turn
// is not work on the task being followed. Uploaded, it would read in the
// App as the agent being at it again (a loading under an answer it already
// gave), with "still waiting" in its steps. turnGate leaves those turns out.
//
// The turn the follower starts in (the one that acknowledged the task) is
// always work. After a turn ends, the next one is held back until it shows
// what it is: a tool call that is not a chore sends it all on its way at
// once; an end with only chores in it drops it; an end without any tool
// call (it only spoke) sends it.
type turnGate struct {
	ended  bool // a turn has ended since the follower started
	work   bool // the current turn has done real work: pass it through
	chores bool // the current turn has run a chore
	held   []harness.Event
}

func (g *turnGate) add(ev harness.Event) []harness.Event {
	if !g.ended || g.work {
		if ev.Kind == "turn_end" {
			g.ended, g.work = true, false
		}
		return []harness.Event{ev}
	}
	switch {
	case ev.Kind == "tool" && isChore(ev.Input):
		g.chores = true
		g.held = append(g.held, ev)
		return nil
	case ev.Kind == "tool":
		out := append(g.held, ev)
		g.held, g.chores, g.work = nil, false, true
		return out
	case ev.Kind == "turn_end":
		out := append(g.held, ev)
		chores := g.chores
		g.held, g.chores = nil, false
		if chores {
			return nil
		}
		return out
	default:
		g.held = append(g.held, ev)
		return nil
	}
}

var (
	// a shell the CLI wraps the command in: bash -lc "…"
	shellWrap = regexp.MustCompile(`^(?:/\S*/)?(?:ba|z)?sh\s+-l?c\s+`)
	chore     = regexp.MustCompile(`^myco\s+(?:inbox\s+(?:wait|pull)|message\s+(?:ack|done|release))(?:\s|$)`)
	cdTo      = regexp.MustCompile(`^cd(?:\s|$)`)
	cmdParts  = regexp.MustCompile(`&&|\|\||;|\n`)
)

// isChore says whether a command only keeps the session listening: every
// part of it is myco inbox wait/pull, myco message ack/done/release, or a
// cd before them.
func isChore(input string) bool {
	cmd := strings.TrimSpace(shellWrap.ReplaceAllString(strings.TrimSpace(input), ""))
	cmd = strings.Trim(cmd, `"'`)
	if cmd == "" {
		return false
	}
	seen := false
	for _, part := range cmdParts.Split(cmd, -1) {
		p := strings.TrimSpace(part)
		// what is piped on (| head, | tail) or redirected does not change it
		if i := strings.Index(p, "|"); i >= 0 {
			p = strings.TrimSpace(p[:i])
		}
		switch {
		case p == "", cdTo.MatchString(p):
		case chore.MatchString(p):
			seen = true
		default:
			return false
		}
	}
	return seen
}
