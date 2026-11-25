package runner

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/harness"
)

type projectHub struct {
	*fakeHub
	reads  int
	brief  string
	memory *api.ProjectDoc
}

func (h *projectHub) GetProject(_ context.Context, name string, _ bool) (*api.Project, error) {
	h.reads++
	p := &api.Project{Name: name, Title: "myco 开发", Goal: "ship projects", Brief: "Run make check before pushing." + h.brief,
		Members: []api.ProjectMember{{Name: "lead", Role: "coordinator", Note: "splits work"}, {Name: "me", Note: "后端"}},
		Docs:    []api.ProjectDoc{{Name: "conventions"}}}
	if h.memory != nil {
		m := *h.memory
		p.Memory = &m
		p.Docs = append(p.Docs, api.ProjectDoc{Name: m.Name})
	}
	return p, nil
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
	if hub.reads != 3 {
		t.Fatalf("project read %d times", hub.reads)
	}
	if strings.Contains(first, "Project memory") {
		t.Fatalf("an older hub has no project memory:\n%s", first)
	}
}

// The project memory and the whole brief go in the prompt; when the memory
// changes, the next message of the session gets the project again.
func TestProjectMemoryInPrompt(t *testing.T) {
	long := "\n" + strings.Repeat("背景资料。", 1500) + "\nThe staging port is 7443."
	hub := &projectHub{brief: long, memory: &api.ProjectDoc{Name: "memory", Body: "# Project memory\n- alice wants reports in Chinese (lead, 2026-11-21)\n"},
		fakeHub: newFakeHub(
			api.Message{ID: "t1", Type: "task", From: "lead", ThreadID: "th", Project: "myco", Body: "one"},
			api.Message{ID: "t2", Type: "task", From: "lead", ThreadID: "th", Project: "myco", Body: "two"},
			api.Message{ID: "t3", Type: "task", From: "lead", ThreadID: "th", Project: "myco", Body: "three"},
		)}
	var prompts []string
	h := fakeHarness{fn: func(r harness.Request) (harness.Result, error) {
		prompts = append(prompts, r.Prompt)
		return harness.Result{Output: "ok", SessionID: "s"}, nil
	}}
	r := New(hub, h, NewHubSessions(&fakeBinder{}, ""), nil, Options{Self: "me", Once: true, Concurrency: 1}, quiet())
	r.Run(context.Background())
	r.Run(context.Background())
	hub.memory.Body += "- use port 7443 for staging (me, 2026-11-21)\n"
	hub.memory.UpdatedAt = hub.memory.UpdatedAt.Add(time.Minute)
	r.Run(context.Background())
	if len(prompts) != 3 {
		t.Fatalf("runs: %d", len(prompts))
	}
	for _, want := range []string{"The staging port is 7443.", "Project memory", "alice wants reports in Chinese", "myco project memory add --name myco", "--ui ui.json", "myco ui schema --format text"} {
		if !strings.Contains(prompts[0], want) {
			t.Fatalf("prompt lacks %q", want)
		}
	}
	if strings.Contains(prompts[0], "Shared documents: conventions, memory") {
		t.Fatal("the memory is not listed among the documents")
	}
	if strings.Contains(prompts[1], "Project memory") {
		t.Fatalf("unchanged project: one line:\n%s", prompts[1])
	}
	if strings.Contains(prompts[1], "--ui") {
		t.Fatal("structured UI guidance must not repeat on every task")
	}
	if !strings.Contains(prompts[2], "updated since you last saw it") || !strings.Contains(prompts[2], "use port 7443 for staging") {
		t.Fatalf("changed memory is given again:\n%s", prompts[2])
	}
	if !strings.Contains(prompts[2], "myco ui schema --format text") {
		t.Fatal("updated project context must include structured UI guidance")
	}
}

// Raising binary file quotas must not inject thousands of file names into a prompt.
func TestProjectContextSummarizesBinaryFiles(t *testing.T) {
	p := &api.Project{Name: "large", Docs: []api.ProjectDoc{{Name: "guide"}, {Name: "first.png", Binary: true}, {Name: "second.mp4", Binary: true}}}
	text := projectSection("me", p)
	if !strings.Contains(text, "Shared documents: guide") || !strings.Contains(text, "Other shared files: 2") || strings.Contains(text, "first.png") || strings.Contains(text, "second.mp4") {
		t.Fatal(text)
	}
}
