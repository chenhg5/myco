package runner

import (
	"context"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/harness"
)

type superHub struct {
	*fakeHub
	owner string
}

func (h *superHub) MeSuperagent(context.Context) (*api.MeSuperagent, error) {
	return &api.MeSuperagent{Superagent: h.owner != "", Owner: h.owner}, nil
}

// A superagent is told what it is: the brief the first time in a session, a
// line after; an ordinary agent is told nothing.
func TestSuperagentBriefInPrompt(t *testing.T) {
	hub := &superHub{fakeHub: newFakeHub(
		api.Message{ID: "t1", Type: "task", From: "@console:glenn", ThreadID: "th1", Body: "set up a project"},
		api.Message{ID: "t2", Type: "task", From: "@console:glenn", ThreadID: "th2", Body: "and who is stuck?"},
	), owner: "glenn"}
	var prompts []string
	h := fakeHarness{fn: func(r harness.Request) (harness.Result, error) {
		prompts = append(prompts, r.Prompt)
		return harness.Result{Output: "ok", SessionID: "s"}, nil
	}}
	r := New(hub, h, NewHubSessions(&fakeBinder{}, ""), nil, Options{Self: "shroom", Once: true, Concurrency: 1}, quiet())
	r.Run(context.Background())
	r.Run(context.Background())
	if len(prompts) != 2 {
		t.Fatalf("runs: %d", len(prompts))
	}
	if !strings.Contains(prompts[0], "You are glenn's superagent") || !strings.Contains(prompts[0], "approval_pending") ||
		!strings.Contains(prompts[0], "never speak as glenn") {
		t.Fatalf("brief:\n%s", prompts[0])
	}
	if strings.Contains(prompts[1], "chief of staff") || !strings.Contains(prompts[1], "glenn's superagent (the brief is earlier") {
		t.Fatalf("second:\n%s", prompts[1])
	}

	plain := &superHub{fakeHub: newFakeHub(api.Message{ID: "t3", Type: "task", From: "@console:glenn", ThreadID: "th3", Body: "hi"})}
	prompts = nil
	New(plain, h, NewHubSessions(&fakeBinder{}, ""), nil, Options{Self: "coder", Once: true, Concurrency: 1}, quiet()).Run(context.Background())
	if len(prompts) != 1 || strings.Contains(prompts[0], "superagent") {
		t.Fatalf("ordinary agent:\n%s", prompts)
	}
}
