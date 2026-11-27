package integration_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myco-ai/myco/internal/api"
	"github.com/myco-ai/myco/internal/client"
)

func TestCoordinationCommandsRealHub(t *testing.T) {
	c := ctx(t)
	dir := t.TempDir()
	h := startHub(t, filepath.Join(dir, "hub.db"), policy(time.Minute, 5))
	coordinator := h.client(h.register(api.RegisterRequest{Name: "coordinator", Capabilities: []string{"plan"}}))
	worker := h.client(h.register(api.RegisterRequest{Name: "worker", Capabilities: []string{"code"}, Binding: api.Binding{Harness: "codex", Environment: "demo-mac"}}))
	outsider := h.client(h.register(api.RegisterRequest{Name: "outsider"}))
	task, err := coordinator.Send(c, api.SendRequest{To: "worker", Body: "private example task"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = worker.Pull(c, 1, 0); err != nil {
		t.Fatal(err)
	}
	if err = worker.Ack(c, task.Message.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = worker.Reply(c, task.Message.ID, "private example reply", false); err != nil {
		t.Fatal(err)
	}
	stats, err := coordinator.AgentStats(c, client.AgentStatsFilter{Name: "worker"})
	if err != nil {
		t.Fatal(err)
	}
	if len(stats.Agents) != 1 || stats.Agents[0].Done != 1 || stats.Agents[0].QueueWait.Samples != 1 {
		t.Fatalf("stats: %+v", stats)
	}
	raw, _ := json.Marshal(stats)
	if strings.Contains(string(raw), "private example") {
		t.Fatal("body leaked")
	}
	hidden, err := outsider.AgentStats(c, client.AgentStatsFilter{Name: "worker"})
	if err != nil {
		t.Fatal(err)
	}
	if hidden.Agents[0].Tasks != 0 {
		t.Fatal("unrelated history disclosed")
	}
	picked, err := coordinator.PickAgents(c, client.AgentPickFilter{Skill: "code", CLI: "codex", Machine: "demo-mac", Count: 1, Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(picked.Candidates) != 1 || picked.Candidates[0].Name != "worker" {
		t.Fatalf("pick: %+v", picked)
	}

	// Real person -> coordinator -> worker flow, including a follow-up,
	// a stopped delegation and an observable replacement in the same thread.
	person, err := h.admin().ConsoleSend(c, api.SendRequest{To: "coordinator", Body: "example objective"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = coordinator.Pull(c, 10, 0); err != nil {
		t.Fatal(err)
	}
	if err = coordinator.Ack(c, person.Message.ID); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	if _, err = h.admin().ConsoleSteer(c, person.Message.ID, "example clarification"); err != nil {
		t.Fatal(err)
	}
	if _, err = coordinator.Reply(c, person.Message.ID, "example delivery", false); err != nil {
		t.Fatal(err)
	}
	stopped, err := coordinator.Send(c, api.SendRequest{To: "worker", Body: "example delegation"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = coordinator.StopTask(c, stopped.Message.ID, false); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond) // persisted message timestamps have millisecond precision
	if _, err = coordinator.Send(c, api.SendRequest{To: "outsider", Body: "example replacement", ThreadID: stopped.Message.ThreadID}, false); err != nil {
		t.Fatal(err)
	}
	measured, err := coordinator.CoordinatorStats(c, "", "7d")
	if err != nil {
		t.Fatal(err)
	}
	if measured.PersonTasks != 1 || measured.PersonFollowups != 1 || measured.DelegatedTasks != 3 || measured.Reassigned != 1 || measured.Withdrawn != 0 {
		t.Fatalf("coordinator flow: %+v", measured)
	}
	if _, err = h.admin().CreateProject(c, api.CreateProjectRequest{Name: "demo", Visibility: "members"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"coordinator", "worker"} {
		if _, err = h.admin().PutProjectMember(c, "demo", name, api.ProjectMemberRequest{}); err != nil {
			t.Fatal(err)
		}
	}
	projectStats, err := coordinator.AgentStats(c, client.AgentStatsFilter{Project: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	if len(projectStats.Agents) != 2 || projectStats.Agents[0].Tasks != 0 {
		t.Fatalf("project filtering: %+v", projectStats)
	}

	if _, err = h.admin().InviteMember(c, api.InviteRequest{Name: "viewer"}); err != nil {
		t.Fatal(err)
	}
	if _, err = h.admin().SetAgentOwner(c, "outsider", "viewer"); err != nil {
		t.Fatal(err)
	}
	if _, err = outsider.AgentStats(c, client.AgentStatsFilter{Project: "demo"}); err == nil {
		t.Fatal("private project accepted an outsider")
	}

	// A caller who cannot read a paid account must not learn its state via
	// candidate ordering, strict filtering, score or explanatory text.
	now := time.Now().UTC()
	if err = worker.ReportRuntime(c, api.Runtime{CLI: "codex", Quota: &api.Quota{Source: "codex", At: now, Windows: []api.QuotaWindow{{Name: "primary", Used: 1}}}}); err != nil {
		t.Fatal(err)
	}
	available, err := outsider.PickAgents(c, client.AgentPickFilter{Skill: "code", Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(available.Candidates) != 1 || available.Candidates[0].Quota != "unknown" {
		t.Fatalf("private quota affected outsider selection: %+v", available)
	}
	blocked, err := coordinator.PickAgents(c, client.AgentPickFilter{Skill: "code", Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(blocked.Candidates) != 0 {
		t.Fatalf("owner's blocked quota ignored: %+v", blocked)
	}
	e := newCLI(t, h, filepath.Join(dir, "missing.yaml"))
	for _, args := range [][]string{{"agent", "stats", "--name", "worker"}, {"agent", "pick", "--skill", "code", "--count", "1"}, {"stats", "coordinator", "--name", "coordinator"}} {
		var v map[string]any
		if err := json.Unmarshal([]byte(e.must(c, args...)), &v); err != nil {
			t.Fatal(err)
		}
		if v["version"] != float64(1) {
			t.Fatalf("missing version: %v", v)
		}
	}
	for _, args := range [][]string{{"agent", "stats", "--since", "-1d"}, {"agent", "pick", "--count", "0"}, {"stats", "coordinator", "--since", "bad", "--name", "coordinator"}} {
		code, _, stderr := e.run(c, args...)
		if code != 2 || !strings.Contains(stderr, "suggestion") {
			t.Fatalf("validation: %d %s", code, stderr)
		}
	}
	schema := e.must(c, "schema", "--all")
	for _, name := range []string{"pick", "stats", "coordinator", "strict"} {
		if !strings.Contains(schema, name) {
			t.Fatal("schema lacks " + name)
		}
	}
}
