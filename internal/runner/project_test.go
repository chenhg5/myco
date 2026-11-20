package runner

import (
	"context"
	"strings"
	"testing"

	"github.com/chenhg5/myco/internal/api"
	"github.com/chenhg5/myco/internal/harness"
)

type projectHub struct {
	*fakeHub
	reads int
}

func (h *projectHub) GetProject(_ context.Context, name string, _ bool) (*api.Project, error) {
	h.reads++
	return &api.Project{Name: name, Title: "myco 开发", Goal: "ship projects", Brief: "Run make check before pushing.",
		Members: []api.ProjectMember{{Name: "lead", Role: "coordinator", Note: "splits work"}, {Name: "me", Note: "后端"}},
		Docs:    []api.ProjectDoc{{Name: "conventions"}}}, nil
}

// A project task gets the project's context once per session, and after
// that a line naming its project (an agent in several projects gets them in
// one session); a message without a project, after project work, says so.
func TestProjectContextInPrompt(t *testing.T) {
	hub := &projectHub{fakeHub: newFakeHub(
		api.Message{ID: "t1", Type: "task", From: "lead", ThreadID: "th1", Project: "myco", Body: "add the filter"},
		api.Message{ID: "t2", Type: "task", From: "lead", ThreadID: "th2", Project: "myco", Body: "and docs"},
		api.Message{ID: "t3", Type: "task", From: "lead", ThreadID: "th3", Body: "unrelated"},
		api.Message{ID: "t4", Type: "task", From: "lead", ThreadID: "th4", Project: "site", Body: "new landing page"},
	)}
	var prompts []string
	h := fakeHarness{fn: func(r harness.Request) (harness.Result, error) {
		prompts = append(prompts, r.Prompt)
		return harness.Result{Output: "ok", SessionID: "s"}, nil
	}}
	r := New(hub, h, NewHubSessions(&fakeBinder{}, ""), nil, Options{Self: "me", Once: true, Concurrency: 1}, quiet())
	for range 4 {
		r.Run(context.Background())
	}
	if len(prompts) != 4 {
		t.Fatalf("runs: %d", len(prompts))
	}
	first := prompts[0]
	for _, want := range []string{`project "myco" (myco 开发)`, "Goal: ship projects", "- lead [coordinator]: splits work", "- me (you): 后端",
		"Run make check before pushing.", "Shared documents: conventions"} {
		if !strings.Contains(first, want) {
			t.Fatalf("prompt lacks %q:\n%s", want, first)
		}
	}
	if strings.Index(first, "--- project ---") > strings.Index(first, "--- message body ---") {
		t.Fatalf("the project goes before the body:\n%s", first)
	}
	if strings.Contains(prompts[1], "Goal:") || !strings.Contains(prompts[1], `This message belongs to project "myco"`) {
		t.Fatalf("a later message of the project: the line naming it, not the whole context again:\n%s", prompts[1])
	}
	if !strings.Contains(prompts[2], "not part of a project") {
		t.Fatalf("a message outside projects after project work should say so:\n%s", prompts[2])
	}
	if !strings.Contains(prompts[3], `project "site"`) || !strings.Contains(prompts[3], "Goal:") {
		t.Fatalf("another project gets its own context:\n%s", prompts[3])
	}
	if hub.reads != 2 {
		t.Fatalf("project read %d times", hub.reads)
	}
}
